// Bakes highlight rectangles into images, for marking text that OCR found
// inside them ("parch mark"). specs maps an image-src hash (FNV-1a, same
// as text_layer.js) to rectangles in IMAGE-RELATIVE fractions
// [x, y, w, h, term] of the displayed box — fractions are scale-invariant,
// so they apply directly at natural resolution; term indexes opts.colors
// (missing = opts.color). The highlight is painted into the image bytes
// and the src swapped, so it survives any reflow and any viewer, with no
// wrappers, scripts, or CSS tricks. Rects paint in term order, so where
// different-term highlights overlap, the last term wins. opts: {
// grayscale: bool (mute the image so the highlight pops), color: css
// fill, stroke: css stroke, colors: [css fill per term],
// style: 'bg' | 'box' (box = outline only, reads better in B&W) }.
function bakeImageMarks(specs, opts) {
    const stats = { marked: 0, missed: 0 };

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

    const imgs = [];
    collect(document, imgs);

    for (const img of imgs) {
        const src = img.currentSrc || img.src || '';
        const rects = specs[fnv1a(src)];
        if (!rects) continue;
        const w = img.naturalWidth;
        const h = img.naturalHeight;
        if (!w || !h || !img.complete) {
            stats.missed++;
            continue;
        }
        try {
            const canvas = document.createElement('canvas');
            canvas.width = w;
            canvas.height = h;
            const ctx = canvas.getContext('2d');
            ctx.drawImage(img, 0, 0);
            if (opts.grayscale) {
                // Explicit luminance loop, NOT ctx.filter='grayscale(1)':
                // the canvas filter is silently flaky in headless Chrome —
                // observed applying to one image and not the next within a
                // single run. Pixels can't lie.
                const id = ctx.getImageData(0, 0, w, h);
                const d = id.data;
                for (let i = 0; i < d.length; i += 4) {
                    const g = (d[i] * 0.299 + d[i + 1] * 0.587 + d[i + 2] * 0.114) | 0;
                    d[i] = d[i + 1] = d[i + 2] = g;
                }
                ctx.putImageData(id, 0, 0);
            }
            for (const [fx, fy, fw, fh, term] of rects) {
                // A little breathing room so tight OCR boxes don't clip
                // ascenders/descenders.
                const px = fh * h * 0.15;
                const x = fx * w - px, y = fy * h - px;
                const rw = fw * w + 2 * px, rh = fh * h + 2 * px;
                const fill = (opts.colors && opts.colors[term | 0]) || opts.color;
                if (opts.style === 'box') {
                    // Outline only — the text stays unobscured.
                    ctx.strokeStyle = fill;
                    ctx.lineWidth = Math.max(2, h / 300);
                    ctx.strokeRect(x, y, rw, rh);
                } else {
                    ctx.fillStyle = fill;
                    ctx.fillRect(x, y, rw, rh);
                    if (opts.stroke) {
                        ctx.strokeStyle = opts.stroke;
                        ctx.lineWidth = Math.max(1, h / 400);
                        ctx.strokeRect(x, y, rw, rh);
                    }
                }
            }
            const mime = src.startsWith('data:image/jpeg') ? 'image/jpeg' : 'image/png';
            img.src = canvas.toDataURL(mime, 0.92);
            stats.marked++;
        } catch (e) {
            stats.missed++;
        }
    }
    return stats;
}
