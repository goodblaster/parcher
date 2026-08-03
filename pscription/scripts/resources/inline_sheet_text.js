// Inlines a stylesheet's text next to the <link> that loaded it, so the
// rules become readable through the CSSOM.
//
// The <style> goes immediately AFTER its <link>, never at the end of head:
// the cascade is document order, and moving a sheet to the end would let it
// win against rules that were meant to override it.
//
// The <link> is deliberately left in place. It is what the page declared,
// and a serializer that keeps it loses nothing — the inline copy is what
// carries the rules once there is no network.
((href, css) => {
    const link = document.querySelector('link[href="' + href.replace(/"/g, '\\"') + '"]');
    if (!link) return { inlined: false, reason: 'link not found' };
    if (link.nextElementSibling && link.nextElementSibling.hasAttribute('data-pscription-inlined')) {
        return { inlined: false, reason: 'already inlined' };
    }

    const style = document.createElement('style');
    style.setAttribute('data-pscription-inlined', href);
    if (link.media) style.media = link.media;
    style.textContent = css;
    link.parentNode.insertBefore(style, link.nextSibling);

    return { inlined: true };
})
