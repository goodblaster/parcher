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

    // Picks an encoding that preserves what the frame actually contains.
    //
    // JPEG has NO alpha channel, so a transparent video flattens to BLACK —
    // and cutout animations (a subject over the page background, with no
    // backdrop of its own) are common: test-site-1's are WebM measuring 40-54%
    // transparent pixels. Encoding those as JPEG replaces the section
    // background with a black box.
    //
    // Alpha is sampled from a small downscale rather than the full frame:
    // downscaling averages alpha, so any transparency still shows up, and a
    // 3008x692 getImageData is 8 MB of work per video.
    function encodeFrame(canvas) {
        let alpha = false;
        try {
            const probe = document.createElement('canvas');
            probe.width = 48;
            probe.height = 48;
            const pc = probe.getContext('2d');
            pc.drawImage(canvas, 0, 0, probe.width, probe.height);
            const d = pc.getImageData(0, 0, probe.width, probe.height).data;
            for (let i = 3; i < d.length; i += 4) {
                if (d[i] < 250) { alpha = true; break; }
            }
        } catch (e) { /* tainted probe: fall through to jpeg */ }

        if (!alpha) return canvas.toDataURL('image/jpeg', 0.85);
        // WebP keeps alpha at a fraction of PNG's size for photographic
        // frames. toDataURL silently falls back to PNG when a format is
        // unsupported, so confirm rather than assume.
        const webp = canvas.toDataURL('image/webp', 0.9);
        return webp.startsWith('data:image/webp') ? webp : canvas.toDataURL('image/png');
    }

    function makeReplacement(video, src) {
        const img = document.createElement('img');
        img.src = src;
        img.alt = 'video frame';
        // Inline styles and class/id are still worth carrying: class and id
        // selectors keep matching an <img>. Only TAG selectors are lost —
        // which is exactly why adoptBox pins the box explicitly afterwards.
        img.style.cssText = video.style.cssText;
        if (video.id) img.id = video.id;
        const c = video.getAttribute('class');
        if (c) img.setAttribute('class', c);
        adoptBox(video, img);
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

            // Capture at the size the still will be DISPLAYED at, times the
            // device pixel ratio — never at the video's intrinsic size, which
            // is unrelated to it. A 720p source in a 1600px hero was being
            // captured at 1280 wide and upscaled; at --dpr 2 every still was
            // effectively half resolution. Never ask for more pixels than the
            // source has, and take fewer when the slot is smaller.
            const dpr = window.devicePixelRatio || 1;
            const shown = video.getBoundingClientRect().width || video.videoWidth;
            const scale = Math.min(1, Math.max(1, Math.round(shown * dpr)) / video.videoWidth);
            const canvas = document.createElement('canvas');
            canvas.width = Math.max(1, Math.round(video.videoWidth * scale));
            canvas.height = Math.max(1, Math.round(video.videoHeight * scale));
            try {
                canvas.getContext('2d').drawImage(video, 0, 0, canvas.width, canvas.height);
                frameSrc = encodeFrame(canvas);
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
    //
    // hqdefault (480x360) is the only size guaranteed to exist, but it is 4:3
    // and lands badly upscaled in a wide 16:9 slot. Pick by the size the embed
    // is actually SHOWN at, and verify a larger variant exists before
    // committing — YouTube answers a missing size with a 120x90 placeholder
    // rather than an error, so a load alone is not proof.
    // YouTube's thumbnail variants are NOT all the same shape:
    //   maxresdefault 1280x720  16:9   (true frame)
    //   sddefault      640x480  4:3    (letterboxed — black bars baked in)
    //   hqdefault      480x360  4:3    (letterboxed)
    //   mqdefault      320x180  16:9   (true frame)
    // Video slots are almost always 16:9, so picking purely by width lands a
    // 4:3 image in a 16:9 box. Prefer the true-frame variants; the 4:3 ones
    // are a last resort, where object-fit:cover crops their bars rather than
    // stretching the picture.
    const ytSizes = [
        { name: 'maxresdefault', w: 1280, wide: true },
        { name: 'mqdefault', w: 320, wide: true },
        { name: 'sddefault', w: 640, wide: false },
        { name: 'hqdefault', w: 480, wide: false },
    ];

    const usable = (url) => new Promise((resolve) => {
        const probe = new Image();
        let settled = false;
        const done = (ok) => { if (!settled) { settled = true; resolve(ok); } };
        probe.onload = () => done(probe.naturalWidth > 120);
        probe.onerror = () => done(false);
        setTimeout(() => done(false), 2000);
        probe.src = url;
    });

    async function bestThumb(id, needW) {
        const wide = ytSizes.filter((c) => c.wide);
        // Smallest true-frame variant that still covers the slot, then the
        // larger true-frame ones, then the 4:3 fallbacks (hqdefault always
        // exists).
        const covering = wide.filter((c) => c.w >= needW);
        const order = covering.length
            ? [covering[covering.length - 1], ...wide.filter((c) => c !== covering[covering.length - 1])]
            : [...wide].sort((a, b) => b.w - a.w);
        for (const c of order.concat(ytSizes.filter((c) => !c.wide))) {
            const url = 'https://img.youtube.com/vi/' + id + '/' + c.name + '.jpg';
            if (await usable(url)) return url;
        }
        return 'https://img.youtube.com/vi/' + id + '/hqdefault.jpg';
    }

    const frames = Array.from(document.querySelectorAll(
        'iframe[src*="youtube.com/embed/"], iframe[src*="youtube-nocookie.com/embed/"]'));
    for (const iframe of frames) {
        const m = (iframe.getAttribute('src') || '').match(/\/embed\/([^/?#]+)/);
        if (!m) continue;

        const dpr = window.devicePixelRatio || 1;
        const shown = iframe.getBoundingClientRect().width ||
            parseFloat(iframe.getAttribute('width')) || 0;

        const img = document.createElement('img');
        img.src = await bestThumb(m[1], Math.round(shown * dpr));
        img.alt = 'Video thumbnail';
        if (iframe.id) img.id = iframe.id;
        const c = iframe.getAttribute('class');
        if (c) img.setAttribute('class', c);
        adoptBox(iframe, img);
        iframe.parentNode.replaceChild(img, iframe);
        stats.youtube++;
    }

    return stats;

    // Transfers the replaced element's COMPUTED box onto its replacement.
    //
    // Authored styling cannot carry a box across a tag change: cssText holds only
    // INLINE styles, and any selector written for `video`/`iframe`/`canvas` stops
    // matching the moment the element becomes an <img>. The measured rect is also
    // zero whenever the element is not laid out — inside a carousel slide that has
    // never been shown, say — and an unsized replacement then renders at its
    // natural size: small and mispositioned inside a much larger slot.
    //
    // So: measure, fall back to the element's own attributes, and finally fill
    // whatever slot it lands in. Explicit inline dimensions also beat any class
    // rule that does still match, which keeps the result deterministic.
    //
    // Keep this identical across the substitution scripts — see CLAUDE.md,
    // "element → image substitution".
    function adoptBox(src, img) {
        const cs = window.getComputedStyle(src);
        const rect = src.getBoundingClientRect();
        let w = rect.width, h = rect.height;
        if (!(w > 0 && h > 0)) {
            w = parseFloat(src.getAttribute('width')) || 0;
            h = parseFloat(src.getAttribute('height')) || 0;
        }
        if (w > 0 && h > 0) {
            img.style.width = w + 'px';
            img.style.height = h + 'px';
        } else {
            img.style.width = '100%';
            img.style.height = '100%';
        }
        // Deliberately narrow: copying layout shorthands (display, margin) can
        // double-apply against rules that still match by class or id.
        // Out-of-flow placement must be carried as a COMPLETE group or not at
        // all. Absolutely positioned media is common (full-bleed heroes,
        // carousel tiles) and its parent often has no width of its own, so
        // dropping `position` collapses the layout. But copying the offsets
        // alone is worse: computed `left` is the page's placement already
        // resolved, and the `transform` that usually completes it
        // (left:50% + translateX(-50%)) must come with it or the image lands
        // shifted by half its width. In-flow elements are left alone —
        // the class and id we carry still match the page's own rules.
        if (cs.position === 'absolute' || cs.position === 'fixed') {
            // Offsets and translations are copied as PERCENTAGES, not the
            // resolved pixels. `left: 50%` computes to `left: 800px` at a
            // 1600px capture, and pinning that px value leaves the element
            // stuck there — so an archive opened in a wider window shows it
            // drifting left of centre, further the wider you go. The
            // percentages reproduce the captured position exactly (they are
            // derived from it) AND keep scaling the way the original fluid
            // layout did.
            const cb = src.offsetParent || src.parentElement;
            const cbr = cb ? cb.getBoundingClientRect() : { width: 0, height: 0 };
            const pct = (px, basis) => {
                const n = parseFloat(px);
                if (!isFinite(n) || !(basis > 0)) return null;
                return (n / basis * 100).toFixed(4) + '%';
            };

            img.style.position = cs.position;
            for (const [p, basis] of [['top', cbr.height], ['bottom', cbr.height],
                                      ['left', cbr.width], ['right', cbr.width]]) {
                const v = cs[p];
                if (!v || v === 'auto') continue;
                img.style[p] = pct(v, basis) || v;
            }
            if (cs.zIndex && cs.zIndex !== 'auto') img.style.zIndex = cs.zIndex;

            // A pure translation (the other half of the centring idiom)
            // becomes a percentage of the element's OWN box, which is what
            // CSS transform percentages resolve against.
            const m = /^matrix\(([^)]+)\)$/.exec(cs.transform || '');
            if (m) {
                const n = m[1].split(',').map(parseFloat);
                const pure = n.length === 6 && n[0] === 1 && n[1] === 0 && n[2] === 0 && n[3] === 1;
                if (pure && (n[4] || n[5])) {
                    const tx = w > 0 ? (n[4] / w * 100).toFixed(4) + '%' : n[4] + 'px';
                    const ty = h > 0 ? (n[5] / h * 100).toFixed(4) + '%' : n[5] + 'px';
                    img.style.transform = 'translate(' + tx + ', ' + ty + ')';
                } else if (cs.transform !== 'none') {
                    img.style.transform = cs.transform;
                    if (cs.transformOrigin) img.style.transformOrigin = cs.transformOrigin;
                }
            }
        }
        for (const p of ['objectFit', 'objectPosition', 'borderRadius', 'aspectRatio']) {
            const v = cs[p];
            // 'fill' is object-fit's INITIAL value, so every iframe/video/
            // canvas reports it whether or not the page asked for it —
            // and on an <img> it STRETCHES the picture to the box. Treat it
            // as unset, or a 4:3 still lands squished in a 16:9 slot.
            if (p === 'objectFit' && v === 'fill') continue;
            if (v && v !== 'auto' && v !== 'none' && v !== 'normal') img.style[p] = v;
        }
        if (!img.style.objectFit) img.style.objectFit = 'cover';
        return { width: w, height: h };
    }
}
