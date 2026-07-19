// Scrolls the iframe marked data-gb-frame-shot=<id> into the viewport and
// returns its viewport-relative box (accumulated across same-origin frame
// boundaries), so the Go side can screenshot exactly that region while the
// frame is composited. Fixed/sticky elements overlapping the box (site
// headers, breadcrumb bars) are hidden for the shot — they would be baked
// into the still — and restored on the next call or by the Go side's
// final restore. Companion to locate_opaque_iframes.js.
function focusOpaqueIframe(id) {
    // Restore whatever the previous focus call hid.
    for (const it of (window.__gbFrameShotHidden || [])) {
        try {
            if (it.prev) it.el.style.setProperty('visibility', it.prev, it.prio);
            else it.el.style.removeProperty('visibility');
        } catch (e) { /* element may be gone */ }
    }
    window.__gbFrameShotHidden = [];

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
    if (!frame || !frame.isConnected) return { found: false };

    // Align to the viewport BOTTOM: sticky site headers live at the top,
    // and with block:'center' they overlap the frame's box and get baked
    // into the captured still.
    frame.scrollIntoView({ block: 'end', behavior: 'instant' });

    // Viewport-relative box, walking out of any same-origin ancestor frames.
    const rect = frame.getBoundingClientRect();
    let x = rect.left;
    let y = rect.top;
    let win = frame.ownerDocument.defaultView;
    while (win.frameElement) {
        const outer = win.frameElement.getBoundingClientRect();
        x += outer.left;
        y += outer.top;
        win = win.parent;
    }

    hideOccluders(frame, x, y, rect.width, rect.height);
    return { found: true, x: x, y: y, w: rect.width, h: rect.height };

    // Hide (visibility only — no reflow) every fixed/sticky element that
    // overlaps the box and does not contain the frame, recording previous
    // inline values for restoration.
    function hideOccluders(target, bx, by, bw, bh) {
        const ancestors = new Set();
        let n = target;
        while (n) {
            ancestors.add(n);
            n = n.parentNode || n.host;
        }
        const hidden = window.__gbFrameShotHidden;

        function scan(root) {
            for (const el of root.querySelectorAll('*')) {
                if (el.shadowRoot) scan(el.shadowRoot);
                if (ancestors.has(el)) continue;
                const cs = (el.ownerDocument.defaultView || window).getComputedStyle(el);
                if (cs.position !== 'fixed' && cs.position !== 'sticky') continue;
                const r = el.getBoundingClientRect();
                if (r.width === 0 || r.height === 0) continue;
                if (r.left >= bx + bw || r.right <= bx || r.top >= by + bh || r.bottom <= by) continue;
                hidden.push({
                    el: el,
                    prev: el.style.getPropertyValue('visibility'),
                    prio: el.style.getPropertyPriority('visibility'),
                });
                el.style.setProperty('visibility', 'hidden', 'important');
            }
        }
        scan(target.ownerDocument);
        if (target.ownerDocument !== document) scan(document);
    }
}
