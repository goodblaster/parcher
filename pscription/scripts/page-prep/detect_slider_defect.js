// Reports whether the prepared page is in a known-broken state that warrants
// re-capturing: a slider/carousel library is present (loaded, or referenced
// by a script/link the page pulled in) but has not initialized any element.
//
// When that happens the slider's slides render stacked vertically instead of
// as a compact carousel — an intermittent failure (the library's init
// sometimes just doesn't run in headless) that no amount of waiting fixes.
// The capture layer uses this to retry.
//
// Returns { defect: bool, lib?: string, signature: string }.
//
// The signature summarises the observable state the verdict was drawn from.
// A caller re-capturing on a defect can compare signatures between attempts:
// identical evidence means the failure is NOT transient, so retrying cannot
// help and only costs a full extra pass.
(() => {
    const referenced = (needle) =>
        !!document.querySelector(
            `script[src*="${needle}" i], link[href*="${needle}" i]`);

    const jq = window.jQuery;
    const libs = [
        { loaded: !!(jq && jq.fn && jq.fn.slick), ref: 'slick',
          marker: '.slick-initialized', candidates: '[class*="slick-"]' },
        { loaded: !!(jq && jq.fn && jq.fn.owlCarousel), ref: 'owl.carousel',
          marker: '.owl-loaded', candidates: '[class*="owl-"]' },
        { loaded: !!window.Swiper, ref: 'swiper',
          marker: '.swiper-initialized', candidates: '[class*="swiper"]' },
    ];

    // Height is part of the evidence because the defect this detects IS a
    // layout one: un-initialised slides stack instead of collapsing.
    const height = document.documentElement.scrollHeight;

    for (const l of libs) {
        const candidates = document.querySelectorAll(l.candidates).length;
        const initialized = document.querySelectorAll(l.marker).length;
        if (!(l.loaded || referenced(l.ref)) || initialized > 0) continue;

        // A library with nothing to initialise is not a broken page. Plugin
        // bundles ship carousel libraries unconditionally, so "referenced but
        // no marker" is unfalsifiable on its own — without elements it would
        // manage, there is no evidence of breakage to act on.
        if (candidates === 0) {
            return { defect: false, lib: l.ref, unused: true,
                     signature: l.ref + ':unused:' + height };
        }
        return { defect: true, lib: l.ref,
                 signature: l.ref + ':' + candidates + '/' + initialized + ':' + height };
    }
    return { defect: false, signature: 'ok:' + height };
})
