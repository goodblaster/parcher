// Last-resort overlay demolition, run just before capture. Anything still
// covering the page at this point has survived cookie pre-seeding, the
// known-CMP selectors, and the text heuristics — remove it.
//
// A candidate must look like a blocking overlay, not content:
//   - position fixed/sticky and visible
//   - covers the viewport's center point AND a large share of its area,
//     or is an obvious backdrop (near-full-viewport, elevated z-index)
// Guards so real page furniture survives:
//   - never html/body
//   - skipped if it holds a meaningful share of the page's text (a fixed
//     hero/article container is content, not chrome)
//
// What an overlay leaves behind is cleaned up too, but only when something
// was actually removed: the scroll lock (overflow AND the position:fixed
// body trick, same as dismiss_popups.js), and `inert` on the page's
// top-level containers — modal libraries set it on everything behind the
// dialog, and with the dialog deleted rather than closed nothing ever takes
// it off, leaving an archive whose text cannot be selected or found with
// the browser's find-in-page.
//
// Returns { removed, unlocked, uninerted } for logging.
(() => {
    let removed = 0;

    const vw = window.innerWidth;
    const vh = window.innerHeight;
    const bodyTextLength = Math.max(1, (document.body?.innerText || '').length);

    const coversCenter = (el) => {
        const hit = document.elementFromPoint(vw / 2, vh / 2);
        return hit !== null && (el === hit || el.contains(hit));
    };

    const candidates = [];
    for (const el of document.querySelectorAll('body *')) {
        const style = window.getComputedStyle(el);
        if (style.position !== 'fixed' && style.position !== 'sticky') continue;
        if (style.display === 'none' || style.visibility === 'hidden') continue;
        // Ancestors count: an overlay inside a faded-out or hidden wrapper
        // is not covering anything.
        if (!el.checkVisibility({ opacityProperty: true, visibilityProperty: true })) continue;

        const rect = el.getBoundingClientRect();
        if (rect.width === 0 || rect.height === 0) continue;

        const areaShare = (rect.width * rect.height) / (vw * vh);
        const zIndex = parseInt(style.zIndex, 10) || 0;

        // Full-viewport sheet, or an elevated layer sitting over the middle
        // of the page. Sticky headers fail both tests (small height, don't
        // cover center).
        const fullSheet = areaShare >= 0.85;
        const blockingLayer = areaShare >= 0.35 && zIndex >= 100 && coversCenter(el);
        if (!fullSheet && !blockingLayer) continue;

        // Content guard: overlays are chrome, they don't hold the page's
        // words. (elements nested in a candidate are covered by removing
        // the outermost one)
        const textShare = ((el.innerText || '').length) / bodyTextLength;
        if (textShare > 0.2) continue;

        candidates.push(el);
    }

    // Remove outermost candidates only.
    for (const el of candidates) {
        if (!el.isConnected) continue; // parent already removed
        if (candidates.some((other) => other !== el && other.contains(el))) continue;
        el.remove();
        removed++;
    }

    // Overlays often leave a scroll lock behind.
    let unlocked = 0;
    for (const el of [document.documentElement, document.body]) {
        if (!el) continue;
        const style = window.getComputedStyle(el);
        if (style.overflow === 'hidden' || style.overflowY === 'hidden') {
            el.style.setProperty('overflow', 'auto', 'important');
            unlocked++;
        }
        // The other common lock: body pinned with position:fixed. Only
        // undone when an overlay was removed — a page may pin its own body.
        if (removed > 0 && style.position === 'fixed') {
            el.style.setProperty('position', 'static', 'important');
            unlocked++;
        }
    }

    let uninerted = 0;
    if (removed > 0 && document.body) {
        for (const el of [document.body, ...document.body.children]) {
            if (el.hasAttribute('inert')) {
                el.removeAttribute('inert');
                uninerted++;
            }
        }
    }

    return { removed, unlocked, uninerted };
})
