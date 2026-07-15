// Waits until the page's images have finished loading, up to maxWaitMs.
// Checks <img> elements directly (complete + naturalWidth) and probes CSS
// background-image URLs via Image() objects (served from cache when loaded).
// Every probe carries its own timeout so a single stalled image can never
// hang the whole wait past its budget.
async function waitForImages(maxWaitMs) {
    const start = Date.now();
    maxWaitMs = maxWaitMs || 10000;
    const deadline = () => Math.max(0, maxWaitMs - (Date.now() - start));
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

    // 1. DOM images: poll until all are settled (loaded or errored).
    const imgs = Array.from(document.images).filter(
        (img) => img.src || img.srcset);
    const settled = (img) => img.complete; // complete is true on error too
    let pending = imgs.filter((img) => !settled(img));

    while (pending.length > 0 && deadline() > 0) {
        await sleep(Math.min(200, deadline()));
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
    for (let i = 0; i < urls.length && deadline() > 0; i += 10) {
        const results = await Promise.all(urls.slice(i, i + 10).map(probe));
        bgLoaded += results.filter(Boolean).length;
    }

    return {
        total: imgs.length,
        loaded,
        stillPending: pending.length,
        backgrounds: urls.length,
        backgroundsLoaded: bgLoaded,
        elapsed: Date.now() - start,
    };
}
