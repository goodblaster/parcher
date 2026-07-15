// Neutralizes viewport-relative heights (100vh and calc() expressions using
// it) that cause huge blank regions in full-page captures. Stylesheet rules
// are rewritten and inline styles adjusted.
(() => {
    let stylesheets = 0;
    let inline = 0;

    // Each pattern keeps the rule's terminator ([;}]) intact — a capture
    // group carries it through so the CSS block structure is never broken.
    const patterns = [
        [/(^|[;{\s])height\s*:\s*100vh\s*([;}])/gi, '$1height: auto$2'],
        [/(^|[;{\s])min-height\s*:\s*100vh\s*([;}])/gi, '$1min-height: 0$2'],
        [/(^|[;{\s])max-height\s*:\s*100vh\s*([;}])/gi, '$1max-height: none$2'],
        [/(^|[;{\s])height\s*:\s*calc\([^)]*100vh[^)]*\)\s*([;}])/gi, '$1height: auto$2'],
        [/(^|[;{\s])min-height\s*:\s*calc\([^)]*100vh[^)]*\)\s*([;}])/gi, '$1min-height: 0$2'],
        [/(^|[;{\s])max-height\s*:\s*calc\([^)]*100vh[^)]*\)\s*([;}])/gi, '$1max-height: none$2'],
    ];

    for (const style of document.querySelectorAll('style')) {
        if (!style.textContent) continue;
        let css = style.textContent;
        const before = css;
        for (const [re, replacement] of patterns) {
            css = css.replace(re, replacement);
        }
        if (css !== before) {
            style.textContent = css;
            stylesheets++;
        }
    }

    for (const el of document.querySelectorAll('[style*="vh"]')) {
        const style = el.style;
        if (style.height && style.height.includes('100vh')) {
            style.height = 'auto';
            inline++;
        }
        if (style.minHeight && style.minHeight.includes('100vh')) {
            style.minHeight = '0';
            inline++;
        }
        if (style.maxHeight && style.maxHeight.includes('100vh')) {
            style.maxHeight = 'none';
            inline++;
        }
    }

    return { stylesheets, inline };
})
