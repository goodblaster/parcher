// Collects the @font-face rules whose fonts the page ACTUALLY loaded, with
// their descriptors and external URLs. Chrome's MHTML snapshot omits CSS web
// fonts, so we fetch these server-side and re-inject them as data: URIs (see
// fetch_fonts.go) — otherwise MHT archives fall back to system fonts and
// icon fonts render as tofu boxes.
//
// Critically, it filters to fonts that loaded. Sites often declare hundreds
// of subsetted @font-face rules (each family × weight × language range);
// the browser only fetches the subsets it renders, so embedding every
// declared rule would bloat the archive many-fold. We cross-reference
// document.fonts (the FontFaceSet) and keep only rules matching a loaded
// entry.
//
// Cross-origin stylesheets whose rules can't be read are skipped. Returns an
// array of { family, style, weight, stretch, unicodeRange, display, urls }.
async function extractFontFaces() {
    // Wait for font loading to settle so status is accurate.
    try {
        await document.fonts.ready;
    } catch (e) { /* ignore */ }

    const norm = (s) => (s || '').replace(/["']/g, '').replace(/\s+/g, ' ').trim().toLowerCase();
    const FULL_RANGE = 'u+0-10ffff';
    const rangeKey = (r) => {
        const n = norm(r);
        return n === '' ? FULL_RANGE : n;
    };
    const key = (family, style, weight, range) =>
        norm(family) + '|' + norm(style || 'normal') + '|' + norm(weight || 'normal') + '|' + rangeKey(range);

    // Descriptor keys of fonts that actually loaded.
    const loaded = new Set();
    document.fonts.forEach((ff) => {
        if (ff.status === 'loaded') {
            loaded.add(key(ff.family, ff.style, ff.weight, ff.unicodeRange));
        }
    });

    const faces = [];
    for (const sheet of document.styleSheets) {
        let rules;
        try {
            rules = sheet.cssRules;
        } catch (e) {
            continue; // cross-origin
        }
        if (!rules) continue;

        const base = sheet.href || document.baseURI;
        for (const rule of rules) {
            if (rule.type !== 5 && rule.constructor.name !== 'CSSFontFaceRule') continue;
            const s = rule.style;
            const src = s.getPropertyValue('src');
            if (!src) continue;

            const family = s.getPropertyValue('font-family');
            const style = s.getPropertyValue('font-style') || 'normal';
            const weight = s.getPropertyValue('font-weight') || 'normal';
            const unicodeRange = s.getPropertyValue('unicode-range') || '';

            // Only embed if this exact face loaded.
            if (loaded.size > 0 && !loaded.has(key(family, style, weight, unicodeRange))) {
                continue;
            }

            const urls = [];
            const re = /url\(\s*(['"]?)([^'")]+)\1\s*\)/g;
            let m;
            while ((m = re.exec(src)) !== null) {
                if (!m[2].startsWith('data:')) {
                    try {
                        urls.push(new URL(m[2], base).href);
                    } catch (e) { /* skip */ }
                }
            }
            if (urls.length === 0) continue;

            faces.push({
                family,
                style,
                weight,
                stretch: s.getPropertyValue('font-stretch') || '',
                unicodeRange,
                display: s.getPropertyValue('font-display') || '',
                urls,
            });
        }
    }

    return faces;
}
