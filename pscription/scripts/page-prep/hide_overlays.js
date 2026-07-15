// Injects CSS that permanently hides popup/overlay chrome that should never
// appear in a static snapshot: marketing popups, modal backdrops, and page
// preloaders. The CSS persists into the captured document, so anything it
// hides stays hidden offline.
//
// Deliberately NOT hidden here: age gates and cookie walls. Those often
// gate the real content behind them and must be clicked through instead
// (see dismiss_popups.js).
(() => {
    if (document.getElementById('gb-hide-overlays')) {
        return { injected: false };
    }

    const style = document.createElement('style');
    style.id = 'gb-hide-overlays';
    style.textContent = `
        /* Marketing popup frameworks (OptinMonster etc.) */
        [class*="optinmonster" i],
        [id*="optinmonster" i],
        [class*="om-position" i],
        [class*="om-element" i],
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
