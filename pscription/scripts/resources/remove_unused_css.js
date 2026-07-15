async function removeUnusedCSS() {
    let rulesRemoved = 0;
    let rulesKept = 0;
    let stylesheetsProcessed = 0;

    // Collect all elements from regular DOM and Shadow DOM
    function collectAllElements(root = document) {
        const elements = new Set();

        // Add the root if it's an element
        if (root.nodeType === Node.ELEMENT_NODE) {
            elements.add(root);
        }

        // Get all elements from current root
        if (root.querySelectorAll) {
            const found = root.querySelectorAll('*');
            found.forEach(el => elements.add(el));
        }

        // Traverse shadow roots
        const elementsArray = Array.from(elements);
        for (const element of elementsArray) {
            if (element.shadowRoot) {
                const shadowElements = collectAllElements(element.shadowRoot);
                shadowElements.forEach(el => elements.add(el));
            }
        }

        return elements;
    }

    // Check if a selector matches any elements
    function selectorMatchesAny(selector, allElements) {
        // Skip some selectors that are always needed
        if (!selector || selector.trim() === '') return true;

        // Keep @-rules (they don't have selectors in the traditional sense)
        if (selector.startsWith('@')) return true;

        // Try to check if selector matches any element
        try {
            // Handle pseudo-elements by removing them for matching
            // (::before, ::after, etc. can't be matched via querySelectorAll)
            const cleanSelector = selector
                .replace(/::(before|after|first-line|first-letter|selection|backdrop|placeholder|marker|cue)/g, '')
                .replace(/::([\w-]+)/g, '') // Remove any other pseudo-elements
                .trim();

            if (!cleanSelector) {
                // If only pseudo-element remains, keep the rule
                return true;
            }

            // Check against all elements
            for (const element of allElements) {
                try {
                    if (element.matches && element.matches(cleanSelector)) {
                        return true;
                    }
                } catch (e) {
                    // Complex selectors might fail, keep them to be safe
                    return true;
                }
            }

            // Also try querySelectorAll from document (catches some edge cases)
            try {
                if (document.querySelector(cleanSelector)) {
                    return true;
                }
            } catch (e) {
                // If selector is invalid, keep it (might be vendor-specific)
                return true;
            }

            return false;
        } catch (error) {
            // If we can't check the selector, keep it to be safe
            console.warn('Error checking selector:', selector, error);
            return true;
        }
    }

    // Process a stylesheet and remove unused rules
    function processStylesheet(stylesheet, allElements) {
        try {
            if (!stylesheet.cssRules && !stylesheet.rules) {
                return;
            }

            const rules = Array.from(stylesheet.cssRules || stylesheet.rules);
            const rulesToRemove = [];

            for (let i = 0; i < rules.length; i++) {
                const rule = rules[i];

                // Handle different rule types
                if (rule.type === CSSRule.STYLE_RULE) {
                    // Regular style rule
                    const selector = rule.selectorText;
                    if (!selectorMatchesAny(selector, allElements)) {
                        rulesToRemove.push(i);
                        rulesRemoved++;
                    } else {
                        rulesKept++;
                    }
                } else if (rule.type === CSSRule.MEDIA_RULE) {
                    // Process rules inside @media
                    const mediaRules = Array.from(rule.cssRules || []);
                    const mediaRulesToRemove = [];

                    for (let j = 0; j < mediaRules.length; j++) {
                        const mediaRule = mediaRules[j];
                        if (mediaRule.type === CSSRule.STYLE_RULE) {
                            if (!selectorMatchesAny(mediaRule.selectorText, allElements)) {
                                mediaRulesToRemove.push(j);
                                rulesRemoved++;
                            } else {
                                rulesKept++;
                            }
                        }
                    }

                    // Remove rules in reverse order
                    mediaRulesToRemove.reverse().forEach(index => {
                        try {
                            rule.deleteRule(index);
                        } catch (e) {
                            console.warn('Failed to delete media rule:', e);
                        }
                    });

                    // If media rule is now empty, mark it for removal
                    if (rule.cssRules.length === 0) {
                        rulesToRemove.push(i);
                    }
                } else if (rule.type === CSSRule.SUPPORTS_RULE) {
                    // Process rules inside @supports
                    const supportsRules = Array.from(rule.cssRules || []);
                    const supportsRulesToRemove = [];

                    for (let j = 0; j < supportsRules.length; j++) {
                        const supportsRule = supportsRules[j];
                        if (supportsRule.type === CSSRule.STYLE_RULE) {
                            if (!selectorMatchesAny(supportsRule.selectorText, allElements)) {
                                supportsRulesToRemove.push(j);
                                rulesRemoved++;
                            } else {
                                rulesKept++;
                            }
                        }
                    }

                    // Remove rules in reverse order
                    supportsRulesToRemove.reverse().forEach(index => {
                        try {
                            rule.deleteRule(index);
                        } catch (e) {
                            console.warn('Failed to delete supports rule:', e);
                        }
                    });

                    // If supports rule is now empty, mark it for removal
                    if (rule.cssRules.length === 0) {
                        rulesToRemove.push(i);
                    }
                } else {
                    // Keep @font-face, @keyframes, @import, etc.
                    rulesKept++;
                }
            }

            // Remove unused rules in reverse order (to maintain indices)
            rulesToRemove.reverse().forEach(index => {
                try {
                    stylesheet.deleteRule(index);
                } catch (e) {
                    console.warn('Failed to delete rule:', e);
                }
            });

            stylesheetsProcessed++;
        } catch (error) {
            console.warn('Error processing stylesheet:', error);
        }
    }

    // Collect all stylesheets (including from shadow DOM)
    function collectStylesheets(root = document) {
        const sheets = [];

        // Get stylesheets from current root
        if (root.styleSheets) {
            sheets.push(...Array.from(root.styleSheets));
        }

        // Get inline styles and style elements
        if (root.querySelectorAll) {
            const styleElements = root.querySelectorAll('style');
            styleElements.forEach(style => {
                if (style.sheet) {
                    sheets.push(style.sheet);
                }
            });
        }

        // Traverse shadow roots
        const elements = root.querySelectorAll ? Array.from(root.querySelectorAll('*')) : [];
        for (const element of elements) {
            if (element.shadowRoot) {
                sheets.push(...collectStylesheets(element.shadowRoot));
            }
        }

        return sheets;
    }

    try {
        // Collect all elements once
        console.log('Collecting all elements...');
        const allElements = collectAllElements();
        console.log(`Found ${allElements.size} elements`);

        // Collect all stylesheets
        console.log('Collecting stylesheets...');
        const stylesheets = collectStylesheets();
        console.log(`Found ${stylesheets.length} stylesheets`);

        // Process each stylesheet
        for (const stylesheet of stylesheets) {
            processStylesheet(stylesheet, allElements);
        }

        console.log(`CSS optimization complete: removed ${rulesRemoved} rules, kept ${rulesKept} rules`);
        return {
            removed: rulesRemoved,
            kept: rulesKept,
            stylesheets: stylesheetsProcessed
        };
    } catch (error) {
        console.error('Error in removeUnusedCSS:', error);
        return {
            removed: rulesRemoved,
            kept: rulesKept,
            stylesheets: stylesheetsProcessed,
            error: error.message
        };
    }
}
