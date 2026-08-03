// Emits the DOCUMENT's constructed stylesheets as a real <style> element.
//
// `document.adoptedStyleSheets` holds CSSStyleSheet OBJECTS that no element
// carries, so every DOM serializer drops them — measured: Chrome's MHTML
// writer and the vendored serializer it replaced both lost one, and the archive
// rendered unstyled exactly where a web component had put its CSS. It is not
// a quirk of one serializer; nothing that walks markup can see CSSOM-only
// state. Flattening it into markup fixes every backend at once.
//
// Adding a <style> whose rules are already in effect changes no layout: the
// same declarations apply, at the same specificity, later in the cascade.
//
// Shadow roots are NOT handled here — their adopted sheets go through
// link_shadow_styles.js, because a <style> inside a shadow root does not
// survive serialization in the first place.
(() => {
    const adopted = document.adoptedStyleSheets;
    if (!adopted || adopted.length === 0) return { sheets: 0, rules: 0 };

    let sheets = 0;
    let rules = 0;
    let css = '';

    for (const sheet of adopted) {
        try {
            for (const rule of sheet.cssRules) {
                css += rule.cssText + '\n';
                rules++;
            }
            sheets++;
        } catch (e) {
            // A sheet constructed from a cross-origin source throws on
            // cssRules; nothing to emit, and not worth failing over.
        }
    }
    if (!css) return { sheets, rules };

    const style = document.createElement('style');
    style.setAttribute('data-pscription-adopted', '');
    style.textContent = css;
    (document.head || document.documentElement).appendChild(style);

    return { sheets, rules };
})
