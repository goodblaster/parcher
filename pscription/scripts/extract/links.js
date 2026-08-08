// Collects the page's links as absolute URLs with their anchor text.
// Same-page fragments, mailto:/tel:/javascript: are skipped. Deduplicated by
// URL. This is the extraction half of the "crawler without a crawler"
// pattern: `pscribe <index-url> extract/links.js -only links | xargs -n1 parch`.
//
// Returns an array of { url, text }.
(() => {
    const seen = new Set();
    const out = [];
    const pageBase = location.href.split('#')[0];

    for (const a of document.querySelectorAll('a[href]')) {
        const raw = a.getAttribute('href') || '';
        if (raw === '' || raw.startsWith('#')) continue;
        if (/^(mailto:|tel:|javascript:)/i.test(raw)) continue;

        let abs;
        try {
            abs = new URL(raw, location.href);
        } catch (e) {
            continue;
        }
        if (abs.protocol !== 'http:' && abs.protocol !== 'https:') continue;

        const key = abs.href.split('#')[0];
        if (key === pageBase || seen.has(key)) continue;
        seen.add(key);

        out.push({ url: abs.href, text: (a.textContent || '').replace(/\s+/g, ' ').trim() });
    }

    return out;
})
