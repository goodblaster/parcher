// Wraps matched ranges in <mark data-parchmint="mark"> elements, using the
// text-node segment maps that text_layer.js cached on window.__pmRunCache
// (call it first with cacheRuns=true). Because the marks apply to the SAME
// extraction the Go matcher ran against — not a fresh, possibly-divergent
// re-walk — every hit lands; there is no "the two walks disagreed" class
// of miss.
//
// marks: { blockId: { ranges: [[start, end, term], …] } } — start/end are
// UTF-16 offsets into that block's text; term indexes opts.colors (per-
// query presentation). The Go side sends DISJOINT ranges (overlaps
// between terms already resolved, last term wins), which is what keeps
// the single back-to-front wrap pass valid — see below.
//
// A cache entry that is not an array is a whole-element block — a form
// control or SVG <text>, whose text is not in wrappable text nodes; see
// markElement.
//
// opts (optional): { colors: [cssColor, …] per term — missing/empty =
// the default <mark> look; style: 'bg' | 'underline' | 'box' | 'bold' }.
// Styling is inline on each <mark>, so it survives serialization with no
// injected CSS.
//
// A phrase reads as ONE continuous highlight, spaces included, not
// per-word confetti: consecutive segments in the same text node merge into
// one wrap (interior spaces come free), and at text-node boundaries the
// wrap extends across adjoining whitespace. Wraps go back-to-front:
// surroundContents splits the text node, leaving the original node as the
// part BEFORE the split, so earlier offsets in the same node stay valid.
// That invariant is also why overlap resolution CANNOT happen here: a
// second wrapping pass (e.g. per term) would see nodes the first pass
// split, and its cached offsets would be stale.
function applyTextMarks(marks, opts) {
    const cache = window.__pmRunCache || {};
    const stats = { marked: 0, skipped: 0 };
    const colors = (opts && opts.colors) || [];
    const styleKind = (opts && opts.style) || 'bg';

    function styleMark(mark, color) {
        // The UA stylesheet gives <mark> `color: black` along with the
        // yellow background. Any style that drops the background must
        // also restore `color: inherit`, or marked text on a dark theme
        // renders black-on-dark.
        switch (styleKind) {
            case 'underline':
                mark.style.background = 'transparent';
                mark.style.color = 'inherit';
                mark.style.textDecoration = 'underline';
                mark.style.textDecorationThickness = '0.12em';
                mark.style.textDecorationSkipInk = 'none';
                mark.style.textDecorationColor = color || 'gold';
                break;
            case 'box':
                mark.style.background = 'transparent';
                mark.style.color = 'inherit';
                mark.style.outline = '2px solid ' + (color || 'gold');
                mark.style.outlineOffset = '1px';
                break;
            case 'bold':
                mark.style.background = 'transparent';
                mark.style.fontWeight = 'bold';
                mark.style.color = color || 'inherit';
                break;
            default: // 'bg': browser <mark> yellow unless a color is given
                if (color) mark.style.backgroundColor = color;
        }
    }

    function markElement(entry, color) {
        const el = entry.el;
        if (!el || !el.isConnected) return false;
        try {
            if (entry.via === 'svg' && typeof el.getBBox === 'function') {
                const box = el.getBBox();
                const rect = el.ownerDocument.createElementNS('http://www.w3.org/2000/svg', 'rect');
                rect.setAttribute('data-parchmint', 'mark');
                rect.setAttribute('x', box.x - 1);
                rect.setAttribute('y', box.y - 1);
                rect.setAttribute('width', box.width + 2);
                rect.setAttribute('height', box.height + 2);
                rect.setAttribute('fill', color || 'yellow');
                rect.setAttribute('fill-opacity', '0.5');
                rect.setAttribute('pointer-events', 'none');
                const t = el.getAttribute('transform');
                if (t) rect.setAttribute('transform', t);
                el.before(rect);
                return true;
            }
            el.setAttribute('data-parchmint-mark', 'control');
            el.style.setProperty('outline', '3px solid ' + (color || 'gold'), 'important');
            el.style.setProperty('outline-offset', '1px', 'important');
            return true;
        } catch (e) {
            return false;
        }
    }

    for (const blockId in marks) {
        const map = cache[blockId];
        if (!map) {
            stats.skipped += marks[blockId].ranges.length;
            continue;
        }
        // A whole-element block (form control, SVG <text>): there are no
        // text nodes to wrap, so the element itself is marked — an outline
        // on a control, a backing rect behind SVG text.
        if (!Array.isArray(map)) {
            const n = marks[blockId].ranges.length;
            const term = n > 0 ? marks[blockId].ranges[n - 1][2] : 0;
            if (markElement(map, colors[term] || '')) stats.marked += n;
            else stats.skipped += n;
            continue;
        }
        const ranges = marks[blockId].ranges.slice().sort((a, b) => b[0] - a[0]);
        for (const [start, end, term] of ranges) {
            const segs = map.filter((s) => s.end > start && s.start < end);

            // One wrap per text node covered by the match.
            const groups = [];
            for (const seg of segs) {
                if (!seg.node.isConnected) continue;
                const from = seg.nodeStart + Math.max(0, start - seg.start);
                const to = seg.nodeStart + Math.min(seg.nodeEnd - seg.nodeStart, end - seg.start);
                if (to <= from) continue;
                const last = groups[groups.length - 1];
                if (last && last.node === seg.node) {
                    last.to = Math.max(last.to, to);
                } else {
                    groups.push({ node: seg.node, from: from, to: to });
                }
            }
            if (groups.length === 0) {
                stats.skipped++;
                continue;
            }

            // Bridge whitespace at node boundaries so the highlight is
            // continuous across "apple <b>gift</b> card"-style markup.
            for (let i = 0; i < groups.length; i++) {
                const g = groups[i];
                const data = g.node.nodeValue || '';
                if (i > 0) {
                    while (g.from > 0 && /\s/.test(data[g.from - 1])) g.from--;
                }
                if (i < groups.length - 1) {
                    while (g.to < data.length && /\s/.test(data[g.to])) g.to++;
                }
            }

            let wrapped = false;
            for (let i = groups.length - 1; i >= 0; i--) {
                const g = groups[i];
                try {
                    const range = g.node.ownerDocument.createRange();
                    range.setStart(g.node, g.from);
                    range.setEnd(g.node, g.to);
                    const mark = g.node.ownerDocument.createElement('mark');
                    mark.setAttribute('data-parchmint', 'mark');
                    const color = colors[term] || '';
                    if (color) mark.setAttribute('data-parchmint-color', color);
                    styleMark(mark, color);
                    range.surroundContents(mark);
                    wrapped = true;
                } catch (e) {
                    /* a range that can't be surrounded (rare) is skipped */
                }
            }
            if (wrapped) stats.marked++;
            else stats.skipped++;
        }
    }
    return stats;
}
