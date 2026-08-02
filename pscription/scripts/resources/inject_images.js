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

            // Freeze only what the swap can actually change — the bitmap's
            // size — and never by pinning pixels onto an axis the page
            // leaves `auto`.
            //
            // width/height attributes are PRESENTATIONAL HINTS: they apply
            // as CSS to any axis the page's own stylesheet does not set. A
            // responsive image (`width:100%`, height from `aspect-ratio` or
            // `auto`) therefore keeps a responsive width while its height
            // stays pinned at the capture viewport's value, and the picture
            // is stretched at every other width. Measured on test-site-3:
            // promo tiles captured at 453x604 rendered 340x604 in a 1200px
            // window instead of 340x453 — a 33% vertical stretch — because
            // `height="604"` outlived the layout it was measured in.
            //
            // Whatever is set must take both dimensions from ONE source:
            // filling in only the missing one from the bitmap's intrinsic
            // size while keeping the page's value for the other forces an
            // aspect ratio the image never had (a square avatar sized by
            // CSS to height=100 came out 200x100 — every round avatar an
            // oval).
            if (!img.getAttribute('width') || !img.getAttribute('height')) {
                const rect = img.getBoundingClientRect();
                const natW = img.naturalWidth;
                const natH = img.naturalHeight;
                const laidOut = rect.width >= 1 && rect.height >= 1;
                const intrinsicSized = natW && natH &&
                    Math.abs(rect.width - natW) <= 1 && Math.abs(rect.height - natH) <= 1;

                if (!laidOut) {
                    // display:none — no layout to preserve and no
                    // responsive axis to break; the bitmap is the only
                    // size information there is.
                    if (natW && natH) {
                        img.width = natW;
                        img.height = natH;
                    }
                } else if (intrinsicSized) {
                    // Laid out at the bitmap's own size, so nothing is
                    // constraining it: pixels reproduce exactly what the
                    // page was already doing, and a data URL that decodes
                    // to a different size can't resize the element.
                    img.width = natW;
                    img.height = natH;
                } else if (natW && natH &&
                           (img.ownerDocument.defaultView || window)
                               .getComputedStyle(img).aspectRatio === 'auto') {
                    // CSS drives the box but dictates no ratio, so an axis
                    // left `auto` still follows the bitmap. Keep the OLD
                    // bitmap's RATIO — not the capture viewport's pixels —
                    // so a data URL decoding to different dimensions than
                    // the srcset candidate it replaces cannot reflow the
                    // page. A page that sets its own aspect-ratio (the
                    // common responsive case) needs nothing from us.
                    img.style.setProperty('aspect-ratio', natW + ' / ' + natH);
                }
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
