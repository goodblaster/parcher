// Replaces every visible <canvas> with a static <img> of its current
// pixels, so charts, games, maps, and WebGL scenes survive archiving — a
// serialized <canvas> is an empty box in every backend except full-page
// screenshots. WebGL canvases are only readable because
// preserve_webgl_buffer.js forced preserveDrawingBuffer at page load.
//
// Each canvas is copied into a scratch canvas and inspected:
//   - readable with visible pixels → replaced with an <img> data URL
//     (JPEG when fully opaque, PNG when transparency is involved).
//   - readable but fully transparent → left alone: it renders as nothing,
//     and an untouched empty canvas is the faithful capture. (Replacing it
//     with a CDP screenshot would bake the page background behind it into
//     an opaque image — worse.)
//   - unreadable (tainted by cross-origin pixels, OffscreenCanvas
//     placeholder) → marked with data-gb-canvas-shot=<n>; the Go action
//     screenshots the element via CDP (the compositor sees the pixels
//     regardless of taint) and swaps it via replace_canvas_with_image.js.
//     Only top-document canvases are marked — the CDP selector fallback
//     cannot target elements inside iframes.
//
// Same-origin iframes are walked too; cross-origin frames are unreachable
// from page context (their canvases survive only in screenshot backends).
function rasterizeCanvases() {
    const stats = { replaced: 0, transparent: 0, skipped: 0, unreadable: 0, marked: [] };
    let nextShotId = 0;

    // Backing stores can be huge (zoomed maps, DPR-scaled game buffers);
    // cap the copy so the archive and this step stay bounded.
    const MAX_DIM = 4096;

    // root is a Document or ShadowRoot; open shadow roots are pierced.
    // topLevel stays true only for canvases the CDP selector fallback can
    // reach: top document, outside any iframe and outside shadow DOM.
    function collect(root, topLevel, out) {
        for (const canvas of root.querySelectorAll('canvas')) out.push({ canvas, topLevel });
        for (const frame of root.querySelectorAll('iframe')) {
            let child = null;
            try { child = frame.contentDocument; } catch (e) { /* cross-origin */ }
            if (child) collect(child, false, out);
        }
        for (const host of root.querySelectorAll('*')) {
            if (host.shadowRoot) collect(host.shadowRoot, false, out);
        }
    }

    function makeReplacement(canvas, src) {
        const img = canvas.ownerDocument.createElement('img');
        img.src = src;
        img.alt = '';
        img.style.cssText = canvas.style.cssText;
        // Canvases are commonly styled by id or class; carrying both over
        // keeps the page's own CSS applying to the replacement.
        if (canvas.id) img.id = canvas.id;
        const c = canvas.getAttribute('class');
        if (c) img.setAttribute('class', c);

        if (canvas.hasAttribute('width')) img.setAttribute('width', canvas.getAttribute('width'));
        if (canvas.hasAttribute('height')) img.setAttribute('height', canvas.getAttribute('height'));
        if (!img.hasAttribute('width') && !img.hasAttribute('height')) {
            const rect = canvas.getBoundingClientRect();
            if (rect.width > 0 && rect.height > 0) {
                img.style.width = rect.width + 'px';
                img.style.height = rect.height + 'px';
            }
        }
        return img;
    }

    function markForShot(canvas, topLevel) {
        if (!topLevel || !canvas.isConnected) {
            stats.unreadable++;
            return;
        }
        const id = nextShotId++;
        canvas.setAttribute('data-gb-canvas-shot', String(id));
        stats.marked.push(id);
    }

    const found = [];
    collect(document, true, found);

    for (const { canvas, topLevel } of found) {
        try {
            if (!canvas.isConnected || !canvas.parentNode) { stats.skipped++; continue; }
            const view = canvas.ownerDocument.defaultView || window;
            const cs = view.getComputedStyle(canvas);
            const rect = canvas.getBoundingClientRect();
            if (rect.width < 2 || rect.height < 2 || cs.display === 'none' || cs.visibility === 'hidden') {
                stats.skipped++;
                continue;
            }
            if (!canvas.width || !canvas.height) { stats.skipped++; continue; }

            const scale = Math.min(1, MAX_DIM / Math.max(canvas.width, canvas.height));
            const w = Math.max(1, Math.round(canvas.width * scale));
            const h = Math.max(1, Math.round(canvas.height * scale));

            const scratch = document.createElement('canvas');
            scratch.width = w;
            scratch.height = h;
            const sctx = scratch.getContext('2d');
            // Throws for OffscreenCanvas placeholders → catch → CDP mark.
            sctx.drawImage(canvas, 0, 0, w, h);

            let pixels;
            try {
                pixels = sctx.getImageData(0, 0, w, h).data;
            } catch (e) {
                // SecurityError: tainted by cross-origin content.
                markForShot(canvas, topLevel);
                continue;
            }

            // Sample up to ~200k pixels: anything visible? any transparency?
            const total = w * h;
            const stride = Math.max(1, Math.floor(total / 200000));
            let maxAlpha = 0;
            let minAlpha = 255;
            for (let i = 0; i < total; i += stride) {
                const a = pixels[i * 4 + 3];
                if (a > maxAlpha) maxAlpha = a;
                if (a < minAlpha) minAlpha = a;
            }

            if (maxAlpha === 0) {
                stats.transparent++;
                continue;
            }

            const src = minAlpha === 255
                ? scratch.toDataURL('image/jpeg', 0.9)
                : scratch.toDataURL('image/png');
            canvas.parentNode.replaceChild(makeReplacement(canvas, src), canvas);
            stats.replaced++;
        } catch (e) {
            markForShot(canvas, topLevel);
        }
    }

    return stats;
}
