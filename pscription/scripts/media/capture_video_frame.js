// Replaces every <video> with a static <img> of its current frame, and
// YouTube embed iframes with their thumbnail image, so the capture shows a
// meaningful still instead of a black rectangle.
//
// Per video, in order of preference:
//   1. A real frame drawn to canvas (loading/seeking the video if needed,
//      each wait bounded by a timeout).
//   2. The poster image.
//   3. Hide the video (visibility:hidden) — an empty box that shows the
//      section background beats a black one.
//
// Must run BEFORE the image-embedding steps: the poster/thumbnail <img>
// elements it creates use external URLs that still need to be inlined.
async function captureVideoFrames() {
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

    // Wait for an event with a timeout; resolves true if the event fired.
    const awaitEvent = (target, events, timeoutMs) => new Promise((resolve) => {
        let done = false;
        const finish = (ok) => {
            if (done) return;
            done = true;
            for (const ev of events) target.removeEventListener(ev, onEvent);
            resolve(ok);
        };
        const onEvent = () => finish(true);
        for (const ev of events) target.addEventListener(ev, onEvent, { once: true });
        setTimeout(() => finish(false), timeoutMs);
    });

    const stats = { frames: 0, posters: 0, hidden: 0, youtube: 0 };

    function makeReplacement(video, src) {
        const img = document.createElement('img');
        img.src = src;
        img.alt = 'video frame';
        img.style.cssText = video.style.cssText;
        const c = video.getAttribute('class');
        if (c) img.setAttribute('class', c);

        if (video.hasAttribute('width')) img.setAttribute('width', video.getAttribute('width'));
        if (video.hasAttribute('height')) img.setAttribute('height', video.getAttribute('height'));
        if (!img.hasAttribute('width') && !img.hasAttribute('height')) {
            const rect = video.getBoundingClientRect();
            if (rect.width > 0 && rect.height > 0) {
                img.style.width = rect.width + 'px';
                img.style.height = rect.height + 'px';
            }
        }

        const computed = window.getComputedStyle(video);
        img.style.objectFit = computed.objectFit || 'cover';
        if (computed.objectPosition) img.style.objectPosition = computed.objectPosition;
        return img;
    }

    async function freezeVideo(video) {
        // Make sure the video is allowed to load and (briefly) play.
        video.muted = true;
        video.setAttribute('playsinline', '');
        if (video.preload === 'none') video.preload = 'auto';

        // Kick loading if it never started (e.g. lazy <source> children).
        if (video.readyState === 0 && (video.currentSrc || video.querySelector('source'))) {
            try { video.load(); } catch (e) { /* ignore */ }
        }

        // HAVE_CURRENT_DATA (2) means a frame is available.
        if (video.readyState < 2) {
            const p = video.play();
            if (p && p.catch) p.catch(() => {});
            await awaitEvent(video, ['loadeddata', 'canplay'], 2500);
        }

        let frameSrc = null;
        if (video.readyState >= 2 && video.videoWidth > 0) {
            // A tiny seek off t=0 avoids blank first frames.
            if (video.currentTime === 0) {
                try {
                    video.currentTime = 0.1;
                    await awaitEvent(video, ['seeked'], 1000);
                } catch (e) { /* ignore */ }
            }
            video.pause();
            await sleep(50); // let the frame settle

            const canvas = document.createElement('canvas');
            canvas.width = video.videoWidth;
            canvas.height = video.videoHeight;
            try {
                canvas.getContext('2d').drawImage(video, 0, 0, canvas.width, canvas.height);
                frameSrc = canvas.toDataURL('image/jpeg', 0.85);
            } catch (e) {
                // Tainted canvas (cross-origin video without CORS headers).
                frameSrc = null;
            }
        }

        if (frameSrc) {
            video.parentNode.replaceChild(makeReplacement(video, frameSrc), video);
            stats.frames++;
        } else if (video.poster) {
            video.parentNode.replaceChild(makeReplacement(video, video.poster), video);
            stats.posters++;
        } else {
            collapseVideo(video);
            stats.hidden++;
        }
    }

    // When a video can't be turned into a still (no frame, no poster), it
    // must be COLLAPSED, not merely hidden: visibility:hidden keeps the
    // element's box, leaving a tall blank gap in the archive (a hero video
    // is often ~800px). display:none removes it from layout so the page
    // flows naturally.
    function collapseVideo(video) {
        if (!video || !video.isConnected) return;
        try { video.pause(); } catch (e) { /* ignore */ }
        video.style.setProperty('display', 'none', 'important');
    }

    const videos = Array.from(document.querySelectorAll('video'))
        .filter((v) => v.parentNode);
    await Promise.all(videos.map((v) =>
        freezeVideo(v).catch(() => { collapseVideo(v); stats.hidden++; })));

    // YouTube embeds → thumbnail image (inlined by a later embed step).
    for (const iframe of document.querySelectorAll(
        'iframe[src*="youtube.com/embed/"], iframe[src*="youtube-nocookie.com/embed/"]')) {
        const m = (iframe.getAttribute('src') || '').match(/\/embed\/([^/?#]+)/);
        if (!m) continue;

        const img = document.createElement('img');
        img.src = 'https://img.youtube.com/vi/' + m[1] + '/hqdefault.jpg';
        img.alt = 'Video thumbnail';
        const c = iframe.getAttribute('class');
        if (c) img.setAttribute('class', c);
        const rect = iframe.getBoundingClientRect();
        if (rect.width > 0 && rect.height > 0) {
            img.style.width = rect.width + 'px';
            img.style.height = rect.height + 'px';
            img.style.objectFit = 'cover';
        }
        iframe.parentNode.replaceChild(img, iframe);
        stats.youtube++;
    }

    return stats;
}
