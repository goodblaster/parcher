// Actively dismisses dialogs that block page content: age gates, cookie
// consent banners, newsletter modals, and login nags.
//
// Rules of engagement:
//   - Only VISIBLE controls are ever clicked.
//   - Strongly-worded affirmatives ("enter site", "i am over 18",
//     "accept all cookies") are clicked wherever they appear.
//   - Generic words ("yes", "accept", "agree", "close", "×") are only
//     clicked inside something that looks like an overlay/dialog, so we
//     never press a "Yes" that belongs to the page itself.
//   - After clicking, leftover gate/preloader wrappers are removed from the
//     DOM so they cannot appear in the capture.
//
//   - Native <dialog>s are closed only when MODAL (or named like an
//     overlay): a non-modal <dialog open> is a content panel the page chose
//     to show, and closing every dialog[open] deleted those from archives.
//   - Popover promos (the Popover API's top layer) are hidden the same way.
//
// Returns { clicked, removed, closedDialogs, closedPopovers } for logging.
(() => {
    const MAX_CLICKS = 6;
    let clicked = 0;
    let removed = 0;
    let closedDialogs = 0;

    // className can be an SVGAnimatedString on SVG elements; always go
    // through getAttribute so we operate on plain strings.
    const cls = (el) => (el.getAttribute && el.getAttribute('class')) || '';

    // checkVisibility walks the ANCESTOR chain, which the element-only test
    // below it cannot: a button inside a dialog faded to opacity:0, or
    // inside content-visibility:hidden, has a box and its own opacity of 1,
    // and used to get clicked.
    const isVisible = (el) => {
        if (!el.isConnected) return false;
        const rect = el.getBoundingClientRect();
        if (rect.width === 0 || rect.height === 0) return false;
        if (!el.checkVisibility({ opacityProperty: true, visibilityProperty: true, contentVisibilityAuto: true })) {
            return false;
        }
        const style = window.getComputedStyle(el);
        return style.display !== 'none' &&
            style.visibility !== 'hidden' &&
            parseFloat(style.opacity) > 0.05;
    };

    const overlayClassRe = /modal|popup|pop-up|overlay|dialog|lightbox|consent|cookie|gdpr|age.?gate|age.?verif|newsletter|interstitial|banner/i;

    // Does el live inside something that looks like a floating dialog?
    const inOverlay = (el) => {
        let node = el;
        for (let depth = 0; node && node !== document.body && depth < 10; depth++) {
            if (node.getAttribute) {
                if (node.getAttribute('role') === 'dialog' ||
                    node.getAttribute('role') === 'alertdialog' ||
                    node.getAttribute('aria-modal') === 'true' ||
                    overlayClassRe.test(cls(node)) ||
                    overlayClassRe.test(node.id || '')) {
                    return true;
                }
                const style = window.getComputedStyle(node);
                if (style.position === 'fixed' || style.position === 'sticky') {
                    return true;
                }
            }
            node = node.parentElement;
        }
        return false;
    };

    const norm = (s) => (s || '').replace(/\s+/g, ' ').trim().toLowerCase();

    // Clicked anywhere when visible: these phrases only ever mean "get this
    // dialog out of my way".
    const strongAffirmatives = new Set([
        'enter site', 'enter the site', 'i am over 18', "i'm over 18",
        'i am over 21', "i'm over 21", 'i am 18 or older', 'i am 21 or older',
        'yes, i am over 18', 'yes, i am over 21', 'yes, i am of legal age',
        'accept all cookies', 'accept all', 'allow all', 'allow all cookies',
        'accept cookies', 'i accept cookies', 'agree and continue',
        'accept and continue', 'agree & continue',
    ]);

    // Clicked only inside an overlay container.
    const overlayAffirmatives = new Set([
        'yes', 'enter', 'accept', 'i accept', 'agree', 'i agree', 'got it',
        'ok', 'okay', 'confirm', 'i understand', 'understood', 'continue',
        'allow',
    ]);

    const dismissives = new Set([
        '×', '✕', '✖', 'x', 'close', 'dismiss', 'skip', 'no thanks',
        'not now', 'maybe later', 'continue without accepting',
    ]);

    const candidates = document.querySelectorAll(
        'button, a, input[type="button"], input[type="submit"], [role="button"]');

    for (const el of candidates) {
        if (clicked >= MAX_CLICKS) break;
        if (!isVisible(el)) continue;

        const text = norm(el.tagName === 'INPUT' ? el.value : el.textContent);
        const aria = norm(el.getAttribute('aria-label'));
        if (text.length > 60) continue; // real buttons are short

        const isAgeGateButton = /age.?gate/i.test(cls(el)) &&
            /yes|submit|enter|confirm/i.test(cls(el) + ' ' + text);

        if (strongAffirmatives.has(text) || isAgeGateButton) {
            el.click();
            clicked++;
            continue;
        }

        if (!inOverlay(el)) continue;

        if (overlayAffirmatives.has(text) ||
            dismissives.has(text) ||
            aria.includes('close') || aria.includes('dismiss')) {
            el.click();
            clicked++;
        }
    }

    // Native <dialog> elements: modal ones block the page by definition;
    // a non-modal one is closed only if it names itself as overlay chrome.
    const overlayNamed = (el) => overlayClassRe.test(cls(el)) || overlayClassRe.test(el.id || '');
    for (const dialog of document.querySelectorAll('dialog[open]')) {
        let modal = false;
        try { modal = dialog.matches(':modal'); } catch (e) { /* old engine */ }
        if (!modal && !overlayNamed(dialog)) continue;
        try {
            dialog.close();
        } catch (e) {
            dialog.removeAttribute('open');
        }
        closedDialogs++;
    }

    // Popovers sit in the top layer above everything, like a modal dialog.
    // Hide the ones that look like promos: named as an overlay, or covering
    // a large share of the viewport. A small open popover (a menu, a
    // tooltip) is left as the page had it.
    let closedPopovers = 0;
    let openPopovers = [];
    try { openPopovers = document.querySelectorAll(':popover-open'); } catch (e) { /* old engine */ }
    for (const pop of openPopovers) {
        const r = pop.getBoundingClientRect();
        const share = (r.width * r.height) / (window.innerWidth * window.innerHeight);
        if (!overlayNamed(pop) && share < 0.25) continue;
        try {
            pop.hidePopover();
            closedPopovers++;
        } catch (e) { /* not hideable */ }
    }

    // Remove gate/preloader remnants outright. By the time this script runs
    // (and it runs several times), any gate that wanted clicking has been
    // clicked; whatever is left of it would only pollute the capture.
    //
    // Guards: sites put state classes like "age-gate-open" on <body> or a
    // page wrapper — matching by class alone must never delete the page.
    // Structural elements (html/body or anything with a large subtree) get
    // the matching classes stripped instead of being removed.
    const remnants = document.querySelectorAll(
        '[class*="age-gate" i], [id*="age-gate" i], [class*="age_gate" i], ' +
        '#preloader, .preloader, .page-preloader, .page-loader, ' +
        '[id*="page-preloader" i], .loading-overlay, .loader-overlay');
    for (const el of remnants) {
        const structural = el === document.body ||
            el === document.documentElement ||
            el.querySelectorAll('*').length > 200;
        if (structural) {
            for (const c of Array.from(el.classList)) {
                if (/age.?gate|preloader|page-loader|loading-overlay|loader-overlay/i.test(c)) {
                    el.classList.remove(c);
                }
            }
            continue;
        }
        el.remove();
        removed++;
    }

    // Unlock scrolling if a (now dismissed) modal froze it. Guarded: a
    // dismissal click can trigger navigation, tearing the document down
    // under us mid-run.
    const unlock = (el) => {
        if (!el || !(el instanceof Element) || !el.isConnected) return;
        const style = window.getComputedStyle(el);
        if (style.overflow === 'hidden' || style.overflowY === 'hidden') {
            el.style.setProperty('overflow', 'auto', 'important');
        }
        if (style.position === 'fixed') {
            el.style.setProperty('position', 'static', 'important');
        }
    };
    try {
        unlock(document.documentElement);
        unlock(document.body);
    } catch (e) { /* document mid-navigation */ }

    return { clicked, removed, closedDialogs, closedPopovers };
})
