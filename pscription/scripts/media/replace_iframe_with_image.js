// Replaces the iframe marked data-gb-frame-shot=<id> with an <img> whose
// src is the given data URL — the frame's own pixels, screenshotted while
// it was composited. Cross-origin iframe content can't be serialized into
// a DOM archive at all, so a still of what the user saw is the most
// faithful representation available. Companion to locate_opaque_iframes.js.
function replaceIframeWithImage(id, dataURL) {
    function find(root) {
        const hit = root.querySelector('iframe[data-gb-frame-shot="' + id + '"]');
        if (hit) return hit;
        for (const f of root.querySelectorAll('iframe')) {
            try {
                if (f.contentDocument) {
                    const r = find(f.contentDocument);
                    if (r) return r;
                }
            } catch (e) { /* cross-origin */ }
        }
        for (const host of root.querySelectorAll('*')) {
            if (host.shadowRoot) {
                const r = find(host.shadowRoot);
                if (r) return r;
            }
        }
        return null;
    }

    const frame = find(document);
    if (!frame || !frame.parentNode) return { replaced: false };

    const img = frame.ownerDocument.createElement('img');
    img.src = dataURL;
    img.alt = '';
    img.style.cssText = frame.style.cssText;
    if (frame.id) img.id = frame.id;
    const c = frame.getAttribute('class');
    if (c) img.setAttribute('class', c);

    if (frame.hasAttribute('width')) img.setAttribute('width', frame.getAttribute('width'));
    if (frame.hasAttribute('height')) img.setAttribute('height', frame.getAttribute('height'));
    if (!img.hasAttribute('width') && !img.hasAttribute('height')) {
        const rect = frame.getBoundingClientRect();
        if (rect.width > 0 && rect.height > 0) {
            img.style.width = rect.width + 'px';
            img.style.height = rect.height + 'px';
        }
    }

    frame.parentNode.replaceChild(img, frame);
    return { replaced: true };
}
