// Converts the element found by the CSS selector (an inline <svg> or an
// <img> pointing at an .svg) into a base64 JPEG data URL. JPEG has no alpha,
// so the canvas gets a white background first.
// Returns the data URL, or a string starting with "ERROR:".
async function convertSvgToJpg(selector) {
    const elem = document.querySelector(selector);
    if (!elem) {
        return 'ERROR: Element not found';
    }

    const canvas = document.createElement('canvas');
    const ctx = canvas.getContext('2d');
    const tag = elem.tagName.toLowerCase();

    const whiteBackground = () => {
        ctx.fillStyle = '#ffffff';
        ctx.fillRect(0, 0, canvas.width, canvas.height);
    };

    try {
        if (tag === 'img') {
            canvas.width = elem.naturalWidth || elem.width || 500;
            canvas.height = elem.naturalHeight || elem.height || 500;
            if (!elem.complete) {
                await elem.decode();
            }
            whiteBackground();
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
            whiteBackground();
            ctx.drawImage(img, 0, 0, width, height);
        } else {
            return 'ERROR: Unsupported element type: ' + tag;
        }

        return canvas.toDataURL('image/jpeg', 0.9);
    } catch (e) {
        return 'ERROR: ' + e.message;
    }
}
