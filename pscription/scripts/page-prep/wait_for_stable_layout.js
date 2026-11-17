// Waits until the page's total height stops changing — a proxy for "the
// layout has settled." Carousels/sliders and tab panels initialize via the
// site's JS (often triggered by scrolling into view or window load); until
// they do, their slides render stacked instead of one-at-a-time, which can
// triple the page height and leave a blank void where the active slide
// belongs. Capturing mid-initialization freezes that broken state.
//
// This resolves once scrollHeight has held steady for quietMs, or after
// maxMs. Unlike wait_for_quiet (which watches DOM mutations and can return
// during a lull before a late layout shift), this keys directly on the
// height instability that produces the blank-chunk bug.
//
// Params: quietMs (default 700), maxMs (default 8000).
async function waitForStableLayout(quietMs, maxMs) {
    quietMs = quietMs || 700;
    maxMs = maxMs || 8000;
    const start = Date.now();
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
    const height = () => Math.max(
        document.documentElement.scrollHeight,
        document.body ? document.body.scrollHeight : 0);

    const rec = window.__gbSettle;
    const monitored = !!(rec && rec.sampleHeight);
    let last = height();
    let stableSince = performance.now();

    for (;;) {
        if (monitored) {
            rec.sampleHeight();
            last = rec.height;
            stableSince = rec.heightSince;
        } else {
            const h = height();
            if (h !== last) {
                last = h;
                stableSince = performance.now();
            }
        }
        if (performance.now() - stableSince >= quietMs || Date.now() - start >= maxMs) break;
        await sleep(150);
    }

    return {
        height: last,
        elapsed: Date.now() - start,
        stable: performance.now() - stableSince >= quietMs,
        monitored,
    };
}
