// Finds iframes whose documents page JS cannot reach (cross-origin or
// sandboxed without allow-same-origin) and marks them with
// data-gb-frame-shot=<n>. The Go side (actions.FreezeOpaqueIframes) then
// scrolls each into view, screenshots its box while it is actually
// composited — offscreen cross-origin iframes are never rasterized, even
// by captureBeyondViewport — and swaps in the pixels as an <img>.
//
// Open shadow roots and same-origin child frames are pierced (MDN wraps
// its live-sample runners in web components). Skipped: frames smaller
// than minSize (trackers, pixels), frames taller than the viewport (the
// visible crop couldn't represent them), and anything beyond maxCount
// (ad-heavy pages would otherwise burn the whole budget).
function locateOpaqueIframes(minSize, maxCount) {
    minSize = minSize || 80;
    maxCount = maxCount || 12;
    const stats = { marked: [], skipped: 0, tall: 0 };
    let next = 0;

    function reachable(frame) {
        try { return !!frame.contentDocument; } catch (e) { return false; }
    }

    function walk(root, out) {
        for (const f of root.querySelectorAll('iframe')) {
            if (reachable(f)) {
                walk(f.contentDocument, out);
            } else {
                out.push(f);
            }
        }
        for (const host of root.querySelectorAll('*')) {
            if (host.shadowRoot) walk(host.shadowRoot, out);
        }
    }

    const frames = [];
    walk(document, frames);

    for (const f of frames) {
        const rect = f.getBoundingClientRect();
        const style = (f.ownerDocument.defaultView || window).getComputedStyle(f);
        if (rect.width < minSize || rect.height < minSize ||
            style.display === 'none' || style.visibility === 'hidden') {
            stats.skipped++;
            continue;
        }
        if (rect.height > window.innerHeight) {
            stats.tall++;
            continue;
        }
        if (stats.marked.length >= maxCount) {
            stats.skipped++;
            continue;
        }
        const id = next++;
        f.setAttribute('data-gb-frame-shot', String(id));
        stats.marked.push(id);
    }
    return stats;
}
