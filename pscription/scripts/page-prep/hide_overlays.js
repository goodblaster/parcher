// Injects CSS that permanently hides popup/overlay chrome that should never
// appear in a static snapshot: marketing popups, modal backdrops, and page
// preloaders. The CSS persists into the captured document, so anything it
// hides stays hidden offline.
//
// Deliberately NOT hidden here: age gates and cookie walls. Those often
// gate the real content behind them and must be clicked through instead
// (see dismiss_popups.js).
//
// SELECTOR RULE: scoped, never substring-matched on a short prefix. "om-" is
// two characters and belongs to nobody: [class*="om-element" i] also matches
// custom-element and zoom-element, and — measured on test-site-2 —
// a video plugin's own "keywest-element keywest-ele-1 Element om-element",
// four of them, which hid the site's hero video AND every review thumbnail.
// The page archived with two large blank bands and no error anywhere.
//
// So: only markup that identifies itself as OptinMonster, plus om-*
// descendants of such a root, which is where a campaign's own elements live.
// A popup that slips through is not lost — it is still handled at capture
// time by dismiss-popups and, failing that, remove-blocking-overlays.
// Under-hiding here is cheap; over-hiding destroys content silently.
//
// Keep this rationale OUT of the CSS template literal below. A backtick in
// prose ends the literal and the whole script dies with a SyntaxError — and
// because this step is stepIgnoreError, that failure is logged and swallowed,
// so the page silently gets NO overlay hiding at all.
(() => {
    if (document.getElementById('gb-hide-overlays')) {
        return { injected: false };
    }

    const style = document.createElement('style');
    style.id = 'gb-hide-overlays';
    style.textContent = `
        /* Marketing popup frameworks (OptinMonster etc.) — scoped
           deliberately; see the SELECTOR RULE note above before widening. */
        [class*="optinmonster" i],
        [id*="optinmonster" i],
        [class*="optinmonster" i] [class^="om-" i],
        [id*="optinmonster" i] [class^="om-" i],
        .om-popup,
        .om-overlay,

        /* Modal backdrops (the dimming layer, not the dialog content) */
        .modal-backdrop,
        .popup-backdrop,
        .overlay-backdrop,
        .modal-overlay,
        .popup-overlay,
        .modal-mask,

        /* Page preloaders / loading screens */
        #preloader,
        .preloader,
        .page-preloader,
        .page-loader,
        .loading-overlay,
        .loader-overlay {
            display: none !important;
            visibility: hidden !important;
            opacity: 0 !important;
            pointer-events: none !important;
        }
    `;
    (document.head || document.documentElement).appendChild(style);

    return { injected: true };
})
