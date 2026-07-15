// Applies a link policy to the snapshot. Runs as the final prep step,
// only when a non-default policy is requested (the default is to leave
// links exactly as the page had them).
//
// Modes:
//   "new-tab" — external links open in a new tab (target=_blank,
//               rel=noopener) so following one never replaces the archive.
//   "disable" — links keep their href (the URL is part of what was
//               archived, and removing href changes :link styling) but
//               become unclickable via injected pointer-events CSS.
//               Form actions are parked in data-gb-action so a stray
//               submit can't navigate either.
//
// Internal fragment links (#section, or full URLs to this page with a
// fragment) and mailto:/tel: stay live in every mode — they don't leave
// the page.
((mode) => {
    if (mode !== 'new-tab' && mode !== 'disable') {
        return { mode, changed: 0 };
    }

    const pageHref = location.href.split('#')[0];

    const staysInPage = (a) => {
        const href = a.getAttribute('href') || '';
        if (href === '' || href.startsWith('#')) return true;
        if (/^(mailto:|tel:|javascript:)/i.test(href)) return true;
        try {
            const u = new URL(href, location.href);
            if (u.hash && u.href.split('#')[0] === pageHref) return true;
        } catch (e) { /* unparseable href — treat as external */ }
        return false;
    };

    let changed = 0;

    if (mode === 'new-tab') {
        for (const a of document.querySelectorAll('a[href], area[href]')) {
            if (staysInPage(a)) continue;
            a.setAttribute('target', '_blank');
            a.setAttribute('rel', 'noopener');
            changed++;
        }
        return { mode, changed };
    }

    // mode === 'disable'
    if (!document.getElementById('gb-disable-links')) {
        const style = document.createElement('style');
        style.id = 'gb-disable-links';
        style.textContent = `
            a[href]:not([data-gb-internal]),
            area[href]:not([data-gb-internal]) {
                pointer-events: none !important;
                cursor: default !important;
            }
        `;
        (document.head || document.documentElement).appendChild(style);
    }
    for (const a of document.querySelectorAll('a[href], area[href]')) {
        if (staysInPage(a)) {
            a.setAttribute('data-gb-internal', '');
        } else {
            changed++;
        }
    }
    for (const form of document.querySelectorAll('form')) {
        const action = form.getAttribute('action');
        if (action !== null) {
            form.setAttribute('data-gb-action', action);
            form.removeAttribute('action');
        }
    }

    return { mode, changed };
})
