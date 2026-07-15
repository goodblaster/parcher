(() => {
    let getVisibleText = function (node) {
        // If the node is a text node, return its content
        if (node.nodeType === Node.TEXT_NODE) {
            return node.nodeValue;
        }

        // If the node is not an element node, skip it
        if (node.nodeType !== Node.ELEMENT_NODE) {
            return '';
        }

        // Skip <script>, <style>, and other non-visible tags
        const excludedTags = ['SCRIPT', 'STYLE'];
        if (excludedTags.includes(node.nodeName)) {
            return '';
        }

        // // Check for "hidden" attribute or inline styles for visibility
        // const hiddenAttributes = ['hidden'];
        // if (
        //     hiddenAttributes.some(attr => node.hasAttribute(attr)) ||
        //     (node.style && (node.style.display === 'none' || node.style.visibility === 'hidden'))
        // ) {
        //     return '';
        // }

        // Recurse through child nodes and gather visible text
        let text = '';
        for (const child of node.childNodes) {
            text += getVisibleText(child);
        }

        // Add a line break for block-level elements
        const blockTags = ['DIV', 'P', 'SECTION', 'ARTICLE', 'TABLE', 'TR', 'UL', 'OL', 'LI', 'HEADER', 'FOOTER'];
        if (blockTags.includes(node.nodeName)) {
            text += '\n';
        }

        return text;
    }

    let processVisibleText = function (visibleText) {
        return visibleText
            .split('\n')            // Split text by line breaks
            .map(line => line.trim()) // Trim each line
            .filter(line => line.length > 0); // Exclude empty lines
    }

    const visibleText = getVisibleText(document.body).trim();
    return processVisibleText(visibleText);
})