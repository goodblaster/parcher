// Forces eager loading of every image on the page, defeating the common
// lazy-loading patterns:
//   - native loading="lazy"
//   - data-src / data-srcset style deferred attributes (img, source, video)
//   - data-bg / data-background deferred CSS backgrounds
//   - lazyload marker classes
// Traverses open shadow roots. Safe to run repeatedly.
//
// IntersectionObserver-based lazy loaders that ignore these attributes are
// handled separately by scroll_and_wait_images.js, which actually scrolls
// the viewport through the page.
(() => {
    let processed = 0;

    function collect(root) {
        const found = Array.from(root.querySelectorAll(
            'img, picture source, video, [data-src], [data-lazy-src], [data-bg], [data-background], [data-background-image]'));
        for (const el of root.querySelectorAll('*')) {
            if (el.shadowRoot) {
                found.push(...collect(el.shadowRoot));
            }
        }
        return found;
    }

    const srcAttrs = ['data-src', 'data-lazy-src', 'data-original', 'data-lazy', 'data-fallback-src', 'data-hi-res-src'];
    const srcsetAttrs = ['data-srcset', 'data-lazy-srcset', 'data-original-srcset'];
    const bgAttrs = ['data-background', 'data-bg', 'data-background-image'];
    const lazyClasses = ['lazy', 'lazyload', 'lazyloading', 'lazy-image'];

    for (const el of collect(document)) {
        const tag = el.tagName.toLowerCase();

        if (tag === 'img') {
            if (el.loading === 'lazy') {
                el.loading = 'eager';
                processed++;
            }
            for (const attr of srcsetAttrs) {
                const v = el.getAttribute(attr);
                if (v) {
                    el.srcset = v;
                    el.removeAttribute(attr);
                    processed++;
                }
            }
            for (const attr of srcAttrs) {
                const v = el.getAttribute(attr);
                if (v) {
                    el.src = v;
                    el.removeAttribute(attr);
                    processed++;
                }
            }
            for (const c of lazyClasses) {
                el.classList.remove(c);
            }
        } else if (tag === 'source') {
            const v = el.getAttribute('data-srcset') || el.getAttribute('data-src');
            if (v) {
                el.srcset = v;
                el.removeAttribute('data-srcset');
                el.removeAttribute('data-src');
                processed++;
            }
        } else if (tag === 'video') {
            if (el.preload !== 'auto') {
                el.preload = 'auto';
            }
            const v = el.getAttribute('data-src');
            if (v) {
                el.src = v;
                el.removeAttribute('data-src');
                processed++;
            }
        }

        for (const attr of bgAttrs) {
            const v = el.getAttribute && el.getAttribute(attr);
            if (v) {
                el.style.backgroundImage = `url("${v}")`;
                el.removeAttribute(attr);
                processed++;
            }
        }
    }

    return { processed };
})
