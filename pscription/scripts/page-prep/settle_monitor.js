// Runs on every new document BEFORE the page's own JavaScript (registered
// via Page.addScriptToEvaluateOnNewDocument). Records the time of the last
// DOM mutation, continuously, for the whole life of the document.
//
// This exists so a quiet-wait can answer "has this page been quiet for N
// ms?" from HISTORY. A wait that installs its own observer can only answer
// prospectively — it must sit through the full window watching nothing
// happen — so a page that finished rendering seconds ago pays exactly as
// much as one still mid-render. The record makes an already-settled page
// free.
//
// Surviving reloads is the point of registering it as a new-document script:
// an age-gate click that reloads the page gets a fresh observer with it.
//
// The observer stays attached for the whole capture and therefore also sees
// prep's OWN edits (injected CSS, embedded images). That is deliberate — a
// wait placed after a step that changes the DOM should let the page react to
// that change — but it does mean the record cannot distinguish our writes
// from the page's.
(() => {
    if (window.__gbSettle) return;

    const rec = {
        // performance.now() of the most recent mutation. Starts at document
        // start, so a document that never mutates reads as quiet since 0.
        last: performance.now(),
        // Total MutationRecords seen; diagnostic only.
        count: 0,
    };

    // Non-enumerable so page code walking window doesn't trip over it.
    Object.defineProperty(window, '__gbSettle', {
        value: rec,
        configurable: true,
        enumerable: false,
        writable: false,
    });

    // Observing `document` (not documentElement) covers the whole tree
    // including a documentElement that has not been parsed yet.
    new MutationObserver((records) => {
        rec.last = performance.now();
        rec.count += records.length;
    }).observe(document, {
        childList: true,
        subtree: true,
        attributes: true,
        characterData: true,
    });
})();
