// Detect visible access decisions, not ordinary footer disclaimers. This is a
// conservative DOM heuristic, not a claim that every site's gate is detectable.
// Approval re-observes the exact evidence before clicking normal controls.
((expected = null) => {
    const norm = s => (s || '').replace(/\s+/g, ' ').trim();
    const visible = el => {
        const r = el.getBoundingClientRect();
        return r.width > 0 && r.height > 0 &&
            el.checkVisibility({opacityProperty: true, visibilityProperty: true, contentVisibilityAuto: true});
    };
    const visibleText = el => {
        const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
        const parts = [];
        while (walker.nextNode()) {
            const parent = walker.currentNode.parentElement;
            if (parent && !parent.closest('script, style, noscript, [role="status"], [role="alert"]') && visible(parent)) parts.push(walker.currentNode.textContent);
        }
        return norm(parts.join(' '));
    };
    const label = el => norm(el.getAttribute('aria-label') ||
        Array.from(el.labels || []).map(l => l.innerText).join(' ') ||
        el.innerText || el.value || el.getAttribute('title'));
    const age = /\b(age verification|verify your age|legal (drinking )?age|of age|years (of age|old)|over (18|21)|(18|21) or older|at least (18|21)|age of (18|21)|age restricted|age-restricted)\b/i;
    const attest = /\b(certify|attest|qualified researcher|research (purposes|use) only|for (in vitro|laboratory) research|not for (human|veterinary))\b/i;
    const entry = /\b(enter|continue|access|confirm|agree|accept|verify|proceed)\b/i;
    const purchase = /\b(checkout|place order|buy now|pay now|add to cart|subscribe for)\b/i;
    const positive = /^(yes\b|enter\b|continue\b|confirm\b|verify\b|proceed\b|i (agree|accept|confirm|am|certify)\b|accept\b|agree\b)/i;
    const negative = /\b(not|exit|leave|cancel|decline|under|no thanks)\b/i;
    const roots = [document];
    for (let i = 0; i < roots.length; i++) {
        for (const el of roots[i].querySelectorAll('*')) {
            if (el.shadowRoot) roots.push(el.shadowRoot);
        }
    }
    const found = [];
    for (const root of roots) {
        const candidates = new Set(root.querySelectorAll(
            'dialog[open], [role="dialog"], [role="alertdialog"], [aria-modal="true"], form, ' +
            '[class*="age-gate" i], [id*="age-gate" i], [class*="age_gate" i], ' +
            '[class*="age-verif" i], [id*="age-verif" i], [data-gate], ' +
            '[class*="modal" i], [class*="overlay" i], [class*="interstitial" i], ' +
            '[id*="modal" i], [id*="overlay" i], [id*="interstitial" i], ' +
            'iframe[title*="captcha" i], iframe[title*="challenge" i]'));
        for (let el of candidates) {
            if (el === document.body || el === document.documentElement || !visible(el)) continue;
            // A form's surrounding section usually contains its heading and
            // terms. Include that context in both classification and approval.
            if (el.tagName === 'FORM') {
                el = el.closest('dialog, [role="dialog"], [aria-modal="true"], main, section, article') || el;
            }
            if (found.some(g => g.element === el || g.element.contains(el))) continue;
            const text = visibleText(el) || norm(el.getAttribute('title'));
            const inputs = Array.from(el.querySelectorAll('input:not([type="hidden"]), select, textarea, [role="checkbox"], [role="radio"], [contenteditable="true"]'))
                .filter(e => visible(e) || (e.type === 'checkbox' && Array.from(e.labels || []).some(visible)));
            const buttons = Array.from(el.querySelectorAll('button, input[type="submit"], input[type="button"], a, [role="button"]')).filter(visible);
            const textWithLabels = text + ' ' + inputs.map(label).join(' ') + ' ' + buttons.map(label).join(' ');
            const isAge = age.test(textWithLabels) || /\b(18|21)\s*\+/.test(textWithLabels) || /age.?gate|age.?verif/i.test(el.id + ' ' + el.className);
            const isAttest = attest.test(textWithLabels);
            const hasRequiredChecks = inputs.some(e => e.type === 'checkbox' && e.required);
            const isLogin = inputs.some(e => e.type === 'password');
            const isChallenge = el.tagName === 'IFRAME' || /\b(verify you are human|verify you're human|complete the captcha|checking your browser)\b/i.test(text);
            let modal = el.matches('dialog[open], [aria-modal="true"], [role="dialog"], [role="alertdialog"]');
            for (let p = el; p && p !== document.body; p = p.parentElement) {
                const s = getComputedStyle(p), r = p.getBoundingClientRect();
                if (s.position === 'fixed' && r.width * r.height > innerWidth * innerHeight * 0.2) modal = true;
            }
            const r = el.getBoundingClientRect();
            if (r.bottom <= 0 || r.right <= 0 || r.top >= innerHeight || r.left >= innerWidth) continue;
            const central = r.bottom > innerHeight * 0.25 && r.top < innerHeight * 0.75;
            const interstitial = central && (isAge || isAttest || isLogin || isChallenge) &&
                (entry.test(textWithLabels) || isLogin || isChallenge);
            if (!(modal || interstitial)) continue;
            const cookieOnly = /\b(cookies?|tracking|newsletter)\b/i.test(textWithLabels) && !isAge && !isAttest;
            if (!(isAge || isAttest || isLogin || isChallenge || (modal && hasRequiredChecks && !cookieOnly))) continue;
            // Terms accepted during a purchase are not an entry gate.
            if (purchase.test(buttons.map(label).join(' '))) continue;
            if (!inputs.length && !buttons.length && !isChallenge) continue;
            const kind = isChallenge ? 'challenge' : isLogin ? 'login' : isAge ? 'age' : isAttest ? 'attestation' : 'unknown';
            const forms = Array.from(el.querySelectorAll('form'));
            if (el.tagName === 'FORM') forms.unshift(el);
            const form = forms.length === 1 ? forms[0] : null;
            const choices = buttons.filter(b => positive.test(label(b)) && !negative.test(label(b)));
            const submit = choices.length === 1 ? choices[0] : null;
            const checks = inputs.filter(e => e.type === 'checkbox');
            // Only simple declarations and unambiguous entry buttons can be
            // approved. Dates of birth, credentials and custom widgets need a
            // manual browser session; never guess values for them.
            const simple = inputs.every(e => e.type === 'checkbox' && e.required && label(e));
            const action = form ? (submit?.getAttribute('formaction') ? submit.formAction : form.action) : (submit?.href || '');
            const method = form ? (submit?.getAttribute('formmethod') || form.method).toUpperCase() : 'CLICK';
            const target = submit?.getAttribute('formtarget') || submit?.getAttribute('target') || form?.target || '';
            const sameOrigin = !form || new URL(action).origin === location.origin;
            const canApprove = (kind === 'age' || kind === 'attestation') && simple && !!submit && sameOrigin && (!target || target === '_self') &&
                (checks.length === 0 || !!form) && (!form || submit.form === form) &&
                (!submit.disabled || checks.some(e => !e.checked));
            const evidence = {
                url: location.href.split('#')[0],
                kind, text,
                controls: inputs.map(e => ({type: e.type || e.tagName.toLowerCase(), name: e.name || '', label: label(e), required: !!e.required})),
                links: Array.from(el.querySelectorAll('a[href]')).filter(visible).map(e => ({text: label(e), url: e.href})),
                action, method,
                button: submit ? label(submit) : '',
                approvable: canApprove
            };
            // Retain the enclosing gate rather than a duplicate nested form.
            for (let i = found.length - 1; i >= 0; i--) if (el.contains(found[i].element)) found.splice(i, 1);
            found.push({element: el, evidence, checks, submit});
        }
    }
    if (expected === null) return found.map(g => g.evidence);
    const matches = found.filter(g => JSON.stringify(g.evidence) === JSON.stringify(expected));
    if (matches.length !== 1 || !matches[0].evidence.approvable) return false;
    const gate = matches[0];
    for (const checkbox of gate.checks) {
        if (!checkbox.isConnected || !window.__pscriptionInspectAccessGates().some(g => JSON.stringify(g) === JSON.stringify(expected))) return false;
        if (!checkbox.checked) checkbox.click();
    }
    // Recheck after change handlers: they may replace the form, change its
    // declarations, or disable the button. Do not submit changed evidence.
    const current = window.__pscriptionInspectAccessGates();
    if (!current.some(g => JSON.stringify(g) === JSON.stringify(expected)) ||
        !gate.submit.isConnected || gate.submit.disabled || gate.checks.some(e => !e.checked)) return false;
    window.__pscriptionSubmittedGateVisible = () => gate.element.isConnected && visible(gate.element);
    gate.submit.click();
    return true;
})
