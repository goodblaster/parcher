// Removes iframes that carry nothing archivable, just before the embed
// steps. This is a COST step, not a cleanup step: a serializer that walks
// frames (SingleFile) gives every surviving iframe a full recursive
// serialization pass with its own resource fetching and timeouts, and
// tracker frames are the expensive majority. Measured on test-site-6,
// the five iframes present were a reCAPTCHA anchor, a Criteo ad-sync
// pixel and three srcless 0x0 frames; serializing them cost 5.0s of a
// 5.4s capture and contributed 1.9 KB to a 6 MB archive.
//
// A frame is junk if it is:
//   - invisible (display:none / visibility:hidden), or
//   - smaller than minSize in either dimension (pixels, sync frames), or
//   - served by a host in `hosts` (ads/analytics/tag managers — dead in a
//     static archive regardless of how much space they occupy).
// Everything else is kept: a visible, sized, non-tracker frame is treated
// as content, because it may be a map, a video embed or a live sample.
//
// Runs BEFORE freeze-iframes so the freeze budget (maxCount) is spent on
// real embeds rather than trackers. Shadow roots and same-origin child
// documents are pierced, matching locate_opaque_iframes.js.
//
// Returns { removed, hidden, tiny, tracker, kept } for logging.
function removeJunkIframes(minSize, hosts) {
    minSize = minSize || 32;
    hosts = hosts || [];
    const stats = { removed: 0, hidden: 0, tiny: 0, tracker: 0, kept: 0 };

    function walk(root, out) {
        for (const f of root.querySelectorAll('iframe')) {
            out.push(f);
            // A junk frame can host real frames; recurse before deciding.
            try {
                if (f.contentDocument) walk(f.contentDocument, out);
            } catch (e) { /* cross-origin: unreachable, nothing to recurse into */ }
        }
        for (const host of root.querySelectorAll('*')) {
            if (host.shadowRoot) walk(host.shadowRoot, out);
        }
    }

    // A frame's effective URL: .src resolves relative values, but srcless
    // frames (about:blank, JS-filled) have none — those are judged on
    // geometry alone.
    function frameHost(f) {
        const raw = f.src || f.getAttribute('src') || '';
        if (!raw || raw === 'about:blank') return '';
        try {
            return new URL(raw, f.ownerDocument.baseURI).hostname;
        } catch (e) {
            return '';
        }
    }

    // Suffix match on labels, so "criteo.com" also covers
    // "gum.criteo.com" without matching "notcriteo.com".
    function isTrackerHost(host) {
        if (!host) return false;
        return hosts.some((h) => host === h || host.endsWith('.' + h));
    }

    const frames = [];
    walk(document, frames);

    const doomed = [];
    for (const f of frames) {
        const view = f.ownerDocument.defaultView || window;
        const style = view.getComputedStyle(f);
        const rect = f.getBoundingClientRect();

        if (isTrackerHost(frameHost(f))) {
            stats.tracker++;
        } else if (style.display === 'none' || style.visibility === 'hidden') {
            stats.hidden++;
        } else if (rect.width < minSize || rect.height < minSize) {
            stats.tiny++;
        } else {
            stats.kept++;
            continue;
        }
        doomed.push(f);
    }

    for (const f of doomed) {
        if (!f.isConnected) continue; // an ancestor frame was already removed
        try {
            f.remove();
            stats.removed++;
        } catch (e) { /* detached mid-pass */ }
    }
    return stats;
}
