// Keeps elements that carry a `hidden` attribute IN the snapshot.
//
// Chrome's MHTML serializer skips every element with a `hidden` attribute —
// by attribute, whatever it renders as. Measured: a paragraph inside
// <div hidden class="shown"> with .shown[hidden]{display:block} is visible
// on the live page and absent from the archive; ARIA tab panels hidden with
// `hidden`, and hidden="until-found" sections the browser's own find-in-page
// would open, are not in the file at all.
//
// So, immediately before a snapshot, each such element trades the attribute
// for data-parchmint-hidden="<original value>" and has what the attribute
// was doing frozen inline: its computed display (none, or whatever the
// page's CSS overrode it to) and, for until-found, content-visibility. The
// page renders exactly as before; the serializer no longer sees `hidden`.
// The original style attribute is kept in data-parchmint-style so the HTML
// backend can put both back (render.restoreHidden); the MHT backend ships
// the data- form, which renders the same.
//
// Run ONLY by the snapshot backends, last thing before they serialize. The
// backends that photograph the page do not need it and must not get it: it
// would unhook the page's own [hidden] selectors for nothing.
// Synchronous on purpose — it also runs on a loaded archive, where page
// script execution is disabled and only plain CDP evaluation works.
(() => {
    const stats = { preserved: 0 };

    function visit(root) {
        for (const el of root.querySelectorAll('[hidden]')) {
            const value = el.getAttribute('hidden') || '';
            const style = getComputedStyle(el);
            const display = style.display;
            const contentVisibility = style.contentVisibility;
            if (el.hasAttribute('style')) el.setAttribute('data-parchmint-style', el.getAttribute('style'));
            el.setAttribute('data-parchmint-hidden', value);
            el.removeAttribute('hidden');
            el.style.setProperty('display', display, 'important');
            if (contentVisibility === 'hidden') {
                el.style.setProperty('content-visibility', 'hidden', 'important');
            }
            stats.preserved++;
        }
        for (const el of root.querySelectorAll('*')) {
            if (el.shadowRoot) visit(el.shadowRoot);
        }
    }

    visit(document);
    return stats;
})
