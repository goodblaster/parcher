// Measure only after preparation has settled. Targets are device pixels, not
// CSS layout dimensions. Keep the measurements even when preserving originals
// so a later, browser-free OCR pass can perform the same optimization.
async function sizeImages(replacements) {
    const sizeAttr = 'data-parch-image-size';
    const densityAttr = 'data-parch-image-density';
    if (replacements) {
        let changed = 0;
        const pending = [];
        for (const item of window.__parchImageSizes || []) {
            const replacement = replacements[item.src];
            if (!replacement) continue;
            const el = item.el;
            if (item.background) {
                const decoded = new Image();
                decoded.src = replacement.src;
                pending.push(decoded.decode().catch(() => {}));
                el.style.setProperty('background-image', 'url("' + replacement.src + '")', 'important');
            } else {
                // Density preserves intrinsic CSS size without pinning a
                // responsive element's rendered width or height.
                const density = replacement.width / item.natural;
                el.setAttribute(densityAttr, String(density));
                if (el.parentElement?.tagName === 'PICTURE') {
                    for (const source of el.parentElement.querySelectorAll('source')) source.remove();
                }
                el.removeAttribute('sizes');
                el.srcset = replacement.src + ' ' + density + 'x';
                // A src fallback is an implicit 1x candidate and would win
                // over a sub-1x density. Materialize it only for a snapshot,
                // AFTER text extraction and only in document backends.
                el.setAttribute('src', '');
                pending.push(el.decode().catch(() => {}));
            }
            changed++;
        }
        await Promise.all(pending);
        delete window.__parchImageSizes;
        return {changed};
    }
    const targets = {};
    const items = [];
    function add(el, src, width, height, natural, background) {
        if (!src.startsWith('data:image/')) return;
        const previous = targets[src];
        // A hidden/unknown use prevents reducing a shared source: it may be
        // revealed later, at a size we cannot measure now.
        const blocked = width <= 0 || height <= 0;
        if (!previous) targets[src] = {width, height, blocked};
        else {
            previous.width = Math.max(previous.width, width);
            previous.height = Math.max(previous.height, height);
            previous.blocked ||= blocked;
        }
        el.setAttribute(background ? 'data-parch-background-size' : sizeAttr,
            [width, height, natural].join(','));
        if (blocked) return;
        items.push({el, src, natural, background});
    }
    function walk(root) {
        const win = root.ownerDocument?.defaultView || root.defaultView;
        const dpr = win.devicePixelRatio || 1;
        for (const el of root.querySelectorAll('*')) {
            const style = win.getComputedStyle(el);
            const rect = el.getBoundingClientRect();
            const visible = rect.width > 0 && rect.height > 0 && el.checkVisibility({checkOpacity:true, checkVisibilityCSS:true});
            if (el.tagName === 'IMG') {
                el.removeAttribute(sizeAttr);
                const src = el.currentSrc || el.src || '';
                let width = rect.width, height = rect.height;
                const nw = el.naturalWidth, nh = el.naturalHeight;
                // Use the painted area for contain/scale-down; cover/fill need
                // enough pixels on BOTH axes. object-fit:none keeps originals.
                if (nw && nh && (style.objectFit === 'contain' || style.objectFit === 'scale-down')) {
                    let scale = Math.min(width/nw, height/nh);
                    if (style.objectFit === 'scale-down') scale = Math.min(1, scale);
                    width = nw*scale; height = nh*scale;
                }
                if (!visible || !nw || style.objectFit === 'none') width = height = 0;
                add(el, src, Math.ceil(width*dpr), Math.ceil(height*dpr), nw, false);
            }
            // Only single-layer, explicitly scaled backgrounds: auto-sized
            // tiles/sprites and pseudo-elements lack a safe measured target.
            el.removeAttribute('data-parch-background-size');
            const bg = /^url\("(data:image\/[^" ]+)"\)$/.exec(style.backgroundImage);
            if (bg && visible && (style.backgroundSize === 'cover' || style.backgroundSize === 'contain')) {
                // The full box is conservative for contain and non-square art.
                add(el, bg[1], Math.ceil(rect.width*dpr), Math.ceil(rect.height*dpr), 0, true);
            }
            if (el.shadowRoot) walk(el.shadowRoot);
            if (el.tagName === 'IFRAME') {
                try { if (el.contentDocument) walk(el.contentDocument); } catch (_) {}
            }
        }
    }
    walk(document);
    window.__parchImageSizes = items;
    for (const [src, target] of Object.entries(targets)) {
        if (target.blocked) { target.width = 0; target.height = 0; }
    }
    return targets;
}
