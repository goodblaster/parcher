# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**pscription** is a browser script-running and page-preparation library; **rx** is its CLI. A "pscription" (like a doctor's ℞ — Latin *recipe*, "take thou") is an ordered list of preparation steps and scripts executed against a live page. It drives headless Chrome (chromedp) and runs a pipeline of named preparation steps (dismiss dialogs, defeat lazy loading, freeze videos/canvases/cross-origin iframes to stills, embed resources as data URIs).

Serialization lives downstream: [parchmint](../parchmint) (binary `parch`) owns the capture backends (SingleFile HTML, MHT, PDF, screenshots) and consumes this library for browser driving and prep. The line between the repos: **pscription = live pages, parchmint = files.**

**Primary Language:** Go 1.24
**Runtime requirement:** a Chrome/Chromium installation (not bundled).

## Build & Run

```bash
go build -o bin/ ./cmd/...
./bin/rx https://news.site @article                  # built-in article extractor
./bin/rx https://shop.com dismiss.js prices.js:out   # your own scripts, capture "out"
./bin/rx -f login-and-scrape.rx https://portal.site  # .rx pscription file
./bin/rx -prep staticize -v https://example.com      # full prep, debug logging
```

stdout carries the JSON report (or one output raw with `-only`); logs go to stderr. No test suite yet; verify library changes by building parchmint's `parch` against this checkout (go.work links them) and running its verification flow.

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

Capture-backend invariants (SingleFile options, encoder limits, PDF quirks) live in parchmint's CLAUDE.md; these are the prep-side ones.

- Media freezing (`capture-video-frame`, `rasterize-font-icons`) must run BEFORE the embed steps — the `<img>`s they create (posters, YouTube thumbnails) carry external URLs that still need inlining. (`rasterize-canvases` is exempt: it produces data URIs directly.)
- WebGL canvases are only readable because the `preserve_webgl_buffer.js` pre-navigation script forces `preserveDrawingBuffer` before any page JS runs; a WebGL context created without it reads back transparent black forever. Never demote it to an ordinary step.
- Chrome throttles rAF for offscreen content, and offscreen cross-origin iframes (OOPIFs) are not rasterized AT ALL by full-page screenshots — `captureBeyondViewport` included. Hence `warm-canvases` (scroll blank canvases into view until they paint; a same-process canvas keeps its last frame) and `freeze-iframes` (screenshot each cross-origin iframe WHILE it is composited in-viewport, then swap in the still). Dwelling alone is useless for OOPIFs — their pixels don't persist offscreen; the shot must happen while visible.
- MDN-style pages put their sample iframes inside open shadow roots — any script that enumerates canvases/iframes must pierce `shadowRoot`s, or it silently finds nothing.
- `actions.BypassCSP` (pre-navigation) is load-bearing for ALL injected data: URIs. A site CSP without data: in img-src silently blocks every injected image on the live page (`complete=true, naturalWidth=0`, broken-image placeholder) — which only shows up in the live-rendering backends (pdf/png/jpeg/webp), because saved HTML carries no CSP header and renders the same data URIs fine. test-site-4 (default-src 'self') is the reference case; debug with `document.querySelector('img').naturalWidth`.
- Server-side fetchers must NOT set `Accept-Encoding` manually — it disables Go's transparent gzip decompression and gzip-served SVGs get embedded as compressed garbage.
- `inject_images.js` removes `<source>` siblings inside `<picture>` — otherwise the external source keeps winning over the injected data URL.
- Never remove elements matched by class alone without a structural guard — sites put state classes like `age-gate-open` on `<body>`.
- An age-gate click can trigger a reload: `actions.WaitForReady` absorbs it, and injected CSS must be re-applied afterwards.
- **`goodblaster/errors.Wrap(nil, msg)` returns a NON-NIL error.** Never tail-return `errors.Wrap` unguarded.

## Verification

Prep changes are exercised end-to-end through parchmint: build `parch` against this checkout (go.work links the repos) and run the reference-site verification described in parchmint's CLAUDE.md. For script-only work, `rx -prep staticize -v <url>` shows per-step logs and timings.

## Conventions

- Error wrapping via `errors.Wrap(err, "context")` (github.com/goodblaster/errors) — guarded, see invariant above.
- New prep behavior = a named pipeline step (JS file → `ScriptFile` constant in `scripts/embed.go` → action wrapper → step in `pipeline/staticize.go` + `Register`), not inline workflow code.
- Scripts return a small stats object; it lands in step logs.
- CLI: logs → stderr, content → stdout/file.

## Known cleanup remaining

- `actions/` could consolidate ~19 small files into ~6 thematic ones.
