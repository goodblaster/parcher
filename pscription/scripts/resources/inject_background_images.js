// Swaps CSS background-image URLs for server-fetched data URLs.
//
// Element backgrounds are overridden with an inline style. Pseudo-element
// (::before/::after) backgrounds can't be styled inline, so affected
// elements get a data-gb-bg marker attribute and a generated stylesheet
// rule that targets it.
async function injectBackgroundImages(urlToDataURLMap) {
    let injected = 0;
    let processed = 0;
    let marker = 0;
    const pseudoRules = [];

    const abs = (url) => {
        try {
            return new URL(url, document.baseURI).href;
        } catch (e) {
            return url;
        }
    };

    const urlRe = /url\(\s*(['"]?)([^'")]+)\1\s*\)/g;

    // Returns the css value with every fetchable URL replaced, or null if
    // nothing changed.
    const rewrite = (cssValue) => {
        if (!cssValue || cssValue === 'none') return null;
        let changed = false;
        const out = cssValue.replace(urlRe, (whole, quote, url) => {
            if (url.startsWith('data:') || url.startsWith('blob:')) return whole;
            const dataURL = urlToDataURLMap[abs(url)];
            if (!dataURL) return whole;
            changed = true;
            injected++;
            return 'url("' + dataURL + '")';
        });
        return changed ? out : null;
    };

    function walk(root) {
        for (const el of root.querySelectorAll('*')) {
            processed++;

            const own = rewrite(window.getComputedStyle(el).backgroundImage);
            if (own) {
                el.style.setProperty('background-image', own, 'important');
            }

            for (const pseudo of ['::before', '::after']) {
                const value = rewrite(window.getComputedStyle(el, pseudo).backgroundImage);
                if (value) {
                    const id = 'gb' + (marker++);
                    el.setAttribute('data-gb-bg', id);
                    pseudoRules.push(
                        `[data-gb-bg="${id}"]${pseudo} { background-image: ${value} !important; }`);
                }
            }

            if (el.shadowRoot) walk(el.shadowRoot);
        }
    }
    walk(document);

    if (pseudoRules.length > 0) {
        const style = document.createElement('style');
        style.id = 'gb-bg-pseudo';
        style.textContent = pseudoRules.join('\n');
        (document.head || document.documentElement).appendChild(style);
    }

    return { injected, processed, pseudoRules: pseudoRules.length };
}
