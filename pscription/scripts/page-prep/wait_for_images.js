// Waits until the page's images have finished loading, up to maxWaitMs.
// <img> elements are awaited through img.decode() — it settles the moment
// the image is loaded AND decoded (or has failed), where polling `complete`
// every 200ms paid up to a full tick per wait for nothing. CSS
// background-image URLs are probed via Image() objects (served from cache
// when loaded) by a small worker pool, so one slow URL holds up one worker
// instead of the nine other probes that used to share its batch.
// Every probe carries its own timeout so a single stalled image can never
// hang the whole wait past its budget.
async function waitForImages(maxWaitMs) {
    const start = Date.now();
    maxWaitMs = maxWaitMs || 10000;
    const deadline = () => Math.max(0, maxWaitMs - (Date.now() - start));
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

    // 1. DOM images: wait until all are settled (loaded or errored).
    const imgs = Array.from(document.images).filter(
        (img) => img.src || img.srcset);
    const settled = (img) => img.complete; // complete is true on error too
    let pending = imgs.filter((img) => !settled(img));

    if (pending.length > 0 && deadline() > 0) {
        // decode() rejects on a broken image, which is still "settled".
        const decoded = Promise.allSettled(pending.map((img) => img.decode()));
        await Promise.race([decoded, sleep(deadline())]);
        pending = pending.filter((img) => !settled(img));
    }

    const loaded = imgs.filter((img) => img.complete && img.naturalWidth > 0).length;

    // 2. Background images: collect unique URLs and probe them.
    const bgUrls = new Set();
    for (const el of document.querySelectorAll('*')) {
        for (const pseudo of [null, '::before', '::after']) {
            const bg = window.getComputedStyle(el, pseudo).backgroundImage;
            if (!bg || bg === 'none') continue;
            for (const m of bg.matchAll(/url\(\s*(['"]?)([^'")]+)\1\s*\)/g)) {
                if (!m[2].startsWith('data:')) {
                    bgUrls.add(m[2]);
                }
            }
        }
    }

    const probe = (url) => new Promise((resolve) => {
        const img = new Image();
        const timer = setTimeout(() => resolve(false), Math.min(3000, deadline() || 1));
        img.onload = () => { clearTimeout(timer); resolve(true); };
        img.onerror = () => { clearTimeout(timer); resolve(false); };
        img.src = url;
        if (img.complete) { clearTimeout(timer); resolve(true); }
    });

    let bgLoaded = 0;
    const urls = Array.from(bgUrls);
    let next = 0;
    const worker = async () => {
        while (next < urls.length && deadline() > 0) {
            if (await probe(urls[next++])) bgLoaded++;
        }
    };
    await Promise.all(Array.from({ length: Math.min(10, urls.length) }, worker));

    return {
        total: imgs.length,
        loaded,
        stillPending: pending.length,
        backgrounds: urls.length,
        backgroundsLoaded: bgLoaded,
        elapsed: Date.now() - start,
    };
}
