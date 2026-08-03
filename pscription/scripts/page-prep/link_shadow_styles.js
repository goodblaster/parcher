// Re-expresses each shadow root's CSS as a <link> to a data: URL, so that a
// DOM serializer preserves it.
//
// Chrome's MHTML writer serializes shadow roots — open and closed — but DROPS
// <style> elements inside them, and constructed sheets were never markup to
// begin with. The result archives with the shadow content present and
// completely unstyled. Measured on a probe page: A (a <style> child) and C
// (adoptedStyleSheets) both came back bare, while B (a <link rel=stylesheet>)
// survived intact inside the serialized <template>. So the fix is to give
// every shadow root the form that survives.
//
// A data: URL is used rather than a real one because it needs no part in the
// container and no rewriting on the way out: it is already self-contained.
//
// The original <style> children are deliberately KEPT. Removing them would
// leave the live page depending on a stylesheet that has to load, and the
// backends that photograph the page (pdf/png/jpeg/webp) could catch it
// mid-flash. Duplicate identical declarations cost a little size and change
// nothing about rendering.
//
// Closed shadow roots cannot be reached from page script, so their styling is
// still lost. Their CONTENT survives, which is already more than a
// page-script serializer can manage.
(() => {
    let roots = 0;
    let sheets = 0;
    let unreadable = 0;

    const cssOf = (sheet) => {
        try {
            let css = '';
            for (const rule of sheet.cssRules) css += rule.cssText + '\n';
            return css;
        } catch (e) {
            // A sheet from a cross-origin URL is unreadable through the
            // CSSOM. If it came from a <link> it is already in the surviving
            // form and needs nothing from us.
            unreadable++;
            return '';
        }
    };

    const linkStyles = (root) => {
        let css = '';
        for (const style of root.querySelectorAll('style')) {
            if (!style.sheet) continue;
            css += cssOf(style.sheet);
            sheets++;
        }
        for (const sheet of root.adoptedStyleSheets || []) {
            css += cssOf(sheet);
            sheets++;
        }
        if (!css.trim()) return;

        const link = document.createElement('link');
        link.rel = 'stylesheet';
        link.setAttribute('data-pscription-shadow', '');
        link.href = 'data:text/css;charset=utf-8,' + encodeURIComponent(css);
        root.insertBefore(link, root.firstChild);
        roots++;
    };

    const walk = (node) => {
        for (const el of node.querySelectorAll('*')) {
            if (!el.shadowRoot) continue;
            if (!el.shadowRoot.querySelector('link[data-pscription-shadow]')) {
                linkStyles(el.shadowRoot);
            }
            walk(el.shadowRoot);
        }
    };
    walk(document);

    return { roots, sheets, unreadable };
})
