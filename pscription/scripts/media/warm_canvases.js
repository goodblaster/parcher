// Gives animated canvases their first frames. Chrome throttles
// requestAnimationFrame for content outside the viewport, so a canvas
// animation that initializes during the scroll pass (or after it) may
// never paint at all — it stays transparent, and rasterize_canvases and
// the screenshot backends capture an empty box. Unlike cross-origin
// iframes (handled by the freeze-iframes step), a same-process canvas
// KEEPS its last frame once painted, so warming it once is enough.
//
// This sweeps the viewport down the page in viewport-height bands,
// dwelling only in bands that contain a reachable canvas with no ink yet,
// and leaving each band as soon as its canvases paint. Canvas-free pages
// pay almost nothing. Open shadow roots and same-origin iframes are
// pierced. Params: bandMs (max dwell per band, default 1200), maxMs
// (total budget, default 15000).
async function warmCanvases(bandMs, maxMs) {
    bandMs = bandMs || 1200;
    maxMs = maxMs || 15000;
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
    const deadline = Date.now() + maxMs;
    const stats = { bands: 0, dwelled: 0, warmed: 0, stillBlank: 0 };

    // Absolute page Y of an element, accumulating offsets across
    // same-origin iframe boundaries.
    function absTop(el) {
        let y = 0;
        while (el) {
            const win = el.ownerDocument.defaultView;
            y += el.getBoundingClientRect().top + win.scrollY;
            el = win.frameElement;
        }
        return y;
    }

    function visible(el) {
        const rect = el.getBoundingClientRect();
        return rect.width >= 2 && rect.height >= 2;
    }

    // root is a Document or ShadowRoot; open shadow roots and same-origin
    // iframes are pierced.
    function collect(root, out) {
        for (const c of root.querySelectorAll('canvas')) out.push(c);
        for (const frame of root.querySelectorAll('iframe')) {
            try {
                if (frame.contentDocument) collect(frame.contentDocument, out);
            } catch (e) { /* cross-origin */ }
        }
        for (const host of root.querySelectorAll('*')) {
            if (host.shadowRoot) collect(host.shadowRoot, out);
        }
    }

    function hasInk(canvas) {
        if (!canvas.width || !canvas.height) return false;
        const w = Math.max(1, Math.min(32, canvas.width));
        const h = Math.max(1, Math.min(32, canvas.height));
        const probe = document.createElement('canvas');
        probe.width = w;
        probe.height = h;
        const ctx = probe.getContext('2d');
        try {
            ctx.drawImage(canvas, 0, 0, w, h);
            const d = ctx.getImageData(0, 0, w, h).data;
            for (let i = 3; i < d.length; i += 4) if (d[i] !== 0) return true;
            return false;
        } catch (e) {
            // Unreadable (tainted / OffscreenCanvas): can't observe, don't stall.
            return true;
        }
    }

    const canvases = [];
    collect(document, canvases);
    if (canvases.length === 0) return stats;

    const vh = window.innerHeight;
    const pageH = Math.max(
        document.documentElement.scrollHeight,
        document.body ? document.body.scrollHeight : 0);

    const inBand = (el, top, bottom) => {
        if (!el.isConnected || !visible(el)) return false;
        const y = absTop(el);
        return y < bottom && (y + el.getBoundingClientRect().height) > top;
    };

    for (let y = 0; y < pageH && Date.now() < deadline; y += vh) {
        stats.bands++;
        window.scrollTo(0, y);
        // A beat for lazy loaders the scroll just triggered.
        await sleep(120);

        const blanks = canvases.filter((c) => inBand(c, y, y + vh) && !hasInk(c));
        if (blanks.length === 0) continue;

        stats.dwelled++;
        const stop = Math.min(Date.now() + bandMs, deadline);
        while (Date.now() < stop) {
            await sleep(150);
            if (blanks.every(hasInk)) break;
        }
        for (const c of blanks) {
            if (hasInk(c)) stats.warmed++;
            else stats.stillBlank++;
        }
    }

    window.scrollTo(0, 0);
    return stats;
}
