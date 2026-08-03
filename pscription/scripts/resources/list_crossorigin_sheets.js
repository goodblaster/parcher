// Lists the stylesheets whose rules the page cannot read.
//
// A stylesheet served from another origin without CORS headers still LOADS
// and still renders, but `sheet.cssRules` throws — so anything that inspects
// the CSSOM sees nothing in it. That silence is the problem: measured on
// test-site-2, 5 of 90 sheets were unreadable, and they held the
// @font-face rules for Orbitron (its heading typeface), Ubuntu, Montserrat,
// Open Sans and Source Sans 3 — 9 of the 15 fonts the page actually loaded.
// The archive came out with the other 6 and rendered its headings in a
// system fallback.
//
// Fetching them server-side and inlining the text makes them readable, which
// is what inline_crossorigin_css.go does with this list.
(() => {
    const hrefs = [];
    for (const sheet of document.styleSheets) {
        if (!sheet.href) continue; // inline <style>: readable by definition
        try {
            // Touching cssRules is the test; a readable sheet needs nothing.
            if (sheet.cssRules) continue;
        } catch (e) {
            hrefs.push(sheet.href);
        }
    }
    return { hrefs };
})
