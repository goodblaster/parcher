// Waits until the DOM has stopped changing: resolves once no mutation has
// occurred for quietMs, or after maxMs regardless. A condition-based
// replacement for fixed sleeps — pages that settle quickly don't make the
// capture wait, and pages that keep mutating get a hard cap.
//
// When settle_monitor.js has been registered (a pre-navigation script, so it
// has been recording since document start), this consults ITS record and can
// return on the first check: a page that went quiet a second ago has already
// proven it. Without the monitor it installs its own observer and falls back
// to the prospective behaviour, where the quiet window is a floor on the
// wait's cost.
async function waitForQuiet(quietMs, maxMs) {
    quietMs = quietMs || 500;
    maxMs = maxMs || 5000;

    const monitored = !!window.__gbSettle;
    let rec = window.__gbSettle;
    let observer = null;

    if (!rec) {
        rec = { last: performance.now() };
        observer = new MutationObserver(() => {
            rec.last = performance.now();
        });
        observer.observe(document.documentElement, {
            childList: true,
            subtree: true,
            attributes: true,
            characterData: true,
        });
    }

    const start = Date.now();

    return new Promise((resolve) => {
        const settled = () => {
            const quietFor = performance.now() - rec.last;
            const elapsed = Date.now() - start;
            if (quietFor < quietMs && elapsed < maxMs) {
                return false;
            }
            if (observer) observer.disconnect();
            resolve({
                elapsed,
                quietFor: Math.round(quietFor),
                timedOut: elapsed >= maxMs,
                monitored,
            });
            return true;
        };

        // Check before arming the timer: with the monitor, an already-quiet
        // page costs one round trip instead of a full window.
        if (settled()) return;

        const timer = setInterval(() => {
            if (settled()) clearInterval(timer);
        }, 100);
    });
}
