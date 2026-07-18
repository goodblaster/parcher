// Fills and submits a login form using credentials supplied via vars, which
// come from a separate (gitignored) secrets file — never from this script.
// Adjust the selectors for your target site.
((vars) => {
    const user = document.querySelector('input[name="username"], input[type="email"], #username, #email');
    const pass = document.querySelector('input[name="password"], input[type="password"], #password');
    if (!user || !pass) {
        return { ok: false, reason: 'login fields not found' };
    }
    user.value = vars.username || '';
    pass.value = vars.password || '';
    user.dispatchEvent(new Event('input', { bubbles: true }));
    pass.dispatchEvent(new Event('input', { bubbles: true }));

    const form = pass.closest('form');
    const submit = form && form.querySelector('button[type="submit"], input[type="submit"], button');
    if (submit) {
        submit.click();
    } else if (form) {
        form.submit();
    }
    return { ok: true };
})
