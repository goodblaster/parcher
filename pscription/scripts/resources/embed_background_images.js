async function inlineBackgroundImages() {
    // Helper to convert URL to data URL
    async function fetchAsDataURL(url) {
        try {
            const response = await fetch(url, {
                credentials: 'include',
                mode: 'cors'
            });
            if (!response.ok) {
                console.warn(`Failed to fetch background image: ${url} (${response.status})`);
                return null;
            }
            const blob = await response.blob();
            return new Promise((resolve) => {
                const reader = new FileReader();
                reader.onloadend = () => resolve(reader.result);
                reader.readAsDataURL(blob);
            });
        } catch (error) {
            console.warn(`Error fetching background image: ${url}`, error);
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

    // Extract URLs from CSS url() values
    function extractURLs(cssValue) {
        const urls = [];
        const urlRegex = /url\(\s*(['"]?)([^'")]+)\1\s*\)/g;
        let match;
        while ((match = urlRegex.exec(cssValue)) !== null) {
            urls.push(match[2]);
        }
        return urls;
    }

    // Collect elements from both regular DOM and Shadow DOM
    function collectElements(root = document) {
        const elements = [];

        // Get all elements from current root
        if (root.querySelectorAll) {
            elements.push(...Array.from(root.querySelectorAll('*')));
        }

        // Traverse shadow roots
        const elementsWithShadow = root.querySelectorAll ?
            Array.from(root.querySelectorAll('*')) : [];
        for (const element of elementsWithShadow) {
            if (element.shadowRoot) {
                elements.push(...collectElements(element.shadowRoot));
            }
        }

        return elements;
    }

    // Process all elements from both DOM and Shadow DOM
    const elements = collectElements();
    const concurrency = 8;
    let processed = 0;
    let inlined = 0;

    async function processElement(el) {
        const style = window.getComputedStyle(el);
        const bgImage = style.backgroundImage;

        if (!bgImage || bgImage === 'none') {
            return;
        }

        const urls = extractURLs(bgImage);
        if (urls.length === 0) {
            return;
        }

        let newBgImage = bgImage;
        let changed = false;

        for (const url of urls) {
            // Skip if already a data URL
            if (url.startsWith('data:')) {
                continue;
            }

            const absoluteURL = getAbsoluteURL(url);
            const dataURL = await fetchAsDataURL(absoluteURL);

            if (dataURL) {
                // Replace the URL in the background-image string
                newBgImage = newBgImage.replace(url, dataURL);
                changed = true;
                inlined++;
            }
        }

        if (changed) {
            // Set inline style (will override most stylesheet rules)
            el.style.setProperty('background-image', newBgImage, 'important');
        }

        processed++;
    }

    // Process elements with concurrency limit
    const elementArray = Array.from(elements).filter(el => {
        const bgImage = window.getComputedStyle(el).backgroundImage;
        return bgImage && bgImage !== 'none';
    });

    const chunks = [];
    for (let i = 0; i < elementArray.length; i += concurrency) {
        chunks.push(elementArray.slice(i, i + concurrency));
    }

    for (const chunk of chunks) {
        await Promise.all(chunk.map(processElement));
    }

    console.log(`Inlined ${inlined} background images from ${processed} elements`);
    return { total: elementArray.length, processed, inlined };
}
