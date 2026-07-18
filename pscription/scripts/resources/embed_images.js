async function embedImages() {
    // Helper to convert URL to data URL
    async function fetchAsDataURL(url) {
        try {
            const response = await fetch(url, {
                credentials: 'include',
                mode: 'cors'
            });
            if (!response.ok) {
                console.warn(`Failed to fetch image: ${url} (${response.status})`);
                return null;
            }
            const blob = await response.blob();
            return new Promise((resolve) => {
                const reader = new FileReader();
                reader.onloadend = () => resolve(reader.result);
                reader.readAsDataURL(blob);
            });
        } catch (error) {
            console.warn(`Error fetching image: ${url}`, error);
            return null;
        }
    }

    // Get absolute URL
    function getAbsoluteURL(url) {
        try {
            return new URL(url, document.baseURI).href;
        } catch {
            return url;
        }
    }

    // Collect images from both regular DOM and Shadow DOM
    function collectImages(root = document) {
        const images = [];

        // Get images from current root
        if (root.images) {
            images.push(...Array.from(root.images));
        } else if (root.querySelectorAll) {
            images.push(...Array.from(root.querySelectorAll('img')));
        }

        // Traverse shadow roots
        const elementsWithShadow = root.querySelectorAll('*');
        for (const element of elementsWithShadow) {
            if (element.shadowRoot) {
                images.push(...collectImages(element.shadowRoot));
            }
        }

        return images;
    }

    // Process all images from both DOM and Shadow DOM
    const images = collectImages();
    const concurrency = 8;
    let processed = 0;
    let inlined = 0;

    async function processImage(img) {
        // Skip if already a data URL
        const src = img.currentSrc || img.src;
        if (!src || src.startsWith('data:')) {
            return;
        }

        const absoluteURL = getAbsoluteURL(src);
        const dataURL = await fetchAsDataURL(absoluteURL);

        if (dataURL) {
            // Preserve dimensions
            if (!img.width && img.naturalWidth) {
                img.width = img.naturalWidth;
            }
            if (!img.height && img.naturalHeight) {
                img.height = img.naturalHeight;
            }

            // Inside <picture>, remove the <source> siblings — otherwise the
            // browser keeps selecting the external source over our data URL.
            if (img.parentElement && img.parentElement.tagName === 'PICTURE') {
                for (const source of img.parentElement.querySelectorAll('source')) {
                    source.remove();
                }
            }

            // Replace with data URL
            img.removeAttribute('srcset');
            img.removeAttribute('sizes');
            img.src = dataURL;
            inlined++;
        }
        processed++;
    }

    // Process images with concurrency limit
    const chunks = [];
    for (let i = 0; i < images.length; i += concurrency) {
        chunks.push(images.slice(i, i + concurrency));
    }

    for (const chunk of chunks) {
        await Promise.all(chunk.map(processImage));
    }

    console.log(`Inlined ${inlined} of ${processed} images`);
    return { total: images.length, inlined, processed };
}
