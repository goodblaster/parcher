// Collects the URLs of every CSS background image in use, including on
// ::before/::after pseudo-elements (a very common place for decorative
// images and icons). Traverses open shadow roots. Paired with
// inject_background_images.js.
async function extractBackgroundURLs() {
    const urls = new Set();

    const abs = (url) => {
        try {
            return new URL(url, document.baseURI).href;
        } catch (e) {
            return url;
        }
    };

    const urlRe = /url\(\s*(['"]?)([^'")]+)\1\s*\)/g;

    const collectFrom = (cssValue) => {
        if (!cssValue || cssValue === 'none') return;
        for (const m of cssValue.matchAll(urlRe)) {
            const url = m[2];
            if (!url.startsWith('data:') && !url.startsWith('blob:')) {
                urls.add(abs(url));
            }
        }
    };

    function walk(root) {
        for (const el of root.querySelectorAll('*')) {
            for (const pseudo of [null, '::before', '::after']) {
                collectFrom(window.getComputedStyle(el, pseudo).backgroundImage);
            }
            if (el.shadowRoot) walk(el.shadowRoot);
        }
    }
    walk(document);

    return Array.from(urls);
}
