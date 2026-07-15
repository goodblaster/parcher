// Swaps image resource URLs for server-fetched data URLs (see
// extract_image_urls.js for what is collected).
//
// For <img> inside <picture>, the sibling <source> elements are removed —
// otherwise the browser keeps selecting the (external) source and the
// injected data URL never shows.
async function injectImages(urlToDataURLMap) {
    let injected = 0;
    let processed = 0;

    const abs = (url) => {
        try {
            return new URL(url, document.baseURI).href;
        } catch (e) {
            return url;
        }
    };

    const lookup = (url) => {
        if (!url || url.startsWith('data:') || url.startsWith('blob:')) return null;
        return urlToDataURLMap[abs(url)] || null;
    };

    function walk(root) {
        for (const img of root.querySelectorAll('img')) {
            processed++;
            const dataURL = lookup(img.currentSrc || img.src);
            if (!dataURL) continue;

            // Freeze the rendered size before the swap.
            if (!img.getAttribute('width') && img.naturalWidth) {
                img.width = img.naturalWidth;
            }
            if (!img.getAttribute('height') && img.naturalHeight) {
                img.height = img.naturalHeight;
            }

            if (img.parentElement && img.parentElement.tagName === 'PICTURE') {
                for (const source of img.parentElement.querySelectorAll('source')) {
                    source.remove();
                }
            }
            img.removeAttribute('srcset');
            img.removeAttribute('sizes');
            img.src = dataURL;
            injected++;
        }

        for (const video of root.querySelectorAll('video[poster]')) {
            processed++;
            const dataURL = lookup(video.getAttribute('poster'));
            if (dataURL) {
                video.setAttribute('poster', dataURL);
                injected++;
            }
        }

        for (const input of root.querySelectorAll('input[type="image"]')) {
            processed++;
            const dataURL = lookup(input.src);
            if (dataURL) {
                input.src = dataURL;
                injected++;
            }
        }

        for (const el of root.querySelectorAll('*')) {
            if (el.shadowRoot) walk(el.shadowRoot);
        }
    }
    walk(document);

    return { injected, processed };
}
