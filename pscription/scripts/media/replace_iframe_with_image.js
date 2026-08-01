// Replaces the iframe marked data-gb-frame-shot=<id> with an <img> whose
// src is the given data URL — the frame's own pixels, screenshotted while
// it was composited. Cross-origin iframe content can't be serialized into
// a DOM archive at all, so a still of what the user saw is the most
// faithful representation available. Companion to locate_opaque_iframes.js.
function replaceIframeWithImage(id, dataURL) {
    function find(root) {
        const hit = root.querySelector('iframe[data-gb-frame-shot="' + id + '"]');
        if (hit) return hit;
        for (const f of root.querySelectorAll('iframe')) {
            try {
                if (f.contentDocument) {
                    const r = find(f.contentDocument);
                    if (r) return r;
                }
            } catch (e) { /* cross-origin */ }
        }
        for (const host of root.querySelectorAll('*')) {
            if (host.shadowRoot) {
                const r = find(host.shadowRoot);
                if (r) return r;
            }
        }
        return null;
    }

    const frame = find(document);
    if (!frame || !frame.parentNode) return { replaced: false };

    const img = frame.ownerDocument.createElement('img');
    img.src = dataURL;
    img.alt = '';
    img.style.cssText = frame.style.cssText;
    if (frame.id) img.id = frame.id;
    const c = frame.getAttribute('class');
    if (c) img.setAttribute('class', c);

    adoptBox(frame, img);

    frame.parentNode.replaceChild(img, frame);
    return { replaced: true };

    // Transfers the replaced element's COMPUTED box onto its replacement.
    //
    // Authored styling cannot carry a box across a tag change: cssText holds only
    // INLINE styles, and any selector written for `video`/`iframe`/`canvas` stops
    // matching the moment the element becomes an <img>. The measured rect is also
    // zero whenever the element is not laid out — inside a carousel slide that has
    // never been shown, say — and an unsized replacement then renders at its
    // natural size: small and mispositioned inside a much larger slot.
    //
    // So: measure, fall back to the element's own attributes, and finally fill
    // whatever slot it lands in. Explicit inline dimensions also beat any class
    // rule that does still match, which keeps the result deterministic.
    //
    // Keep this identical across the substitution scripts — see CLAUDE.md,
    // "element → image substitution".
    function adoptBox(src, img) {
        const cs = window.getComputedStyle(src);
        const rect = src.getBoundingClientRect();
        let w = rect.width, h = rect.height;
        if (!(w > 0 && h > 0)) {
            w = parseFloat(src.getAttribute('width')) || 0;
            h = parseFloat(src.getAttribute('height')) || 0;
        }
        if (w > 0 && h > 0) {
            img.style.width = w + 'px';
            img.style.height = h + 'px';
        } else {
            img.style.width = '100%';
            img.style.height = '100%';
        }
        // Deliberately narrow: copying layout shorthands (display, margin) can
        // double-apply against rules that still match by class or id.
        // Out-of-flow placement must be carried as a COMPLETE group or not at
        // all. Absolutely positioned media is common (full-bleed heroes,
        // carousel tiles) and its parent often has no width of its own, so
        // dropping `position` collapses the layout. But copying the offsets
        // alone is worse: computed `left` is the page's placement already
        // resolved, and the `transform` that usually completes it
        // (left:50% + translateX(-50%)) must come with it or the image lands
        // shifted by half its width. In-flow elements are left alone —
        // the class and id we carry still match the page's own rules.
        if (cs.position === 'absolute' || cs.position === 'fixed') {
            // Offsets and translations are copied as PERCENTAGES, not the
            // resolved pixels. `left: 50%` computes to `left: 800px` at a
            // 1600px capture, and pinning that px value leaves the element
            // stuck there — so an archive opened in a wider window shows it
            // drifting left of centre, further the wider you go. The
            // percentages reproduce the captured position exactly (they are
            // derived from it) AND keep scaling the way the original fluid
            // layout did.
            const cb = src.offsetParent || src.parentElement;
            const cbr = cb ? cb.getBoundingClientRect() : { width: 0, height: 0 };
            const pct = (px, basis) => {
                const n = parseFloat(px);
                if (!isFinite(n) || !(basis > 0)) return null;
                return (n / basis * 100).toFixed(4) + '%';
            };

            img.style.position = cs.position;
            for (const [p, basis] of [['top', cbr.height], ['bottom', cbr.height],
                                      ['left', cbr.width], ['right', cbr.width]]) {
                const v = cs[p];
                if (!v || v === 'auto') continue;
                img.style[p] = pct(v, basis) || v;
            }
            if (cs.zIndex && cs.zIndex !== 'auto') img.style.zIndex = cs.zIndex;

            // A pure translation (the other half of the centring idiom)
            // becomes a percentage of the element's OWN box, which is what
            // CSS transform percentages resolve against.
            const m = /^matrix\(([^)]+)\)$/.exec(cs.transform || '');
            if (m) {
                const n = m[1].split(',').map(parseFloat);
                const pure = n.length === 6 && n[0] === 1 && n[1] === 0 && n[2] === 0 && n[3] === 1;
                if (pure && (n[4] || n[5])) {
                    const tx = w > 0 ? (n[4] / w * 100).toFixed(4) + '%' : n[4] + 'px';
                    const ty = h > 0 ? (n[5] / h * 100).toFixed(4) + '%' : n[5] + 'px';
                    img.style.transform = 'translate(' + tx + ', ' + ty + ')';
                } else if (cs.transform !== 'none') {
                    img.style.transform = cs.transform;
                    if (cs.transformOrigin) img.style.transformOrigin = cs.transformOrigin;
                }
            }
        }
        for (const p of ['objectFit', 'objectPosition', 'borderRadius', 'aspectRatio']) {
            const v = cs[p];
            // 'fill' is object-fit's INITIAL value, so every iframe/video/
            // canvas reports it whether or not the page asked for it —
            // and on an <img> it STRETCHES the picture to the box. Treat it
            // as unset, or a 4:3 still lands squished in a 16:9 slot.
            if (p === 'objectFit' && v === 'fill') continue;
            if (v && v !== 'auto' && v !== 'none' && v !== 'normal') img.style[p] = v;
        }
        if (!img.style.objectFit) img.style.objectFit = 'cover';
        return { width: w, height: h };
    }
}
