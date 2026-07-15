async function inlineIframes(maxDepth = 2, currentDepth = 0) {
    let inlined = 0;
    let skipped = 0;
    let errors = 0;

    if (currentDepth >= maxDepth) {
        console.log('Max iframe nesting depth reached');
        return { inlined, skipped, errors, maxDepthReached: true };
    }

    // Get all iframes in the current document
    const iframes = Array.from(document.querySelectorAll('iframe'));

    for (const iframe of iframes) {
        try {
            // Skip if already processed
            if (iframe.hasAttribute('data-inlined')) {
                skipped++;
                continue;
            }

            // Try to access iframe content
            let iframeDoc;
            try {
                iframeDoc = iframe.contentDocument || iframe.contentWindow?.document;
            } catch (e) {
                // Cross-origin iframe, can't access
                console.warn('Cannot access cross-origin iframe:', iframe.src, e);
                skipped++;
                continue;
            }

            if (!iframeDoc) {
                skipped++;
                continue;
            }

            // Get the complete HTML of the iframe
            const iframeHTML = iframeDoc.documentElement.outerHTML;

            // Create a div to replace the iframe
            const div = document.createElement('div');
            div.className = iframe.className;
            div.setAttribute('data-original-iframe', 'true');
            div.setAttribute('data-original-src', iframe.src || 'about:blank');

            // Copy styles
            const iframeStyle = window.getComputedStyle(iframe);
            div.style.width = iframe.style.width || iframeStyle.width;
            div.style.height = iframe.style.height || iframeStyle.height;
            div.style.border = iframe.style.border || iframeStyle.border;
            div.style.display = iframeStyle.display;

            // Set the content
            div.innerHTML = iframeHTML;

            // Replace iframe with div
            iframe.parentNode.replaceChild(div, iframe);
            inlined++;

            // Recursively process iframes in the inlined content
            if (currentDepth + 1 < maxDepth) {
                const nestedIframes = div.querySelectorAll('iframe');
                if (nestedIframes.length > 0) {
                    // Temporarily make the div the document context
                    // Note: This is a simplified approach; full recursion would need
                    // to process each nested iframe separately
                    console.log(`Found ${nestedIframes.length} nested iframes at depth ${currentDepth + 1}`);
                }
            }
        } catch (e) {
            console.warn('Error processing iframe:', e);
            errors++;
        }
    }

    console.log(`Iframe inlining: ${inlined} inlined, ${skipped} skipped, ${errors} errors`);
    return { inlined, skipped, errors, depth: currentDepth };
}
