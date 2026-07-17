// Waits for JavaScript slider/carousel libraries to finish initializing.
// Before a slider inits, its slides render stacked vertically (a review
// carousel with a dozen video slides becomes ~12000px of stacked content
// with blank/void regions); after init they collapse to a compact
// one-at-a-time carousel. Init is async and fast, but the capture can race
// ahead of it and freeze the stacked state — an intermittent "huge white
// space / 3x-tall page" bug.
//
// wait_for_stable_layout can't catch this: the stacked state is stable
// (height doesn't change), so it looks settled. This keys on each library's
// actual initialized marker instead.
//
// A library counts as EXPECTED if it has loaded OR the page references it
// (script/link src) — the latter matters because the race is often that the
// library's own script hasn't executed yet, so a plain "is it loaded?" check
// would skip the wait and let the capture proceed too early. For each
// expected library we wait until it has loaded AND initialized at least one
// element, up to maxMs. A library that genuinely never initializes just
// times out.
async function waitForSliders(maxMs) {
    maxMs = maxMs || 8000;
    const start = Date.now();
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

    const referenced = (needle) =>
        !!document.querySelector(
            `script[src*="${needle}" i], link[href*="${needle}" i]`);

    const libs = [
        { name: 'slick', loaded: () => !!(window.jQuery && window.jQuery.fn && window.jQuery.fn.slick), ref: 'slick', marker: '.slick-initialized' },
        { name: 'owl', loaded: () => !!(window.jQuery && window.jQuery.fn && window.jQuery.fn.owlCarousel), ref: 'owl.carousel', marker: '.owl-loaded' },
        { name: 'swiper', loaded: () => !!window.Swiper, ref: 'swiper', marker: '.swiper-initialized' },
    ];

    const expected = libs.filter((l) => l.loaded() || referenced(l.ref));
    if (expected.length === 0) {
        return { waited: false };
    }

    // A library is "ready" once it has initialized something. (If it's
    // referenced but the script never runs, it can't init and we time out.)
    const ready = (l) => document.querySelectorAll(l.marker).length > 0;

    while (!expected.every(ready) && Date.now() - start < maxMs) {
        await sleep(150);
    }

    const result = { waited: true, elapsed: Date.now() - start };
    for (const l of expected) {
        result[l.name] = document.querySelectorAll(l.marker).length;
    }
    return result;
}
