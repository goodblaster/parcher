// Return css selectors for all SVGs on the page.
(() => {
    function getInlineSVGSelectors() {
        const elementsWithInlineSVG = [];

        // Check all <svg> elements
        document.querySelectorAll('svg').forEach(svg => {
            if (svg.hasAttribute('xmlns:xlink')) {
                elementsWithInlineSVG.push(getUniqueSelector(svg));
            }
        });

        return elementsWithInlineSVG;
    }

    function getUniqueSelector(el) {
        if (!el || el.nodeType !== 1) return '';

        let path = [];
        while (el.parentElement) {
            let selector = el.tagName.toLowerCase();
            if (el.id) {
                selector += `#${el.id}`;
                path.unshift(selector);
                break;
            } else {
                let siblings = Array.from(el.parentElement.children);
                let sameTagSiblings = siblings.filter(sib => sib.tagName === el.tagName);
                let index = sameTagSiblings.indexOf(el) + 1;
                if (sameTagSiblings.length > 1) {
                    selector += `:nth-of-type(${index})`;
                }
            }
            path.unshift(selector);
            el = el.parentElement;
        }

        return path.join(' > ');
    }

    return getInlineSVGSelectors();
})