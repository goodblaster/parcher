// Snapshot-only repair: Chrome drops srcset without materializing currentSrc.
// Archives store our reduced source just once in srcset; re-snapshotting one
// needs a temporary src for Chrome to carry into the file.
function materializeImageSources() {
    let changed = 0;
    function walk(root) {
        for (const el of root.querySelectorAll('*')) {
            if (el.tagName === 'IMG' && el.hasAttribute('data-parch-image-density') && el.currentSrc) {
                el.src = el.currentSrc;
                changed++;
            }
            if (el.shadowRoot) walk(el.shadowRoot);
            if (el.tagName === 'IFRAME') {
                try { if (el.contentDocument) walk(el.contentDocument); } catch (_) {}
            }
        }
    }
    walk(document);
    return {changed};
}
