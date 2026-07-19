# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**pscription** is a browser script-running and page-capture library; **rx** is its CLI. A "pscription" (like a doctor's ℞ — Latin *recipe*, "take thou") is an ordered list of preparation steps and scripts executed against a live page. It drives headless Chrome (chromedp), runs a pipeline of named preparation steps (dismiss dialogs, defeat lazy loading, freeze videos and canvases to stills, embed resources as data URIs), and captures the prepared page as a single self-contained snapshot.

**Primary Language:** Go 1.24
**Runtime requirement:** a Chrome/Chromium installation (not bundled).

This repo (formerly "webrun") was extracted from `goodblaster/zim` (working dir `zim`, formerly module `webtools`). A second tool — parchmint (binary `parch`, the page archiver) — will consume this library; its parked code (OCR engines, text search, MHT indexing) still lives in the old repo under `pkg/`.

## Build & Run

```bash
go build -o bin/ ./cmd/...
./bin/rx https://example.com                # → example.com.html (SingleFile backend)
./bin/rx -backend mht|screenshot ...
./bin/rx -links keep|new-tab|disable ...    # default keep: links untouched
./bin/rx -v ...                             # debug logging (per-step timings)
./bin/rx https://example.com > page.html    # content → stdout when piped; logs → stderr always
```

No test suite yet. Verify changes by capturing the three reference sites and screenshotting them offline (see "Verification" below).

## Architecture

Capture flow: **runner** (browser session) → **pipeline** (preparation recipe) → **capture** (serialize snapshot).

- **`runner/`** — Browser lifecycle: chromedp context, viewport/user-agent, navigation, auto-accepted JS dialogs. `Config.PreNavigate` runs actions before the page exists (cookie seeding, evaluate-on-new-document scripts).
- **`pipeline/`** — `Step{Name, Action, ContinueOnError}`, `Recipe{Name, PreNavigate, Steps}` with per-step logging/timing. `Staticize()` is the default page-prep recipe. `Register/Lookup` step registry exists so recipes can become data (per-site overrides) without recompiling. `RewriteLinksStep(mode)` is an optional final step.
- **`capture/`** — `Capture(ctx, url, recipe, backend) → *Snapshot{URL, Title, MIME, Bytes}`. Backends: `SingleFile{}` (vendored single-file-core bundle, preferred), `MHT{}` (Chrome MHTML + quoted-printable data-URI repair), `Screenshot{}` (full-page PNG).
- **`actions/`** — Composable `chromedp.ActionFunc` building blocks (dialog dismissal, waits, server-side resource fetching/injection, media freezing, consent pre-seeding).
- **`scripts/`** — Embedded page-context JS + the `ScriptFile` engine: JSON param injection, async detection (leading comments are stripped — a script is async iff its first code token starts `async function`; async scripts are properly awaited), dev-mode reload from disk (`gb_env=dev`). See [SCRIPTS.md](SCRIPTS.md).
- **`agents/`** — User-agent constants.
- **`internal/`** — common (CLI init, .env), config, log. Logs always go to **stderr**.

## Dialog clearing is layered — keep this order when tuning

0. **Prevent** (pre-navigation): `actions.PreseedConsentCookies` + `PreseedConsentStorage` (localStorage via `Page.addScriptToEvaluateOnNewDocument`, runs before page JS, survives reloads). Most banners/age gates never render.
1. **Known CMPs by exact selector**: `dismiss_cmp.js` (~20 platforms incl. Usercentrics shadow DOM; Sourcepoint's cross-origin iframe is removed, not clicked).
2. **Heuristics**: `dismiss_popups.js` — visible-only clicks, overlay-scoped generic words.
3. **Demolition** at capture time: `remove_blocking_overlays.js` — geometry-based, with text-share guards so fixed heroes survive.

## Hard-won invariants (violating these caused real bugs)

- SingleFile capture must pass `blockScripts: true` (this vendored bundle's option name — it has no `removeScripts`). Keeping site JS means age gates re-run when the snapshot is opened.
- Media freezing (`capture-video-frame`, `rasterize-font-icons`) must run BEFORE the embed steps — the `<img>`s they create (posters, YouTube thumbnails) carry external URLs that still need inlining. (`rasterize-canvases` is exempt: it produces data URIs directly.)
- WebGL canvases are only readable because the `preserve_webgl_buffer.js` pre-navigation script forces `preserveDrawingBuffer` before any page JS runs; a WebGL context created without it reads back transparent black forever. Never demote it to an ordinary step.
- Chrome throttles rAF for offscreen content, and offscreen cross-origin iframes (OOPIFs) are not rasterized AT ALL by full-page screenshots — `captureBeyondViewport` included. Hence `warm-canvases` (scroll blank canvases into view until they paint; a same-process canvas keeps its last frame) and `freeze-iframes` (screenshot each cross-origin iframe WHILE it is composited in-viewport, then swap in the still). Dwelling alone is useless for OOPIFs — their pixels don't persist offscreen; the shot must happen while visible.
- MDN-style pages put their sample iframes inside open shadow roots — any script that enumerates canvases/iframes must pierce `shadowRoot`s, or it silently finds nothing.
- Chrome's WebP encoder hard-fails (empty bytes, no error) above 16383px per dimension; `captureFullImage` clamps height and warns. Zero-byte screenshots are treated as errors, never written.
- `actions.BypassCSP` (pre-navigation) is load-bearing for ALL injected data: URIs. A site CSP without data: in img-src silently blocks every injected image on the live page (`complete=true, naturalWidth=0`, broken-image placeholder) — which only shows up in the live-rendering backends (pdf/png/jpeg/webp), because saved HTML carries no CSP header and renders the same data URIs fine. test-site-4 (default-src 'self') is the reference case; debug with `document.querySelector('img').naturalWidth`.
- Server-side fetchers must NOT set `Accept-Encoding` manually — it disables Go's transparent gzip decompression and gzip-served SVGs get embedded as compressed garbage.
- `inject_images.js` removes `<source>` siblings inside `<picture>` — otherwise the external source keeps winning over the injected data URL.
- Never remove elements matched by class alone without a structural guard — sites put state classes like `age-gate-open` on `<body>`.
- An age-gate click can trigger a reload: `actions.WaitForReady` absorbs it, and injected CSS must be re-applied afterwards.
- **`goodblaster/errors.Wrap(nil, msg)` returns a NON-NIL error.** Never tail-return `errors.Wrap` unguarded.

## Verification

Reference sites: `https://www.test-site-1` (videos, product tiles), `https://test-site-2` (age gate, hero video, carousel), `https://test-site-8/wiki/test-site-7:%E9%A6%96%E9%A1%B5` (CJK, logo SVGs). Capture each, then screenshot **offline** so missing embeds fail visibly:

```bash
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new \
  --proxy-server="http://127.0.0.1:9" --hide-scrollbars --window-size=1600,3000 \
  --virtual-time-budget=8000 --screenshot=out.png "file://$PWD/capture.html"
```

Good captures have ~zero `src="http` references (grep) and no `<script>` tags except `application/ld+json`.

Keep `--virtual-time-budget`: sites with entry CSS animations (e.g. test-site-5 fades everything in from `opacity:0` over ~1.5s) screenshot as a blank page without it — the archive is fine, the screenshot just fired too early.

For canvas/WebGL work, `https://test-site-4` (WebGL cube) and `https://www.test-site-5` (2D chart canvases) are the reference sites.

## Conventions

- Error wrapping via `errors.Wrap(err, "context")` (github.com/goodblaster/errors) — guarded, see invariant above.
- New prep behavior = a named pipeline step (JS file → `ScriptFile` constant in `scripts/embed.go` → action wrapper → step in `pipeline/staticize.go` + `Register`), not inline workflow code.
- New capture format: implement `capture.Backend` (`Name`, `Ext`, `Action(snap)`).
- Scripts return a small stats object; it lands in step logs.
- CLI: logs → stderr, content → stdout/file.

## Known cleanup remaining

- `actions/` could consolidate ~19 small files into ~6 thematic ones.
- `single-file-bundle.js` is AGPL-3.0-or-later (see README license note) — relevant if binaries are distributed or offered as a network service; escape hatch is a native HTML serializer backend built on the existing fetch/embed machinery.
