// Replaces the canvas marked data-gb-canvas-shot=<id> with an <img> whose
// src is the given data URL. Companion to rasterize_canvases.js: the Go
// action captures a CDP element screenshot of canvases whose pixels page
// JS cannot read (tainted, OffscreenCanvas) and injects the result here.
function replaceCanvasWithImage(id, dataURL) {
    const canvas = document.querySelector('canvas[data-gb-canvas-shot="' + id + '"]');
    if (!canvas || !canvas.parentNode) return { replaced: false };

    const img = document.createElement('img');
    img.src = dataURL;
    img.alt = '';
    img.style.cssText = canvas.style.cssText;
    // Keep the page's own CSS applying to the replacement.
    if (canvas.id) img.id = canvas.id;
    const c = canvas.getAttribute('class');
    if (c) img.setAttribute('class', c);

    if (canvas.hasAttribute('width')) img.setAttribute('width', canvas.getAttribute('width'));
    if (canvas.hasAttribute('height')) img.setAttribute('height', canvas.getAttribute('height'));
    if (!img.hasAttribute('width') && !img.hasAttribute('height')) {
        const rect = canvas.getBoundingClientRect();
        if (rect.width > 0 && rect.height > 0) {
            img.style.width = rect.width + 'px';
            img.style.height = rect.height + 'px';
        }
    }

    canvas.parentNode.replaceChild(img, canvas);
    return { replaced: true };
}
