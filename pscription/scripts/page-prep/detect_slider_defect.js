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

    // Stacked carousels, found by SHAPE rather than by library.
    //
    // The library checks below cannot see a total init failure. Their
    // evidence that a page has a carousel at all is a class the library
    // ADDS when it initializes — `slick-slide`, `slick-track` — so when
    // slick never runs there are no slick-* elements whatsoever and the
    // page is indistinguishable from one that has no carousel. Measured on
    // test-site-2, whose review carousel is slick bundled inside
    // an Elementor add-on (no slick script URL, so `referenced` is false
    // too): the broken capture had .slick-initialized=0, .slick-slide=0,
    // and reported `unused: true` — no defect, no retry, and an archive
    // with 11.4k px of stacked slides where a 796px carousel belonged.
    //
    // The layout tells the truth in a way the class names do not: a
    // container that should hold one slide at a time is instead as tall as
    // all of them. That is measurable without knowing whose carousel it is.
    const CAROUSEL_SELECTOR = [
        'carousel', 'slider', 'slick', 'swiper', 'owl-', 'splide', 'glide', 'flickity',
    ].map((n) => `[class*="${n}" i]`).join(',');

    const stacked = (minRatio) => {
        const viewportH = window.innerHeight || 900;
        const limit = viewportH * (minRatio || 3);
        const hits = [];
        for (const el of document.querySelectorAll(CAROUSEL_SELECTOR)) {
            // Needs several children to be "stacked" at all, and a height
            // no working carousel would have. The good state on the
            // reference page measures 0.9x the viewport; the broken one
            // 13x, so the threshold is nowhere near either edge.
            if (el.children.length < 3) continue;
            if (el.getBoundingClientRect().height < limit) continue;
            hits.push(el);
        }
        // One broken carousel must count once: drop any hit that merely
        // contains another, keeping the innermost.
        return hits.filter((el) => !hits.some((other) => other !== el && el.contains(other)));
    };

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

    // Shape first: it is direct evidence of the defect, and it catches the
    // libraries the checks below cannot name.
    const tall = stacked();
    if (tall.length > 0) {
        const worst = Math.round(Math.max(...tall.map((el) => el.getBoundingClientRect().height)));
        return {
            defect: true, lib: 'stacked', stacked: tall.length,
            signature: 'stacked:' + tall.length + ':' + worst + ':' + height,
        };
    }

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
