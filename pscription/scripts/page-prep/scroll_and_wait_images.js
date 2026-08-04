// Scrolls the viewport through the whole page from top to bottom in
// viewport-sized steps, pausing at each step. This is what actually fires
// IntersectionObserver/scroll-event lazy loaders — they only load content
// that enters the viewport, so all steps must happen sequentially, in order.
//
// The pause per step is EVIDENCE-DRIVEN, not fixed. A fixed pause is paid in
// full by every band of a long page that has nothing to lazy-load:
// test-site-7/wiki/Trilobite is 30717px — 39 steps — and exactly one
// step produced any DOM mutation at all, yet the walk cost 10.1s of a 15.9s
// capture. Each step now waits for a rendering opportunity, then keeps
// waiting only while something is still happening.
//
// Two signals, because either alone is blind to half the lazy loaders:
//   - DOM mutations: a script-driven loader swapping src/data-src.
//   - Resource timing: native loading="lazy", where the browser fetches the
//     image itself and NOTHING in the DOM changes. test-site-7 is exactly this
//     case, which is why its walk looks completely inert to a mutation
//     observer while still loading forty images.
//
// Async: the Go side awaits completion. Parameters:
//   stepDelayMs — per-step CAP, and the pause at the bottom (default 250)
//   maxMs       — overall budget (default 15000)
async function scrollAndWaitImages(stepDelayMs, maxMs) {
    stepDelayMs = stepDelayMs || 250;
    maxMs = maxMs || 15000;

    // Floor per step once the page has had a chance to paint, and how long
    // "nothing happening" must hold before moving on. A band with no lazy
    // content costs about two frames plus quietMs.
    const minStepMs = 40;
    const quietMs = 50;

    const start = Date.now();

    document.documentElement.style.scrollBehavior = 'auto';
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

    // A rendering opportunity. IntersectionObserver callbacks are delivered
    // as part of updating the rendering, so a scroll that is never painted
    // notifies nobody and the entire walk becomes a no-op. Raced against a
    // timer: rAF does not fire in a throttled or backgrounded tab, and this
    // must never be able to hang the capture.
    const frame = () => Promise.race([
        new Promise((r) => requestAnimationFrame(() => r())),
        sleep(50),
    ]);

    let mutations = 0;
    const observer = new MutationObserver((records) => {
        mutations += records.length;
    });
    observer.observe(document.documentElement, {
        childList: true,
        subtree: true,
        attributes: true,
    });

    let resources = 0;
    let perfObserver = null;
    try {
        perfObserver = new PerformanceObserver((list) => {
            resources += list.getEntries().length;
        });
        perfObserver.observe({ type: 'resource', buffered: false });
    } catch (e) { /* no resource timing: mutations alone still work */ }

    const stop = () => {
        observer.disconnect();
        if (perfObserver) perfObserver.disconnect();
    };

    // Waits out the reaction to one scroll: a rendering opportunity first,
    // then as long as loading keeps producing evidence, up to stepDelayMs.
    const settle = async () => {
        await frame();
        await frame();
        const t0 = performance.now();
        let seen = mutations + resources;
        let lastChange = performance.now();
        for (;;) {
            const now = performance.now();
            const total = mutations + resources;
            if (total !== seen) {
                seen = total;
                lastChange = now;
            }
            const elapsed = now - t0;
            if (elapsed >= stepDelayMs) return;
            if (elapsed >= minStepMs && now - lastChange >= quietMs) return;
            await sleep(10);
        }
    };

    const pageHeight = () => Math.max(
        document.body ? document.body.scrollHeight : 0,
        document.documentElement.scrollHeight);

    // A document that already fits in the viewport has nothing to reveal
    // by scrolling — every element is in view, so IntersectionObservers
    // have already fired. Nudge scroll-event listeners once (some loaders
    // wait for the event, not the position) and skip the walk, which
    // otherwise costs a step pause plus the bottom-and-back round trip.
    if (pageHeight() <= window.innerHeight + 1) {
        window.dispatchEvent(new Event('scroll'));
        await sleep(50);
        stop();
        return { steps: 0, height: pageHeight(), elapsed: Date.now() - start, skipped: 'fits-viewport' };
    }

    let steps = 0;
    let y = 0;

    // Page height can grow as lazy content loads; re-check every step.
    while (y < pageHeight() && Date.now() - start < maxMs) {
        window.scrollTo(0, y);
        // Some lazy loaders listen for scroll events rather than observing.
        window.dispatchEvent(new Event('scroll'));
        await settle();
        y += Math.max(200, window.innerHeight - 100); // slight overlap
        steps++;
    }

    // Bottom, then back to the top for capture. The bottom keeps the full
    // fixed pause: it is where a scroll-throttled loader's trailing call
    // lands, and it is paid once.
    window.scrollTo(0, pageHeight());
    await sleep(stepDelayMs);
    window.scrollTo(0, 0);
    await sleep(100);

    stop();
    return { steps, height: pageHeight(), elapsed: Date.now() - start };
}
