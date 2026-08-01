// Extracts the rendered text of the page with word-level geometry — the
// "text layer" (see parchmint's TEXTLAYER.md for the format contract).
// Returns { viewport, dpr, stats, blocks }; the Go side wraps it with
// url/capturedAt/generator and embeds it in the archive.
//
// Ground rules (the parts of HTML text extraction that are notoriously
// hard, and how they're handled here):
//   - Block boundaries come from COMPUTED display via the anonymous-box
//     rule — accumulate inline-level content, flush at every block-level
//     child — never from tag names. display:block spans split;
//     display:inline divs don't; display:contents is transparent.
//   - Words merge across inline elements (Hel<b>lo</b> is one word):
//     runs accumulate text segments and only whitespace or a block
//     boundary ends a word.
//   - Whitespace collapses per the parent's white-space property;
//     <br> contributes '\n' (a line break, not a new block).
//   - Reading order is DOM order — which is also what select-all/copy
//     emits, our fidelity target.
//   - Rendered = has client rects. Text with no rects (display:none,
//     collapsed sections) or sub-visible rects (1px sr-only clips) is
//     excluded, matching what a human sees.
//   - The composed tree is walked: open shadow roots pierced (slots
//     resolved via assignedNodes), same-origin iframes included with
//     coordinates offset to the top page, cross-origin iframes skipped
//     (the pipeline already froze them to images).
//   - CJK and other spaceless scripts are segmented with the browser's
//     own Intl.Segmenter rather than anything hand-rolled.
//   - Self-check: clean leaf blocks are compared against the element's
//     innerText (the spec'd select-all approximation); mismatches are
//     flagged, and a whole-page Selection.toString() similarity lands in
//     stats. The browser referees our walker.
//
// cacheRuns (optional): when true, each block's text-node segment map is
// stashed on window.__pmRunCache keyed by block id, so a follow-up call to
// apply_text_marks.js can wrap ranges in <mark> against THIS EXACT
// extraction — no second walk, no assumption that two walks agree (they
// don't always: fonts/layout settle between calls and shift block
// boundaries). See apply_text_marks.js.
function extractTextLayer(cacheRuns) {
    const blocks = [];
    // Per-block text-node segment maps, exposed for apply_text_marks.js.
    const runCache = {};
    // Visible images are recorded too (node, page box, natural size, and
    // an FNV-1a hash of the src) so a later `parch index` can OCR the
    // archive's images and place the recognized words in page coordinates
    // WITHOUT re-running a browser. By extraction time the prep pipeline
    // has inlined images as data URIs, so the hash identifies the same
    // bytes the archive will carry.
    const images = [];
    const stats = { blocks: 0, words: 0, chars: 0, oracleMismatches: 0, framesWalked: 0, images: 0 };

    const BLOCK_TYPES = {
        P: 'p', H1: 'h1', H2: 'h2', H3: 'h3', H4: 'h4', H5: 'h5', H6: 'h6',
        LI: 'li', TD: 'td', TH: 'th', CAPTION: 'caption', PRE: 'pre',
        BLOCKQUOTE: 'blockquote',
    };
    const SKIP_TAGS = new Set(['SCRIPT', 'STYLE', 'NOSCRIPT', 'TEMPLATE', 'IFRAME', 'SVG',
        'CANVAS', 'VIDEO', 'AUDIO', 'OBJECT', 'EMBED', 'SELECT', 'TEXTAREA']);

    const hasCJK = /[ᄀ-ᇿ⺀-鿿ꀀ-꓏가-힯豈-﫿ｦ-ﾟ]/;
    const segmenter = typeof Intl !== 'undefined' && Intl.Segmenter
        ? new Intl.Segmenter(undefined, { granularity: 'word' })
        : null;

    // ---- run assembly -----------------------------------------------------
    // A run is the inline content between block boundaries: a growing text
    // string plus a map from text offsets back to (textNode, nodeOffset)
    // so word ranges can be measured. Whitespace contributes at most one
    // ' ' between mapped segments and needs no mapping of its own.

    function newRun(container, path, frame, offset) {
        return { container, path, frame, offset, text: '', map: [], pendingSpace: false };
    }

    function addSegment(run, node, nodeStart, nodeEnd, str) {
        if (run.pendingSpace && run.text.length > 0) run.text += ' ';
        run.pendingSpace = false;
        run.map.push({ start: run.text.length, end: run.text.length + str.length, node, nodeStart, nodeEnd });
        run.text += str;
    }

    function addText(run, node) {
        const raw = node.nodeValue;
        if (!raw) return;
        const parent = node.parentElement;
        const ws = parent ? getComputedStyle(parent).whiteSpace : 'normal';
        const pre = ws === 'pre' || ws === 'pre-wrap' || ws === 'break-spaces';
        if (pre) {
            addSegment(run, node, 0, raw.length, raw);
            return;
        }
        // Collapse mode: alternate whitespace / non-whitespace tokens.
        const re = /(\s+)|(\S+)/g;
        let m;
        while ((m = re.exec(raw)) !== null) {
            if (m[1] !== undefined) {
                run.pendingSpace = true;
            } else {
                addSegment(run, node, m.index, m.index + m[2].length, m[2]);
            }
        }
    }

    function addBreak(run) {
        if (run.text.length === 0) return;
        run.pendingSpace = false;
        run.text += '\n';
    }

    // ---- geometry ---------------------------------------------------------

    function rangeBox(doc, run, start, end) {
        // Find the mapped segments covering [start, end).
        let first = null;
        let last = null;
        for (const seg of run.map) {
            if (seg.end <= start) continue;
            if (seg.start >= end) break;
            if (!first) first = seg;
            last = seg;
        }
        if (!first) return null;
        const range = doc.createRange();
        try {
            range.setStart(first.node, first.nodeStart + Math.max(0, start - first.start));
            range.setEnd(last.node, last.nodeStart + Math.min(last.nodeEnd - last.nodeStart, end - last.start));
        } catch (e) {
            return null;
        }
        let x1 = Infinity, y1 = Infinity, x2 = -Infinity, y2 = -Infinity;
        let visible = false;
        for (const r of range.getClientRects()) {
            // Sub-visible slivers (sr-only 1px clips) don't count as rendered.
            if (r.width < 0.5 || r.height < 3) continue;
            visible = true;
            if (r.left < x1) x1 = r.left;
            if (r.top < y1) y1 = r.top;
            if (r.right > x2) x2 = r.right;
            if (r.bottom > y2) y2 = r.bottom;
        }
        if (!visible) return null;
        return [x1 + run.offset.x, y1 + run.offset.y, x2 - x1, y2 - y1];
    }

    // ---- word segmentation ------------------------------------------------

    function segmentWords(text) {
        // Whitespace-delimited units; spaceless-script units are further
        // split with Intl.Segmenter so CJK gets real word boundaries.
        const words = [];
        const re = /\S+/g;
        let m;
        while ((m = re.exec(text)) !== null) {
            const tok = m[0];
            if (segmenter && tok.length > 1 && hasCJK.test(tok)) {
                for (const seg of segmenter.segment(tok)) {
                    if (seg.segment.trim().length === 0) continue;
                    words.push([m.index + seg.index, m.index + seg.index + seg.segment.length]);
                }
            } else {
                words.push([m.index, m.index + tok.length]);
            }
        }
        return words;
    }

    // ---- oracle -----------------------------------------------------------

    const normWS = (s) => s.replace(/\s+/g, ' ').trim();

    function checkOracle(block, container) {
        // Only clean leaves: a typed container with no block-level element
        // children of its own — its innerText is exactly this block. Shadow
        // trees are exempt: innerText's slot semantics differ from the
        // composed-tree copy order we deliberately produce.
        if (!(container.tagName in BLOCK_TYPES)) return;
        if (container.getRootNode() instanceof ShadowRoot || container.querySelector('slot')) return;
        for (const child of container.children) {
            const d = getComputedStyle(child).display;
            if (d !== 'none' && !d.startsWith('inline') && d !== 'contents') return;
        }
        // Case-insensitive: innerText applies text-transform, DOM text doesn't.
        if (normWS(container.innerText || '').toLowerCase() !== normWS(block.text).toLowerCase()) {
            block.oracle = 'mismatch';
            stats.oracleMismatches++;
        }
    }

    // ---- flushing a run into a block --------------------------------------

    function flushRun(run) {
        const text = run.text.replace(/\s+$/, '');
        run.pendingSpace = false;
        if (text.length === 0) {
            run.text = '';
            run.map = [];
            return;
        }
        const doc = run.container.ownerDocument;
        const map = run.map;
        const words = [];
        let bx1 = Infinity, by1 = Infinity, bx2 = -Infinity, by2 = -Infinity;
        for (const [start, end] of segmentWords(text)) {
            const box = rangeBox(doc, run, start, end);
            if (!box) continue;
            words.push([start, end,
                Math.round(box[0]), Math.round(box[1]), Math.round(box[2]), Math.round(box[3])]);
            if (box[0] < bx1) bx1 = box[0];
            if (box[1] < by1) by1 = box[1];
            if (box[0] + box[2] > bx2) bx2 = box[0] + box[2];
            if (box[1] + box[3] > by2) by2 = box[1] + box[3];
        }
        run.text = '';
        run.map = [];
        if (words.length === 0) return; // nothing rendered

        // Clip-chain check: overflow clipping does NOT shrink client rects,
        // so 1px-clipped "sr-only" text (position:absolute; width:1px;
        // overflow:hidden; clip:…) still measures full-size. Intersect the
        // block's box with every non-visible-overflow ancestor box (the
        // container itself included); a sub-visible remainder isn't seen by
        // a human and stays out of the layer.
        let cx1 = bx1 - run.offset.x, cy1 = by1 - run.offset.y;
        let cx2 = bx2 - run.offset.x, cy2 = by2 - run.offset.y;
        for (let el = run.container; el && el.nodeType === Node.ELEMENT_NODE;
            el = el.parentElement || (el.getRootNode() instanceof ShadowRoot ? el.getRootNode().host : null)) {
            const style = (el.ownerDocument.defaultView || window).getComputedStyle(el);
            if (style.overflow !== 'visible' || style.clip !== 'auto') {
                const r = el.getBoundingClientRect();
                if (r.left > cx1) cx1 = r.left;
                if (r.top > cy1) cy1 = r.top;
                if (r.right < cx2) cx2 = r.right;
                if (r.bottom < cy2) cy2 = r.bottom;
            }
        }
        if (cx2 - cx1 < 2 || cy2 - cy1 < 3) return; // clipped to invisibility

        // Parked fully above/left of the page origin (skip links at
        // top:-9999px and friends): rendered per the engine, invisible to
        // a human. Content below/right stays — pages extend that way.
        if (bx2 <= 0 || by2 <= 0) return;

        const block = {
            id: blocks.length,
            type: BLOCK_TYPES[run.container.tagName] || 'other',
            node: run.path,
            box: [Math.round(bx1), Math.round(by1), Math.round(bx2 - bx1), Math.round(by2 - by1)],
            text: text,
            words: words,
        };
        if (run.frame) block.frame = run.frame;
        checkOracle(block, run.container);
        blocks.push(block);
        stats.blocks++;
        stats.words += words.length;
        stats.chars += text.length;

        // Stash the segment map so apply_text_marks.js can wrap ranges in
        // this block against the exact extraction they were matched on.
        if (cacheRuns) runCache[block.id] = map;
    }

    // ---- images -----------------------------------------------------------

    // FNV-1a (32-bit) over the string's UTF-16 code units; must stay in
    // lockstep with textlayer.FNV1a on the Go side, which re-hashes the
    // archive's img srcs to pair them with these entries at index time.
    function fnv1a(str) {
        let h = 0x811c9dc5;
        for (let i = 0; i < str.length; i++) {
            h ^= str.charCodeAt(i);
            h = Math.imul(h, 0x01000193) >>> 0;
        }
        return ('0000000' + h.toString(16)).slice(-8);
    }

    function collectImage(img, frame, offset) {
        const rect = img.getBoundingClientRect();
        if (rect.width < 16 || rect.height < 16) return;
        const src = img.currentSrc || img.src || '';
        if (!src) return;
        const entry = {
            node: elementPath(img),
            box: [Math.round(rect.left + offset.x), Math.round(rect.top + offset.y),
                Math.round(rect.width), Math.round(rect.height)],
            natural: [img.naturalWidth || 0, img.naturalHeight || 0],
            hash: fnv1a(src),
        };
        if (frame) entry.frame = frame;
        images.push(entry);
        stats.images++;
    }

    // ---- tree walk --------------------------------------------------------

    function elementPath(el) {
        const idx = [];
        let n = el;
        while (n) {
            const parent = n.parentElement || (n.parentNode && n.parentNode.host);
            if (!parent) break;
            let i = 0;
            let sib = n;
            while ((sib = sib.previousElementSibling) !== null) i++;
            idx.unshift(i);
            n = parent;
        }
        return idx.join('/');
    }

    // Walks the children of `el` (composed order) feeding the current run;
    // block-level children flush the run and open their own container walk.
    function walkChildren(el, run, frame, offset) {
        const children = el.shadowRoot ? el.shadowRoot.childNodes
            : (el.tagName === 'SLOT' ? el.assignedNodes({ flatten: true }) : el.childNodes);
        for (const node of children) {
            if (node.nodeType === Node.TEXT_NODE) {
                addText(run, node);
                continue;
            }
            if (node.nodeType !== Node.ELEMENT_NODE) continue;
            const tag = node.tagName.toUpperCase();
            if (tag === 'BR') {
                addBreak(run);
                continue;
            }
            if (tag === 'IMG') {
                collectImage(node, frame, offset);
                continue;
            }
            if (SKIP_TAGS.has(tag)) {
                if (tag === 'IFRAME') walkFrame(node, frame, offset);
                continue;
            }
            const display = getComputedStyle(node).display;
            if (display === 'none') continue;
            if (display === 'contents' || display.startsWith('inline') || tag === 'SLOT') {
                // Participates in the current run (atomic inlines included:
                // their text joins the line, matching copy behavior).
                walkChildren(node, run, frame, offset);
                continue;
            }
            // Block-level child (shadow hosts included — walkChildren picks
            // their shadowRoot children): flush the run, walk as container.
            flushRun(run);
            walkContainer(node, frame, offset);
        }
    }

    function walkContainer(el, frame, offset) {
        const run = newRun(el, elementPath(el), frame, offset);
        walkChildren(el, run, frame, offset);
        flushRun(run);
    }

    // A document's offset converts its viewport-relative client rects to
    // top-page coordinates (rects already account for each frame's own
    // internal scroll). Top document: offset = window scroll. Child frame:
    // offset = the frame element's rect within ITS document + that
    // document's offset — recursion carries the chain down.
    function walkFrame(frameEl, parentFrame, parentOffset) {
        let doc = null;
        try { doc = frameEl.contentDocument; } catch (e) { /* cross-origin */ }
        if (!doc || !doc.body) return;
        const r = frameEl.getBoundingClientRect();
        if (r.width < 2 || r.height < 2) return;
        const offset = {
            x: r.left + frameEl.clientLeft + parentOffset.x,
            y: r.top + frameEl.clientTop + parentOffset.y,
        };
        stats.framesWalked++;
        const framePath = (parentFrame ? parentFrame + '>' : '') + elementPath(frameEl);
        walkContainer(doc.body, framePath, offset);
    }

    // ---- go ---------------------------------------------------------------

    if (!document.body) {
        return {
            viewport: { width: window.innerWidth, height: window.innerHeight, dpr: window.devicePixelRatio || 1 },
            stats: stats,
            blocks: blocks,
            images: images,
        };
    }
    const topOffset = { x: window.scrollX, y: window.scrollY };
    walkContainer(document.body, '', topOffset);
    if (cacheRuns) window.__pmRunCache = runCache;

    // Whole-page self-check against programmatic select-all (top document
    // only — selection does not cross iframe boundaries).
    let similarity = null;
    try {
        const sel = window.getSelection();
        sel.removeAllRanges();
        sel.selectAllChildren(document.body);
        const browserText = sel.toString();
        sel.removeAllRanges();
        const tokens = (s) => {
            const counts = new Map();
            for (const t of s.toLowerCase().split(/\s+/)) {
                if (t) counts.set(t, (counts.get(t) || 0) + 1);
            }
            return counts;
        };
        const a = tokens(browserText);
        const b = tokens(blocks.filter((bl) => !bl.frame).map((bl) => bl.text).join(' '));
        let common = 0, na = 0, nb = 0;
        for (const [, c] of a) na += c;
        for (const [, c] of b) nb += c;
        for (const [t, c] of a) common += Math.min(c, b.get(t) || 0);
        similarity = na + nb === 0 ? 1 : Math.round((2 * common / (na + nb)) * 1000) / 1000;
    } catch (e) { /* best-effort */ }
    stats.pageSimilarity = similarity;

    return {
        viewport: { width: window.innerWidth, height: window.innerHeight, dpr: window.devicePixelRatio || 1 },
        stats: stats,
        blocks: blocks,
        images: images,
    };
}
