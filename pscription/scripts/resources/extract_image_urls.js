// Collects the URLs of every image-like resource that the server-side
// fetcher should download for embedding:
//   - <img> (currentSrc, i.e. the srcset candidate actually in use)
//   - <video poster="...">
//   - <input type="image">
// Traverses open shadow roots. Data URLs are skipped. Paired with
// inject_images.js, which swaps these URLs for the fetched data URLs.
async function extractImageURLs() {
    const urls = new Set();

    const abs = (url) => {
        try {
            return new URL(url, document.baseURI).href;
        } catch (e) {
            return url;
        }
    };

    const add = (url) => {
        if (url && !url.startsWith('data:') && !url.startsWith('blob:')) {
            urls.add(abs(url));
        }
    };

    function walk(root) {
        for (const img of root.querySelectorAll('img')) {
            add(img.currentSrc || img.src);
        }
        for (const video of root.querySelectorAll('video[poster]')) {
            add(video.getAttribute('poster'));
        }
        for (const input of root.querySelectorAll('input[type="image"]')) {
            add(input.src);
        }
        for (const el of root.querySelectorAll('*')) {
            if (el.shadowRoot) walk(el.shadowRoot);
        }
    }
    walk(document);

    return Array.from(urls);
}
