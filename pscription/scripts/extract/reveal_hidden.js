// Reveals hidden blocks so marks placed in them can be seen — the page-side
// half of `parch mark --hidden`. Only ever run against a COPY being marked:
// an archive stays the page as it was, and a capture never reveals anything.
//
// ids: block ids from the `hidden` array of the extraction that
// text_layer.js just ran with cacheRuns=true. Each block's cached text-node
// map locates it; from there the ancestor chain is walked and every cause of
// hiding found on it is undone with an inline !important override:
//
//   closed <details>            -> open
//   hidden / hidden=until-found -> attribute removed
//   display:none                -> display:revert (block if that is still none)
//   content-visibility:hidden   -> visible
//   visibility, opacity:0       -> visible, 1
//   font-size:0, text-indent    -> 1rem, 0
//   a box collapsed to nothing (zero-height accordion, sr-only)
//                               -> size and clipping lifted
//   a sized box showing other content (carousel, scroller)
//                               -> shifted to the match; never un-clipped
//                                  sideways, which would spill every slide.
//                                  The shift is written as a `translate` on
//                                  the box's children, not left as a scroll
//                                  offset: a saved file cannot record
//                                  scrollLeft, so a scrolled box came back
//                                  showing its first slide and the marked
//                                  .html hid the very match it reported.
//                                  A box shows ONE position — the first
//                                  match wins; later ones in other slides
//                                  are marked but counted as `elsewhere`.
//   a carousel library's CLONE of a slide
//                               -> left alone (`duplicate`): the original
//                                  slide holds the same text, and a clone is
//                                  an incomplete copy (on test-site-2 the
//                                  clones have no video thumbnail), so
//                                  parking the carousel on one showed a
//                                  slide the page never really displays
//   parked off-screen           -> position:static
//
// State causes are undone first and geometry measured after, because a
// display:none ancestor gives every box under it zero size and would make
// everything look "clipped".
//
// Nothing is passed off as something the page showed: the outermost element
// changed for each block gets data-parchmint-revealed, a dashed outline, and
// a small label (opts.label, default "hidden on the original page").
//
// This runs BEFORE apply_text_marks.js. It never splits or moves a text
// node, so the cached offsets the marks pass relies on stay valid.
function revealHidden(ids, opts) {
    const cache = window.__pmRunCache || {};
    const labelText = (opts && opts.label) || 'hidden on the original page';
    const stats = { revealed: 0, unresolved: 0, changed: 0, duplicate: 0, elsewhere: 0 };
    // Clipping boxes already shifted to a match, across all blocks.
    const positioned = new Set();
    // What the slider libraries call the copies they add for wrap-around.
    const CLONE = '.slick-cloned, .swiper-slide-duplicate, .owl-item.cloned, .splide__slide--clone, .glide__slide--clone, .tns-slide-cloned, [data-clone="true"]';
    const TABLE_PARTS = new Set(['TR', 'TD', 'TH', 'TBODY', 'THEAD', 'TFOOT', 'TABLE', 'COL', 'COLGROUP', 'CAPTION']);
    const VOID = new Set(['IMG', 'INPUT', 'BR', 'HR', 'SELECT', 'TEXTAREA', 'VIDEO', 'AUDIO', 'CANVAS', 'IFRAME', 'SVG']);

    const parentOf = (el) => el.parentElement
        || (el.getRootNode() instanceof ShadowRoot ? el.getRootNode().host : null);

    function chain(node) {
        const out = [];
        for (let el = node.nodeType === Node.ELEMENT_NODE ? node : node.parentElement; el; el = parentOf(el)) {
            out.push(el);
            if (el.tagName === 'BODY') break;
        }
        return out;
    }

    function set(el, prop, value, changed) {
        el.style.setProperty(prop, value, 'important');
        changed.add(el);
    }

    function visibleRects(seg) {
        try {
            const range = seg.node.ownerDocument.createRange();
            range.setStart(seg.node, seg.nodeStart);
            range.setEnd(seg.node, seg.nodeEnd);
            for (const r of range.getClientRects()) {
                if (r.width >= 0.5 && r.height >= 3) return r;
            }
        } catch (e) { /* fall through */ }
        return null;
    }

    function undoState(els, changed) {
        // Outermost first: an ancestor's display decides whether anything
        // below it computes meaningfully.
        for (let i = els.length - 1; i >= 0; i--) {
            const el = els[i];
            const parent = el.parentElement;
            if (parent && parent.tagName === 'DETAILS' && !parent.open) {
                parent.setAttribute('open', '');
                changed.add(parent);
            }
            if (el.hasAttribute('hidden')) {
                el.removeAttribute('hidden');
                changed.add(el);
            }
            let style = getComputedStyle(el);
            if (style.display === 'none') {
                set(el, 'display', 'revert', changed);
                if (getComputedStyle(el).display === 'none') set(el, 'display', 'block', changed);
                style = getComputedStyle(el);
            }
            // A closed <dialog> revealed with display alone comes back
            // absolutely positioned and centred, on top of the page.
            if (el.tagName === 'DIALOG' && !el.open) {
                set(el, 'position', 'static', changed);
                set(el, 'margin', '0', changed);
            }
            if (el.hasAttribute('data-parchmint-hidden')) {
                // An .mht keeps the capture's stand-in for `hidden`.
                el.removeAttribute('data-parchmint-hidden');
                changed.add(el);
            }
            if (style.contentVisibility === 'hidden') set(el, 'content-visibility', 'visible', changed);
            if (style.visibility !== 'visible') set(el, 'visibility', 'visible', changed);
            if (style.opacity === '0') set(el, 'opacity', '1', changed);
            if (parseFloat(style.fontSize) === 0) set(el, 'font-size', '1rem', changed);
            if (parseFloat(style.textIndent) <= -100) set(el, 'text-indent', '0', changed);
            if (/^inset\(\s*(50|[6-9]\d|100)%/.test(style.clipPath)) set(el, 'clip-path', 'none', changed);
        }
    }

    // Turns the scroll offsets scrollIntoView just gave the boxes on the
    // ancestor chain into a `translate` on their children, and zeroes the
    // offsets. Same picture, but one that survives being saved.
    function bakeScroll(els, changed) {
        for (const el of els) {
            if (el.tagName === 'BODY' || el.tagName === 'HTML') continue;
            const dx = el.scrollLeft, dy = el.scrollTop;
            if (!dx && !dy) continue;
            el.scrollLeft = 0;
            el.scrollTop = 0;
            for (const child of el.children) {
                if (child.getAttribute('data-parchmint') === 'reveal-label') continue;
                child.style.setProperty('translate', (-dx) + 'px ' + (-dy) + 'px', 'important');
            }
            positioned.add(el);
            changed.add(el);
        }
    }

    // Returns true (visible), false (could not be shown), or 'elsewhere'
    // (its box is already showing another match).
    function undoGeometry(seg, els, changed) {
        const scrolled = new Set();
        // Up to a few rounds: lifting one clip can expose the next.
        for (let round = 0; round < 6; round++) {
            const rect = visibleRects(seg);
            if (!rect) return false;
            let x1 = rect.left, y1 = rect.top, x2 = rect.right, y2 = rect.bottom;
            let culprit = null;
            for (const el of els) {
                const style = getComputedStyle(el);
                if (style.overflow !== 'visible' || style.clip !== 'auto') {
                    const r = el.getBoundingClientRect();
                    x1 = Math.max(x1, r.left); y1 = Math.max(y1, r.top);
                    x2 = Math.min(x2, r.right); y2 = Math.min(y2, r.bottom);
                    if (x2 - x1 < 2 || y2 - y1 < 3) { culprit = el; break; }
                }
            }
            if (culprit) {
                // A clipping box with real size is a VIEWPORT onto content
                // laid out beyond it — a carousel showing another slide, a
                // scroller, a "read more" panel. Lifting its overflow would
                // spill every slide across the page (measured on a
                // reference site: one match in an off-view slide made the
                // marked page ten times wider). Scroll it to the match
                // instead; only a box collapsed to nothing gets opened up.
                const box = culprit.getBoundingClientRect();
                if (box.width >= 40 && box.height >= 16) {
                    if (positioned.has(culprit) && !scrolled.has(culprit)) return 'elsewhere';
                    if (!scrolled.has(culprit)) {
                        scrolled.add(culprit);
                        const target = seg.node.parentElement;
                        const pageX = window.scrollX, pageY = window.scrollY;
                        try { target.scrollIntoView({ block: 'nearest', inline: 'nearest' }); } catch (e) { /* ignore */ }
                        window.scrollTo(pageX, pageY);
                        bakeScroll(els, changed);
                        changed.add(culprit);
                        continue;
                    }
                    // Scrolling did not bring it into view (a transformed
                    // track the box cannot scroll). Open the box downwards
                    // only when that is the direction the text is cut off;
                    // never sideways.
                    const cutVertically = rect.right > box.left + 2 && rect.left < box.right - 2;
                    if (!cutVertically) return false;
                    set(culprit, 'max-height', 'none', changed);
                    set(culprit, 'height', 'auto', changed);
                    set(culprit, 'overflow-y', 'visible', changed);
                    continue;
                }
                set(culprit, 'overflow', 'visible', changed);
                set(culprit, 'clip', 'auto', changed);
                set(culprit, 'clip-path', 'none', changed);
                set(culprit, 'max-height', 'none', changed);
                set(culprit, 'max-width', 'none', changed);
                const r = culprit.getBoundingClientRect();
                if (r.height < 3) set(culprit, 'height', 'auto', changed);
                if (r.width < 3) set(culprit, 'width', 'auto', changed);
                // The sr-only pattern takes the box out of flow too.
                const pos = getComputedStyle(culprit).position;
                if ((pos === 'absolute' || pos === 'fixed') && (r.width < 3 || r.height < 3)) {
                    set(culprit, 'position', 'static', changed);
                    set(culprit, 'margin', '0', changed);
                }
                continue;
            }
            // Not clipped: parked off the page?
            const pageRight = rect.right + window.scrollX, pageBottom = rect.bottom + window.scrollY;
            if (pageRight <= 0 || pageBottom <= 0) {
                let moved = false;
                for (const el of els) {
                    const style = getComputedStyle(el);
                    if (style.position === 'absolute' || style.position === 'fixed') {
                        set(el, 'position', 'static', changed);
                        moved = true;
                        break;
                    }
                    if (style.transform !== 'none') {
                        set(el, 'transform', 'none', changed);
                        moved = true;
                        break;
                    }
                }
                if (moved) continue;
                return false;
            }
            return true;
        }
        return !!visibleRects(seg);
    }

    function tag(el) {
        if (el.hasAttribute('data-parchmint-revealed')) return;
        // One labelled box per revealed region: skip when an ancestor
        // already carries it.
        for (let p = parentOf(el); p; p = parentOf(p)) {
            if (p.hasAttribute('data-parchmint-revealed')) return;
        }
        el.setAttribute('data-parchmint-revealed', '');
        el.style.setProperty('outline', '2px dashed #c2410c', 'important');
        el.style.setProperty('outline-offset', '2px', 'important');
        if (TABLE_PARTS.has(el.tagName) || VOID.has(el.tagName.toUpperCase())) return;
        const label = el.ownerDocument.createElement('span');
        label.setAttribute('data-parchmint', 'reveal-label');
        label.textContent = labelText;
        label.style.cssText = 'all:initial;display:block;width:fit-content;margin:0 0 4px;padding:1px 6px;'
            + 'font:600 11px/1.5 system-ui,sans-serif;letter-spacing:.02em;color:#fff;background:#c2410c;'
            + 'border-radius:3px;';
        // After a <summary>, never before it: a details' summary must stay
        // its first child to keep working as one.
        const first = el.firstElementChild;
        if (el.tagName === 'DETAILS' && first && first.tagName === 'SUMMARY') first.after(label);
        else el.prepend(label);
    }

    for (const id of ids) {
        const map = cache[id];
        const seg = Array.isArray(map) ? map.find((s) => s.node && s.node.isConnected) : null;
        if (!seg) { stats.unresolved++; continue; }
        const els = chain(seg.node);
        if (els[0].closest(CLONE)) { stats.duplicate++; continue; }
        const changed = new Set();
        undoState(els, changed);
        const ok = undoGeometry(seg, els, changed);
        stats.changed += changed.size;
        if (changed.size > 0) {
            // The outermost element touched is the region that was hidden.
            let outer = null;
            for (const el of els) if (changed.has(el)) outer = el;
            if (!outer) outer = changed.values().next().value;
            if (outer.tagName === 'BODY' || outer.tagName === 'HTML') outer = els[0];
            tag(outer);
        }
        if (ok === 'elsewhere') stats.elsewhere++;
        else if (ok) stats.revealed++;
        else stats.unresolved++;
    }
    return stats;
}
