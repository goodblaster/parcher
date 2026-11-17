// Gives animated canvases their first frames. Chrome throttles
// requestAnimationFrame for content outside the viewport, so a canvas
// animation that initializes during the scroll pass (or after it) may
// never paint at all — it stays transparent, and rasterize_canvases and
// the screenshot backends capture an empty box. Unlike cross-origin
// iframes (handled by the freeze-iframes step), a same-process canvas
// KEEPS its last frame once painted, so warming it once is enough.
//
// This visits only reachable canvases with no ink yet,
// leaving each visited band as soon as its canvases paint. Canvas-free pages
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

    // The lazy-loading scroll pass already visited the page. Here only
    // canvases which still lack pixels need another rendering opportunity.
    // Recompute positions after each dwell because lazy content can reflow.
    const pending = new Set(canvases.filter((c) => visible(c) && !hasInk(c)));
    while (pending.size && Date.now() < deadline) {
        for (const c of pending) {
            if (!c.isConnected || !visible(c) || hasInk(c)) pending.delete(c);
        }
        if (!pending.size) break;
        const next = Array.from(pending).sort((a, b) => absTop(a) - absTop(b))[0];
        window.scrollTo(0, Math.max(0, absTop(next) - window.innerHeight / 2));
        stats.bands++;
        await sleep(120); // a rendering opportunity for scroll-triggered paint
        const top = window.scrollY;
        const bottom = top + window.innerHeight;
        const blanks = Array.from(pending).filter((c) =>
            c === next || (absTop(c) < bottom && absTop(c) + c.getBoundingClientRect().height > top));
        if (!blanks.every(hasInk)) {
            stats.dwelled++;
            const stop = Math.min(Date.now() + bandMs, deadline);
            while (Date.now() < stop && !blanks.every(hasInk)) await sleep(150);
        }
        for (const c of blanks) {
            if (hasInk(c)) stats.warmed++;
            else stats.stillBlank++;
            pending.delete(c); // blank by design must not consume the whole budget
        }
    }

    window.scrollTo(0, 0);
    return stats;
}
