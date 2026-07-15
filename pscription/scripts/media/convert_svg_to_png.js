// Converts the element found by the CSS selector (an inline <svg> or an
// <img> pointing at an .svg) into a base64 PNG data URL.
// Async so inline-SVG rendering is actually awaited before the canvas is
// read — the previous version returned the canvas before the SVG image had
// decoded, yielding a blank PNG.
// Returns the data URL, or a string starting with "ERROR:".
async function convertSvgToPng(selector) {
    const elem = document.querySelector(selector);
    if (!elem) {
        return 'ERROR: Element not found';
    }

    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d');
    const tag = elem.tagName.toLowerCase();

    try {
        if (tag === 'img') {
            canvas.width = elem.naturalWidth || elem.width || 500;
            canvas.height = elem.naturalHeight || elem.height || 500;
            if (!elem.complete) {
                await elem.decode();
            }
            ctx.drawImage(elem, 0, 0, canvas.width, canvas.height);
        } else if (tag === 'svg') {
            const cloned = elem.cloneNode(true);
            const width = parseFloat(cloned.getAttribute('width')) || elem.clientWidth || 500;
            const height = parseFloat(cloned.getAttribute('height')) || elem.clientHeight || 500;
            canvas.width = width;
            canvas.height = height;
            cloned.setAttribute('width', width);
            cloned.setAttribute('height', height);
            if (!cloned.getAttribute('xmlns')) {
                cloned.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
            }

            const xml = new XMLSerializer().serializeToString(cloned);
            const img = new Image();
            img.src = 'data:image/svg+xml;base64,' +
                btoa(unescape(encodeURIComponent(xml)));
            await img.decode();
            ctx.drawImage(img, 0, 0, width, height);
        } else {
            return 'ERROR: Unsupported element type: ' + tag;
        }

        return canvas.toDataURL('image/png');
    } catch (e) {
        return 'ERROR: ' + e.message;
    }
}
