// Replaces icon-font glyphs (Font Awesome and friends) with rendered <img>
// elements so they survive in archives that don't carry the icon font.
//
// Only elements that are genuinely icon glyphs are touched:
//   - empty elements (no text, no child images/svg)
//   - with ::before content that is a real string (not "none"/"normal")
//   - whose glyph is a Private Use Area codepoint — the defining trait of
//     icon fonts. Regular italic <i>text</i> is never affected.
(() => {
    let replaced = 0;
    let skipped = 0;

    const isPUA = (ch) => {
        const cp = ch.codePointAt(0);
        return (cp >= 0xE000 && cp <= 0xF8FF) ||   // BMP Private Use Area
            (cp >= 0xF0000 && cp <= 0xFFFFD) ||     // Plane 15
            (cp >= 0x100000 && cp <= 0x10FFFD);     // Plane 16
    };

    const glyphOf = (el) => {
        const content = window.getComputedStyle(el, '::before').content;
        if (!content || content === 'none' || content === 'normal') return null;
        // content is a quoted CSS string, e.g. "\f099"
        const m = content.match(/^["'](.+)["']$/s);
        if (!m || m[1].length === 0) return null;
        const glyph = m[1];
        if (![...glyph].every(isPUA)) return null;
        return glyph;
    };

    const rasterize = (el, glyph) => {
        const style = window.getComputedStyle(el, '::before');
        const rect = el.getBoundingClientRect();
        const fontSize = parseFloat(style.fontSize) || 16;
        const width = Math.ceil(rect.width) || Math.ceil(fontSize * 1.25);
        const height = Math.ceil(rect.height) || Math.ceil(fontSize * 1.25);
        const scale = window.devicePixelRatio || 1;

        const canvas = document.createElement('canvas');
        canvas.width = width * scale;
        canvas.height = height * scale;
        const ctx = canvas.getContext('2d');
        ctx.scale(scale, scale);
        ctx.font = style.font ||
            `${style.fontStyle} ${style.fontWeight} ${style.fontSize} ${style.fontFamily}`;
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillStyle = style.color || 'black';
        ctx.fillText(glyph, width / 2, height / 2);

        const img = document.createElement('img');
        img.src = canvas.toDataURL();
        img.alt = '';
        img.setAttribute('role', 'presentation');
        img.style.width = width + 'px';
        img.style.height = height + 'px';
        img.style.verticalAlign = 'middle';
        el.parentNode.replaceChild(img, el);
    };

    // <i> and <span> are where icon fonts live in practice.
    for (const el of document.querySelectorAll('i, span')) {
        if (el.childElementCount > 0) continue;
        if ((el.textContent || '').trim() !== '') continue;

        const glyph = glyphOf(el);
        if (!glyph) continue;

        try {
            rasterize(el, glyph);
            replaced++;
        } catch (e) {
            skipped++;
        }
    }

    return { replaced, skipped };
})
