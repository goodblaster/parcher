// Wraps matched ranges in <mark data-parchmint="mark"> elements, using the
// text-node segment maps that text_layer.js cached on window.__pmRunCache
// (call it first with cacheRuns=true). Because the marks apply to the SAME
// extraction the Go matcher ran against — not a fresh, possibly-divergent
// re-walk — every hit lands; there is no "the two walks disagreed" class
// of miss.
//
// marks: { blockId: { ranges: [[start, end], …] } } — start/end are UTF-16
// offsets into that block's text.
//
// A phrase reads as ONE continuous highlight, spaces included, not
// per-word confetti: consecutive segments in the same text node merge into
// one wrap (interior spaces come free), and at text-node boundaries the
// wrap extends across adjoining whitespace. Wraps go back-to-front:
// surroundContents splits the text node, leaving the original node as the
// part BEFORE the split, so earlier offsets in the same node stay valid.
function applyTextMarks(marks) {
    const cache = window.__pmRunCache || {};
    const stats = { marked: 0, skipped: 0 };

    for (const blockId in marks) {
        const map = cache[blockId];
        if (!map) {
            stats.skipped += marks[blockId].ranges.length;
            continue;
        }
        const ranges = marks[blockId].ranges.slice().sort((a, b) => b[0] - a[0]);
        for (const [start, end] of ranges) {
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
