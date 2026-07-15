// Returns the page's declared languages (html lang attribute, then the
// Content-Language meta), de-duplicated, falling back to en-US when the
// page declares nothing. Empty declarations are ignored so the fallback
// actually fires.
(() => {
    let langs = [];

    const htmlLang = (document.documentElement.lang || '').trim();
    if (htmlLang) {
        langs.push(htmlLang);
    }

    const meta = document.querySelector('meta[http-equiv="Content-Language" i]');
    if (meta) {
        const content = (meta.getAttribute('content') || '').trim();
        if (content) {
            // May be a comma-separated list, e.g. "en, fr".
            for (const lang of content.split(',')) {
                if (lang.trim()) {
                    langs.push(lang.trim());
                }
            }
        }
    }

    langs = Array.from(new Set(langs));
    if (langs.length === 0) {
        langs.push('en-US');
    }

    return langs;
})
