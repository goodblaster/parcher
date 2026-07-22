// Overlays invisible, selectable text over images at their OCR word
// positions, so text that only exists as PIXELS becomes searchable and
// selectable once the page is printed to PDF (Chrome's printToPDF turns
// transparent-color text into real, invisible PDF text). Matched words
// additionally get a translucent-yellow background — a visible highlight
// that is also selectable, in one element.
//
// specs maps an image-src hash (FNV-1a, same as text_layer.js) to its
// words: { imageHash: [ { t, x, y, w, h, hl } ] } where x/y/w/h are
// fractions [0,1] of the displayed image box and t is the word text.
// opts: { color: highlight fill }.
//
// Spans are positioned in absolute PAGE pixels (image box + fraction);
// printToPDF renders the full page at scale 1 so 1 CSS px == the PDF's
// unit, and the overlay text lands on the picture. They are appended to a
// single container so they never disturb the page's own layout.
function overlayOcrText(specs, opts) {
    const stats = { images: 0, words: 0, missed: 0 };

    function fnv1a(str) {
        let h = 0x811c9dc5;
        for (let i = 0; i < str.length; i++) {
            h ^= str.charCodeAt(i);
            h = Math.imul(h, 0x01000193) >>> 0;
        }
        return ('0000000' + h.toString(16)).slice(-8);
    }

    function collect(root, out) {
        for (const img of root.querySelectorAll('img')) out.push(img);
        for (const frame of root.querySelectorAll('iframe')) {
            try {
                if (frame.contentDocument) collect(frame.contentDocument, out);
            } catch (e) { /* cross-origin */ }
        }
        for (const host of root.querySelectorAll('*')) {
            if (host.shadowRoot) collect(host.shadowRoot, out);
        }
    }

    const layer = document.createElement('div');
    layer.setAttribute('data-parchmint', 'ocr-layer');
    layer.style.cssText = 'position:absolute;top:0;left:0;margin:0;padding:0';
    (document.body || document.documentElement).appendChild(layer);

    const imgs = [];
    collect(document, imgs);

    for (const img of imgs) {
        const src = img.currentSrc || img.src || '';
        const words = specs[fnv1a(src)];
        if (!words || !words.length) continue;
        const rect = img.getBoundingClientRect();
        if (rect.width < 2 || rect.height < 2) { stats.missed++; continue; }

        // Absolute page position of the image, walking out of any
        // same-origin iframe ancestors.
        let px = rect.left, py = rect.top;
        let win = img.ownerDocument.defaultView;
        while (win && win.frameElement) {
            const fr = win.frameElement.getBoundingClientRect();
            px += fr.left;
            py += fr.top;
            win = win.parent;
        }
        px += window.scrollX;
        py += window.scrollY;

        for (const word of words) {
            if (!word.t) continue;
            const x = px + word.x * rect.width;
            const y = py + word.y * rect.height;
            const w = Math.max(1, word.w * rect.width);
            const h = Math.max(1, word.h * rect.height);
            const span = document.createElement('span');
            // Trailing space so native PDF text extraction keeps adjacent
            // overlay words separated (our own layer-based find doesn't
            // depend on this, but third-party viewer Ctrl-F does).
            span.textContent = word.t + ' ';
            // A highlight fill is bounded by the OCR word box (width:w), so
            // it lands exactly on the word. The (transparent) glyphs are
            // free to overflow — invisible anyway — so the trailing space
            // glyph survives for native text extraction.
            let css = 'position:absolute;color:transparent;white-space:nowrap;' +
                'left:' + x + 'px;top:' + y + 'px;width:' + w + 'px;height:' + h + 'px;' +
                'font-size:' + (h * 0.82) + 'px;line-height:' + h + 'px';
            if (word.hl) css += ';background:' + (opts.color || 'rgba(255,220,0,0.45)');
            span.style.cssText = css;
            layer.appendChild(span);
            stats.words++;
        }
        stats.images++;
    }
    return stats;
}
