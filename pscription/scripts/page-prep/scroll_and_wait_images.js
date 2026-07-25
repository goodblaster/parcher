// Scrolls the viewport through the whole page from top to bottom in
// viewport-sized steps, pausing at each step. This is what actually fires
// IntersectionObserver/scroll-event lazy loaders — they only load content
// that enters the viewport, so all steps must happen sequentially, in order.
//
// Async: the Go side awaits completion. Parameters:
//   stepDelayMs — pause per step (default 250)
//   maxMs       — overall budget (default 15000)
async function scrollAndWaitImages(stepDelayMs, maxMs) {
    stepDelayMs = stepDelayMs || 250;
    maxMs = maxMs || 15000;
    const start = Date.now();

    document.documentElement.style.scrollBehavior = 'auto';
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

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
        return { steps: 0, height: pageHeight(), elapsed: Date.now() - start, skipped: 'fits-viewport' };
    }

    let steps = 0;
    let y = 0;

    // Page height can grow as lazy content loads; re-check every step.
    while (y < pageHeight() && Date.now() - start < maxMs) {
        window.scrollTo(0, y);
        // Some lazy loaders listen for scroll events rather than observing.
        window.dispatchEvent(new Event('scroll'));
        await sleep(stepDelayMs);
        y += Math.max(200, window.innerHeight - 100); // slight overlap
        steps++;
    }

    // Bottom, then back to the top for capture.
    window.scrollTo(0, pageHeight());
    await sleep(stepDelayMs);
    window.scrollTo(0, 0);
    await sleep(100);

    return { steps, height: pageHeight(), elapsed: Date.now() - start };
}
