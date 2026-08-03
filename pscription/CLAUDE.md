# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**pscription** is a browser script-running and page-preparation library; **rx** is its CLI. A "pscription" (like a doctor's ℞ — Latin *recipe*, "take thou") is an ordered list of preparation steps and scripts executed against a live page. It drives headless Chrome (chromedp), runs a pipeline of named preparation steps (dismiss dialogs, defeat lazy loading, freeze videos/canvases/cross-origin iframes to stills, embed resources as data URIs), and extracts rendered text with word-level geometry (parchmint's text layer, including its capture-time highlighting and image recording).

Serialization lives downstream: [parchmint](../parchmint) (binary `parch`) owns the capture backends (HTML, MHT, PDF, screenshots) and consumes this library for browser driving and prep. The line between the repos: **pscription = live pages, parchmint = files.**

**Primary Language:** Go 1.24
**Runtime requirement:** a Chrome/Chromium installation (not bundled).

## Build & Run

```bash
go build -o bin/ ./cmd/...
./bin/rx https://news.site @article                  # built-in article extractor
./bin/rx https://shop.com dismiss.js prices.js:out   # your own scripts, capture "out"
./bin/rx -r login-and-scrape.rx https://portal.site  # .rx pscription file
./bin/rx --prep staticize -v https://example.com     # full prep, debug logging
```

stdout carries the JSON report (or one output raw with `--only`); logs go to stderr.

`go test ./scripts/` parses every embedded script with node (skipped when node is absent) — the only automated test here, and it exists because a script that fails to PARSE is invisible at runtime: prep steps are `stepIgnoreError`, so the exception is logged once and swallowed, and the page silently loses that step's behavior. Behavior changes are still verified by building parchmint's `parch` against this checkout (go.work links them) and running its verification flow.

## Architecture

Flow: **runner** (browser session) → **pipeline** (preparation recipe) → scripts/extraction; parchmint appends its capture backend to this flow.

- **`runner/`** — Browser lifecycle: chromedp context, viewport/user-agent, navigation, auto-accepted JS dialogs. `Config.PreNavigate` runs actions before the page exists (cookie seeding, evaluate-on-new-document scripts).
- **`pipeline/`** — `Step{Name, Action, ContinueOnError}`, `Recipe{Name, PreNavigate, Steps}` with per-step logging/timing. `Staticize()` is the default page-prep recipe. `Register/Lookup` step registry exists so recipes can become data (per-site overrides) without recompiling. `RewriteLinksStep(mode)` is an optional final step.
- **`pscription/`** — The `.rx` file format: parsing and running step lists, prep selection (`none`/`minimal`/`staticize`), variables, outputs.
- **`actions/`** — Composable `chromedp.ActionFunc` building blocks (dialog dismissal, waits, server-side resource fetching/injection, media freezing, consent pre-seeding, CSP bypass).
- **`scripts/`** — Embedded page-context JS + the `ScriptFile` engine: JSON param injection, async detection (leading comments are stripped — a script is async iff its first code token starts `async function`; async scripts are properly awaited), dev-mode reload from disk (`gb_env=dev`). See [SCRIPTS.md](SCRIPTS.md).
- **`agents/`** — User-agent constants.
- **`internal/`** — common (CLI init, .env), config, log. Logs always go to **stderr**. `internal/log` defaults to delegating to the logos package-level logger, so external consumers get this library's logs just by configuring logos; parchmint mirrors the same pattern with its own `internal/log`.

## Dialog clearing is layered — keep this order when tuning

0. **Prevent** (pre-navigation): `actions.PreseedConsentCookies` + `PreseedConsentStorage` (localStorage via `Page.addScriptToEvaluateOnNewDocument`, runs before page JS, survives reloads). Most banners/age gates never render.
1. **Known CMPs by exact selector**: `dismiss_cmp.js` (~20 platforms incl. Usercentrics shadow DOM; Sourcepoint's cross-origin iframe is removed, not clicked).
2. **Heuristics**: `dismiss_popups.js` — visible-only clicks, overlay-scoped generic words.
3. **Demolition** at capture time: `remove_blocking_overlays.js` — geometry-based, with text-share guards so fixed heroes survive.

## Hard-won invariants (violating these caused real bugs)

Capture-backend invariants (encoder limits, PDF quirks, what Chrome's MHTML writer drops) live in parchmint's CLAUDE.md; these are the prep-side ones.

- **A class the library ADDS at init cannot be the evidence that the library was supposed to run.** `detect_slider_defect.js` and `wait_for_sliders.js` both used `[class*="slick-"]` to decide whether a page has a carousel worth caring about — but slick writes `slick-slide`/`slick-track`/`slick-initialized` itself, so a total init failure leaves ZERO slick-* elements and looks exactly like a page with no carousel. Measured on test-site-2, whose review carousel is slick bundled inside an Elementor add-on (so `script[src*="slick"]` misses it too): roughly **1 run in 3** came back with `.slick-initialized=0`, `.slick-slide=0`, the carousel container **11986px tall instead of 796px**, the page 17039px instead of 5849px — and the detector reported `unused: true`, no defect, no retry, shipping a broken archive ~2 MB lighter than a good one. Detect the SHAPE instead: a carousel-classed container more than 3x the viewport tall with 3+ children is stacked, whoever's library it is. That threshold is nowhere near either edge (good measures 0.9x, broken 13x), it costs nothing on healthy pages (test-site-1 and test-site-5: zero detections, unchanged 9s captures), and the existing retry machinery does the rest — the failing run now logs `evidence="stacked:1:11986:17039"` and re-captures clean.
- **Never hide by substring-matching a short class prefix — over-hiding destroys content in total silence.** `hide_overlays.js` carried `[class*="om-element" i]` for OptinMonster. `om-` is two characters and belongs to nobody: it also matches `custom-element`, `zoom-element`, and — measured on test-site-2 — a video plugin's own `keywest-element keywest-ele-1 Element om-element`, four of them, which hid the site's **hero video and every review thumbnail**. The page archived with two large blank bands, no error anywhere, in every backend, for as long as the selector existed. Two things made it hard to find: the elements are created by page JS as the player initializes, so they do not exist at `--prep minimal` and a selector audit there comes back empty; and the loss is in PREP, so a `-f png` capture of the live prepared page shows the same blanks — which is the fastest way to tell prep damage from serialization loss. Selectors here must identify the framework (`[class*="optinmonster" i]`), with `om-*` matched only as a descendant of such a root. Under-hiding is cheap: a popup that survives is still caught at capture time by `dismiss-popups` and `remove-blocking-overlays`. Over-hiding is not recoverable.
- **A cross-origin stylesheet renders but cannot be READ, and that silence is invisible until something inspects the CSSOM.** `sheet.cssRules` throws on a sheet served from another origin without CORS headers, so every step that walks the CSSOM — font extraction above all — behaves as if the sheet were empty. Measured on test-site-2: 5 of 90 sheets were unreadable and held the @font-face rules for **Orbitron, its heading typeface**, plus Ubuntu, Montserrat, Open Sans and Source Sans 3 — 9 of the 15 fonts the page actually loaded. Archives came out with the other 6 and rendered every heading in a system fallback, and the offline screenshot diff against a reference capture was **15.2%**; with `inline-crossorigin-css` fetching those sheets server-side and inlining their text, it is **2.2%**. The inlined `<style>` goes immediately after its `<link>` (document order IS the cascade) and relative `url()`s are resolved against the sheet's own address first, since the copy now lives in the document. The `<link>` stays — it is what the page declared. This never bit parchmint's former SingleFile backend, which re-fetched such sheets through its own page-context fetch bridge; everything built on Chrome's snapshot needs it done for it, which is why the gap only appeared when that backend was retired.
- **A serializer sees MARKUP, so runtime-only state has to be written back into the DOM — and that is prep's job, not the backend's.** Three steps at the very END of `Staticize` exist for this, and they must stay last, because every step above them can still change the DOM and what gets written back has to be the final state. `materialize-form-state` turns a typed value, a ticked box and a chosen `<option>` into attributes (passwords deliberately excluded — an archive is a file that gets copied around); `<details open>` needs nothing, being an attribute already. `flatten-adopted-stylesheets` emits `document.adoptedStyleSheets` as a real `<style>` — measured, constructed sheets were dropped by **every** serializer tried, Chrome's MHTML writer and SingleFile alike, because CSSOM-only state is in no one's markup. `link-shadow-styles` re-expresses each shadow root's CSS as a `<link>` to a `data:` URL, because Chrome serializes shadow roots but drops `<style>` elements inside them while keeping a `<link>` in the same position — verified with a three-way probe (`<style>` lost, `<link>` survived, `adoptedStyleSheets` lost). It **keeps** the original `<style>`: removing it would leave the live page waiting on a stylesheet to load, and the backends that photograph the page could catch the flash. All three are layout-neutral by construction, so nothing needs to settle after them. Closed shadow roots are unreachable from page script, so their styling cannot be repaired — their content still serializes, which is more than a page-script serializer manages at all.

- **Junk iframes are a capture-time COST, not just clutter.** A DOM serializer that walks frames gives every surviving iframe a full recursive pass with its own resource fetching and timeouts. Measured against parchmint's since-retired SingleFile backend, so the numbers are an upper bound — Chrome's snapshot is cheaper — but the step still earns its place by keeping tracker frames out of archives. test-site-6's five frames (a reCAPTCHA anchor, a Criteo ad-sync pixel, three srcless 0x0) cost **5.0s of a 5.4s** serialization and contributed 1.9 KB to a 6 MB archive; test-site-2 went 7.0s → 2.0s. `remove-junk-iframes` recovers all of it and must stay before `freeze-iframes`. The predicate deletes, so it is deliberately conservative — visible, sized, non-tracker frames are content (MDN's shadow-DOM live samples: `kept=4, removed=0`, all four still frozen). Widen the host list before loosening the geometry test.
- Media freezing (`capture-video-frame`, `rasterize-font-icons`) must run BEFORE the embed steps — the `<img>`s they create (posters, YouTube thumbnails) carry external URLs that still need inlining. (`rasterize-canvases` is exempt: it produces data URIs directly.)
- WebGL canvases are only readable because the `preserve_webgl_buffer.js` pre-navigation script forces `preserveDrawingBuffer` before any page JS runs; a WebGL context created without it reads back transparent black forever. Never demote it to an ordinary step.
- Chrome throttles rAF for offscreen content, and offscreen cross-origin iframes (OOPIFs) are not rasterized AT ALL by full-page screenshots — `captureBeyondViewport` included. Hence `warm-canvases` (scroll blank canvases into view until they paint; a same-process canvas keeps its last frame) and `freeze-iframes` (screenshot each cross-origin iframe WHILE it is composited in-viewport, then swap in the still). Dwelling alone is useless for OOPIFs — their pixels don't persist offscreen; the shot must happen while visible.
- MDN-style pages put their sample iframes inside open shadow roots — any script that enumerates canvases/iframes must pierce `shadowRoot`s, or it silently finds nothing.
- `actions.BypassCSP` (pre-navigation) is load-bearing for ALL injected data: URIs. A site CSP without data: in img-src silently blocks every injected image on the live page (`complete=true, naturalWidth=0`, broken-image placeholder) — which only shows up in the live-rendering backends (pdf/png/jpeg/webp), because saved HTML carries no CSP header and renders the same data URIs fine. test-site-4 (default-src 'self') is the reference case; debug with `document.querySelector('img').naturalWidth`.
- Server-side fetchers must NOT set `Accept-Encoding` manually — it disables Go's transparent gzip decompression and gzip-served SVGs get embedded as compressed garbage.
- `inject_images.js` removes `<source>` siblings inside `<picture>` — otherwise the external source keeps winning over the injected data URL.
- **Never freeze an image's RENDERED box as `width`/`height` attributes.** Those attributes are *presentational hints*: they apply as CSS to whichever axis the page's own stylesheet leaves `auto`. On a responsive image (`width:100%`, height from `aspect-ratio`) the width stays fluid while the height stays nailed to the capture viewport, so the picture distorts at every other window width — and in both directions: test-site-3's promo tiles rendered 340x604 instead of 340x453 at 1200px (33% too tall) while its carousel slide rendered 1248x546 instead of ~970x546 at 1920px (29% too wide). It is NOT format-specific; HTML and MHT both carried it, and it is invisible at exactly the capture width, which is where it hid. Freeze pixels only for images laid out at their bitmap's own size; otherwise preserve the intrinsic RATIO, and only when the page dictates none. Both dimensions must always come from one source (mixing the page's value for one axis with the bitmap's for the other turned round avatars into ovals).
- Never remove elements matched by class alone without a structural guard — sites put state classes like `age-gate-open` on `<body>`.
- An age-gate click can trigger a reload: `actions.WaitForReady` absorbs it, and injected CSS must be re-applied afterwards.
- **`goodblaster/errors.Wrap(nil, msg)` returns a NON-NIL error.** Never tail-return `errors.Wrap` unguarded.

## Verification

Prep changes are exercised end-to-end through parchmint: build `parch` against this checkout (go.work links the repos) and run the reference-site verification described in parchmint's CLAUDE.md. For script-only work, `rx --prep staticize -v <url>` shows per-step logs and timings.

## Conventions

- Error wrapping via `errors.Wrap(err, "context")` (github.com/goodblaster/errors) — guarded, see invariant above.
- New prep behavior = a named pipeline step (JS file → `ScriptFile` constant in `scripts/embed.go` → action wrapper → step in `pipeline/staticize.go` + `Register`), not inline workflow code.
- Scripts return a small stats object; it lands in step logs.
- CLI: logs → stderr, content → stdout/file.
- **Flags use [spf13/pflag](https://github.com/spf13/pflag)**, matching parchmint: long name canonical, short as a shorthand (`StringVarP(&recipe, "recipe", "r", …)`), so `-r`/`--recipe` are one flag. Multi-character flags take two dashes. pflag parses flags anywhere on the line, which is why `rx URL @links --only links` works and why no flag-reordering helper is needed.

## Known cleanup remaining

- `actions/` could consolidate ~19 small files into ~6 thematic ones.
