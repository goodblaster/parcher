// Runs on every new document BEFORE the page's own JavaScript (registered
// via Page.addScriptToEvaluateOnNewDocument). Seeds the localStorage flags
// that cookie consent banners check on startup, so many of them
// decide they've already been answered and never render.
//
// Only cookie-banner dismissal markers are set — no eligibility claims or complex
// per-CMP consent payloads are forged.
(() => {
    const seed = {
        // osano/cookieconsent JS library family.
        'cookieconsent_status': 'dismiss',
        // Common hand-rolled flags.
        'cookiesAccepted': 'true',
        'cookies_accepted': 'true',
        'cookieConsent': 'true',
        'cookie_consent': 'accepted',
        'gdpr_consent': 'true',
    };

    try {
        for (const [key, value] of Object.entries(seed)) {
            if (window.localStorage.getItem(key) === null) {
                window.localStorage.setItem(key, value);
            }
        }
    } catch (e) {
        // Storage can be unavailable (sandboxed frames, disabled storage).
    }
})
