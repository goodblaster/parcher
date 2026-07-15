// Waits until the DOM has stopped changing: resolves once no mutation has
// occurred for quietMs, or after maxMs regardless. A condition-based
// replacement for fixed sleeps — pages that settle quickly don't make the
// capture wait, and pages that keep mutating get a hard cap.
async function waitForQuiet(quietMs, maxMs) {
    quietMs = quietMs || 500;
    maxMs = maxMs || 5000;

    return new Promise((resolve) => {
        const start = Date.now();
        let lastMutation = Date.now();

        const observer = new MutationObserver(() => {
            lastMutation = Date.now();
        });
        observer.observe(document.documentElement, {
            childList: true,
            subtree: true,
            attributes: true,
            characterData: true,
        });

        const timer = setInterval(() => {
            const quietFor = Date.now() - lastMutation;
            const elapsed = Date.now() - start;
            if (quietFor >= quietMs || elapsed >= maxMs) {
                clearInterval(timer);
                observer.disconnect();
                resolve({ elapsed, quietFor, timedOut: elapsed >= maxMs });
            }
        }, 100);
    });
}
