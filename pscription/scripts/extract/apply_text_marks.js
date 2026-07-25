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
        switch (styleKind) {
            case 'underline':
                mark.style.background = 'transparent';
                mark.style.textDecoration = 'underline';
                mark.style.textDecorationThickness = '0.12em';
                mark.style.textDecorationSkipInk = 'none';
                mark.style.textDecorationColor = color || 'gold';
                break;
            case 'box':
                mark.style.background = 'transparent';
                mark.style.outline = '2px solid ' + (color || 'gold');
                mark.style.outlineOffset = '1px';
                break;
            case 'bold':
                mark.style.background = 'transparent';
                mark.style.fontWeight = 'bold';
                if (color) mark.style.color = color;
                break;
            default: // 'bg': browser <mark> yellow unless a color is given
                if (color) mark.style.backgroundColor = color;
        }
    }

    for (const blockId in marks) {
        const map = cache[blockId];
        if (!map) {
            stats.skipped += marks[blockId].ranges.length;
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
