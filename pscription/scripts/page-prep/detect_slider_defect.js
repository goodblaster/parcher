// Reports whether the prepared page is in a known-broken state that warrants
// re-capturing: a slider/carousel library is present (loaded, or referenced
// by a script/link the page pulled in) but has not initialized any element.
//
// When that happens the slider's slides render stacked vertically instead of
// as a compact carousel — an intermittent failure (the library's init
// sometimes just doesn't run in headless) that no amount of waiting fixes.
// The capture layer uses this to retry.
//
// Returns { defect: bool, lib?: string }.
(() => {
    const referenced = (needle) =>
        !!document.querySelector(
            `script[src*="${needle}" i], link[href*="${needle}" i]`);

    const jq = window.jQuery;
    const libs = [
        { loaded: !!(jq && jq.fn && jq.fn.slick), ref: 'slick', marker: '.slick-initialized' },
        { loaded: !!(jq && jq.fn && jq.fn.owlCarousel), ref: 'owl.carousel', marker: '.owl-loaded' },
        { loaded: !!window.Swiper, ref: 'swiper', marker: '.swiper-initialized' },
    ];

    for (const l of libs) {
        if ((l.loaded || referenced(l.ref)) && document.querySelectorAll(l.marker).length === 0) {
            return { defect: true, lib: l.ref };
        }
    }
    return { defect: false };
})
