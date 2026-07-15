async function removeUnusedFonts() {
    let fontsRemoved = 0;
    let fontsKept = 0;

    // Collect all elements from regular DOM and Shadow DOM
    function collectAllElements(root = document) {
        const elements = new Set();

        if (root.nodeType === Node.ELEMENT_NODE) {
            elements.add(root);
        }

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

    // Collect all stylesheets (including from shadow DOM)
    function collectStylesheets(root = document) {
        const sheets = [];

        if (root.styleSheets) {
            sheets.push(...Array.from(root.styleSheets));
        }

        if (root.querySelectorAll) {
            const styleElements = root.querySelectorAll('style');
            styleElements.forEach(style => {
                if (style.sheet) {
                    sheets.push(style.sheet);
                }
            });
        }

        const elements = root.querySelectorAll ? Array.from(root.querySelectorAll('*')) : [];
        for (const element of elements) {
            if (element.shadowRoot) {
                sheets.push(...collectStylesheets(element.shadowRoot));
            }
        }

        return sheets;
    }

    // Extract font family name from @font-face rule
    function getFontFamilyName(rule) {
        try {
            if (rule.type === CSSRule.FONT_FACE_RULE && rule.style.fontFamily) {
                // Remove quotes and normalize
                return rule.style.fontFamily.replace(/['"]/g, '').trim().toLowerCase();
            }
        } catch (e) {
            console.warn('Error getting font family name:', e);
        }
        return null;
    }

    // Normalize font family name for comparison
    function normalizeFontName(fontName) {
        if (!fontName) return '';
        return fontName.replace(/['"]/g, '').trim().toLowerCase();
    }

    // Check if a font is used by any element
    function isFontUsed(fontFamily, allElements) {
        const normalizedFont = normalizeFontName(fontFamily);
        if (!normalizedFont) return false;

        // Check all elements
        for (const element of allElements) {
            try {
                const computedStyle = window.getComputedStyle(element);
                const fontFamilyList = computedStyle.fontFamily;

                if (fontFamilyList) {
                    // Font family can be a comma-separated list
                    const fonts = fontFamilyList.split(',').map(f => normalizeFontName(f));
                    if (fonts.includes(normalizedFont)) {
                        return true;
                    }
                }
            } catch (e) {
                // Element might not support getComputedStyle
                continue;
            }
        }

        return false;
    }

    // Process a stylesheet and collect/remove @font-face rules
    function processStylesheet(stylesheet, allElements) {
        try {
            if (!stylesheet.cssRules && !stylesheet.rules) {
                return;
            }

            const rules = Array.from(stylesheet.cssRules || stylesheet.rules);
            const rulesToRemove = [];

            for (let i = 0; i < rules.length; i++) {
                const rule = rules[i];

                if (rule.type === CSSRule.FONT_FACE_RULE) {
                    const fontFamily = getFontFamilyName(rule);
                    if (fontFamily && !isFontUsed(fontFamily, allElements)) {
                        rulesToRemove.push(i);
                        fontsRemoved++;
                    } else if (fontFamily) {
                        fontsKept++;
                    }
                } else if (rule.type === CSSRule.MEDIA_RULE || rule.type === CSSRule.SUPPORTS_RULE) {
                    // Check @font-face rules inside @media or @supports
                    const nestedRules = Array.from(rule.cssRules || []);
                    const nestedRulesToRemove = [];

                    for (let j = 0; j < nestedRules.length; j++) {
                        const nestedRule = nestedRules[j];
                        if (nestedRule.type === CSSRule.FONT_FACE_RULE) {
                            const fontFamily = getFontFamilyName(nestedRule);
                            if (fontFamily && !isFontUsed(fontFamily, allElements)) {
                                nestedRulesToRemove.push(j);
                                fontsRemoved++;
                            } else if (fontFamily) {
                                fontsKept++;
                            }
                        }
                    }

                    // Remove unused @font-face rules in reverse order
                    nestedRulesToRemove.reverse().forEach(index => {
                        try {
                            rule.deleteRule(index);
                        } catch (e) {
                            console.warn('Failed to delete font-face rule:', e);
                        }
                    });
                }
            }

            // Remove unused @font-face rules in reverse order
            rulesToRemove.reverse().forEach(index => {
                try {
                    stylesheet.deleteRule(index);
                } catch (e) {
                    console.warn('Failed to delete font-face rule:', e);
                }
            });
        } catch (error) {
            console.warn('Error processing stylesheet for fonts:', error);
        }
    }

    try {
        // Collect all elements
        console.log('Collecting elements for font analysis...');
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

        console.log(`Font optimization complete: removed ${fontsRemoved} fonts, kept ${fontsKept} fonts`);
        return {
            removed: fontsRemoved,
            kept: fontsKept
        };
    } catch (error) {
        console.error('Error in removeUnusedFonts:', error);
        return {
            removed: fontsRemoved,
            kept: fontsKept,
            error: error.message
        };
    }
}
