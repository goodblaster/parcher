// Dismisses the major consent-management platforms (CMPs) by their exact,
// documented selectors. Far more reliable than text heuristics: these
// selectors are the same ones browser "auto-consent" extensions use.
//
// Also removes cross-origin CMP iframes (Sourcepoint and friends) that
// cannot be clicked from page JavaScript, along with their scroll locks.
//
// Returns { clicked: [names], removedIframes } for logging.
(() => {
    const clicked = [];
    let removedIframes = 0;

    const isVisible = (el) => {
        if (!el || !el.isConnected) return false;
        const rect = el.getBoundingClientRect();
        if (rect.width === 0 || rect.height === 0) return false;
        const style = window.getComputedStyle(el);
        return style.display !== 'none' && style.visibility !== 'hidden';
    };

    // name → CSS selector for the "accept/dismiss" control.
    const cmps = {
        'onetrust': '#onetrust-accept-btn-handler, #accept-recommended-btn-handler',
        'cookiebot': '#CybotCookiebotDialogBodyLevelButtonLevelOptinAllowAll, #CybotCookiebotDialogBodyButtonAccept',
        'didomi': '#didomi-notice-agree-button',
        'quantcast': '.qc-cmp2-summary-buttons button[mode="primary"]',
        'trustarc': '#truste-consent-button',
        'complianz': '.cmplz-accept',
        'cookieyes': '.cky-btn-accept',
        'osano': '.osano-cm-accept-all',
        'iubenda': '.iubenda-cs-accept-btn',
        'klaro': '.klaro .cm-btn-success',
        'tarteaucitron': '#tarteaucitronPersonalize2, .tarteaucitronAllow',
        'borlabs': '#borlabs-cookie a[data-cookie-accept-all], #borlabs-cookie ._brlbs-accept-all',
        'consentmanager': '#cmpwelcomebtnyes, .cmpboxbtnyes',
        'gdpr-cookie-consent': '#cookie_action_close_header',
        'cookieconsent-js': '.cc-btn.cc-allow, .cc-btn.cc-dismiss',
        'funding-choices': '.fc-cta-consent', // Google Funding Choices (many publishers)
        'ezoic': '#ez-accept-all',
        'axeptio': '#axeptio_btn_acceptAll',
        'google': 'button#L2AGLb', // google.com's own consent page
        'amazon': '#sp-cc-accept',
    };

    for (const [name, selector] of Object.entries(cmps)) {
        try {
            const btn = document.querySelector(selector);
            if (btn && isVisible(btn)) {
                btn.click();
                clicked.push(name);
            }
        } catch (e) { /* selector engine hiccup; keep going */ }
    }

    // Usercentrics renders inside a shadow root.
    const uc = document.querySelector('#usercentrics-root');
    if (uc && uc.shadowRoot) {
        const btn = uc.shadowRoot.querySelector('button[data-testid="uc-accept-all-button"]');
        if (btn) {
            btn.click();
            clicked.push('usercentrics');
        }
    }

    // Sourcepoint (and similar) serve the dialog from a cross-origin iframe
    // that page JS cannot reach into — remove the whole apparatus instead.
    for (const el of document.querySelectorAll(
        'iframe[id^="sp_message_iframe"], div[id^="sp_message_container"]')) {
        el.remove();
        removedIframes++;
    }
    for (const cls of ['sp-message-open']) {
        document.documentElement.classList.remove(cls);
        if (document.body) document.body.classList.remove(cls);
    }

    return { clicked, removedIframes };
})
