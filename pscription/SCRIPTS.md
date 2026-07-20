# JavaScript Scripts Documentation

All JavaScript files in `/scripts/` are embedded via `//go:embed` and executed
in the page context through the `ScriptFile` engine (`scripts/script_files.go`).

Two shapes are supported:

- **Sync:** `(() => { ... })` — invoked as `(script)(params)`. The trailing
  `()` is added at runtime so JSON-encoded parameters can be injected.
- **Async:** `async function name(params) { ... }` — invoked with
  `awaitPromise`, so the Go side genuinely waits for completion. Leading
  comments are fine; detection skips them.

Set `gb_env=dev` to reload scripts from disk instead of the embedded copies.

## Page Preparation (`page-prep/`)

These run in the `pipeline.Staticize()` recipe, in roughly this order.

### Dialog clearing is layered, strongest technique first

0. **Prevent** (pre-navigation, Go side): `actions.PreseedConsentCookies`
   seeds well-known consent/age-gate cookies for the target domain, and
   `actions.PreseedConsentStorage` registers `preseed_consent.js` to run on
   every new document *before the page's own JS*, seeding localStorage
   flags. Banners that see "already answered" state never render.
1. **Known platforms by exact selector**: `dismiss_cmp.js` clicks the
   documented accept buttons of the major CMPs, and removes cross-origin
   CMP iframes (Sourcepoint) that JS can't click into.
2. **Heuristics**: `dismiss_popups.js` clicks visible affirmative/dismiss
   controls, scoped so page-content buttons are never pressed.
3. **Demolition** (last, at capture time): `remove_blocking_overlays.js`
   removes whatever still covers the page, by geometry.

| Script | Shape | Purpose |
|---|---|---|
| `preseed_consent.js` | pre-doc | Runs before page JS on every new document (survives reloads). Seeds localStorage "already consented / age verified" flags for common frameworks and hand-rolled checks. Never overwrites existing values. |
| `preserve_webgl_buffer.js` | pre-doc | Wraps `HTMLCanvasElement.getContext` to force `preserveDrawingBuffer: true` on every WebGL context, so the drawing buffer stays readable at capture time. Without it, reading a WebGL canvas after frame presentation yields transparent black — `rasterize_canvases.js` would see WebGL scenes as blank. |
| `dismiss_cmp.js` | sync | Exact-selector accept for ~20 consent platforms: OneTrust, Cookiebot, Didomi, Quantcast, TrustArc, Usercentrics (shadow DOM), Complianz, CookieYes, Osano, iubenda, Klaro, tarteaucitron, Borlabs, consentmanager, Google Funding Choices, Ezoic, Axeptio, google.com, Amazon. Removes Sourcepoint iframe apparatus + its scroll lock. |
| `remove_blocking_overlays.js` | sync | Geometry-based last resort: removes visible fixed/sticky elements that cover the viewport center and a large area share (or are near-full-viewport sheets). Guards: never html/body; skips anything holding >20% of the page's text (fixed heroes are content, not chrome); removes outermost candidates only; unlocks scrolling. |
| `hide_overlays.js` | sync | Injects persistent CSS hiding popup chrome that should never appear in a snapshot: marketing popups (OptinMonster etc.), modal backdrops, page preloaders. Deliberately does NOT hide age gates/cookie walls — those get clicked instead. Idempotent (`#gb-hide-overlays` guard). |
| `wait_for_quiet.js` | async | Condition-based settle: resolves when no DOM mutation for `quietMs` (default 500), hard cap `maxMs` (default 5000). Replaces blind sleeps. |
| `dismiss_popups.js` | sync | Clicks through age gates, cookie consent, newsletter modals. Only clicks VISIBLE controls; strong phrases ("enter site", "accept all cookies") anywhere, generic ones ("yes", "close", "×") only inside overlay-like containers. Closes `<dialog open>`. Removes gate/preloader remnants from the DOM. Unlocks scrolling. Returns `{clicked, removed, closedDialogs}`. |
| `force_image_loading.js` | sync | Defeats lazy loading by rewriting: native `loading="lazy"`, `data-src`/`data-srcset` (img, source, video), `data-bg`/`data-background`, lazyload classes. Traverses open shadow roots. Safe to run repeatedly. |
| `scroll_and_wait_images.js` | async | Scrolls the viewport through the page top→bottom in sequential steps (fires IntersectionObserver/scroll lazy loaders), re-checking page height as content grows, then returns to top. Params: `stepDelayMs` (250), `maxMs` (15000). |
| `wait_for_images.js` | async | Waits until `<img>` elements settle (polling `complete`) and probes CSS background URLs (including `::before`/`::after`) via `Image()`. Every probe has its own timeout — a stalled image cannot hang the wait. Param: `maxWaitMs`. |
| `normalize_viewport_units.js` | sync | Rewrites `100vh` (and `calc(...100vh...)`) heights in stylesheets and inline styles to prevent huge blank regions in full-page captures. Not in the default recipe; layout-altering, use per-site. |
| `rewrite_links.js` | sync | Optional link policy, applied only when requested (default: links untouched). `"new-tab"`: external links get `target=_blank rel=noopener`. `"disable"`: hrefs stay (they're archived information and removing them changes `:link` styling) but pointer-events CSS makes them unclickable; form actions parked in `data-gb-action`. Fragment/mailto/tel links stay live in every mode. `rx -links keep\|new-tab\|disable`. |

## Media Handling (`media/`)

| Script | Shape | Purpose |
|---|---|---|
| `capture_video_frame.js` | async | Replaces every `<video>` with a still `<img>`: real frame via canvas (loading/seeking with bounded waits) → poster fallback → hide the element (no black boxes). Replaces YouTube embed iframes with their thumbnail. **Must run before the embed steps** — the images it creates use external URLs. Returns `{frames, posters, hidden, youtube}`. |
| `warm_canvases.js` | async | Gives animated canvases their first frames: Chrome throttles rAF outside the viewport, so a canvas animation that initialized offscreen may never have painted. Sweeps the viewport down the page in bands, dwelling only where a reachable canvas has no ink yet, leaving as soon as it paints. Pierces open shadow roots and same-origin iframes. Runs before `rasterize_canvases.js`. Returns `{bands, dwelled, warmed, stillBlank}`. |
| `rasterize_canvases.js` | sync | Replaces every visible `<canvas>` with a still `<img>` (JPEG when fully opaque, PNG when transparent pixels are involved), so charts, games, and WebGL scenes survive archiving. Pierces open shadow roots and same-origin iframes. Fully transparent canvases are left alone (an empty box is the faithful capture). Canvases page JS can't read (tainted, OffscreenCanvas) are marked `data-gb-canvas-shot=<n>` for the Go side's CDP element-screenshot fallback. Returns `{replaced, transparent, skipped, unreadable, marked}`. |
| `replace_canvas_with_image.js` | sync | `(id, dataURL)`: swaps the canvas marked `data-gb-canvas-shot=<id>` for an `<img>`. Companion to the CDP screenshot fallback in `actions.RasterizeCanvases`. |
| `locate_opaque_iframes.js` | sync | Marks iframes whose documents page JS cannot reach (cross-origin / sandboxed) with `data-gb-frame-shot=<n>` for `actions.FreezeOpaqueIframes`. Pierces shadow roots and same-origin frames; skips tiny frames (trackers), frames taller than the viewport, and everything past `maxCount` (default 12). Returns `{marked, skipped, tall}`. |
| `focus_opaque_iframe.js` | sync | `(id)`: scrolls a marked iframe into view (bottom-aligned — sticky site headers live at the top), hides fixed/sticky elements overlapping its box (they'd be baked into the still; restored on the next call or by the Go side), and returns the frame's viewport-relative box for cropping. |
| `replace_iframe_with_image.js` | sync | `(id, dataURL)`: swaps a marked iframe for an `<img>` of its screenshotted pixels. Cross-origin frame content can't be serialized into a DOM archive at all, so a still of what the user saw is the most faithful representation available. |
| `convert_svg_to_png.js` | async | Rasterizes the element at a CSS selector (inline `<svg>` or `<img src=*.svg>`) to a PNG data URL. Awaits image decode (used by OCR, which can't read SVG). |
| `convert_svg_to_jpg.js` | async | Same as above but JPEG on a white background. |
| `locate_svg_elements.js` | sync | Returns CSS selectors for inline SVG elements (OCR pre-pass). |

## Resource Embedding (`resources/`)

The default recipe embeds via server-side fetching: `extract_*` collects URLs,
Go fetches them (bypassing CORS), `inject_*` swaps in the data URLs. The
`embed_*` scripts are in-browser alternates (page-credentialed fetch) for
resources the server can't reach.

| Script | Shape | Purpose |
|---|---|---|
| `extract_image_urls.js` | async | Collects `<img>` (currentSrc), `video[poster]`, `input[type=image]` URLs; shadow-DOM aware. |
| `inject_images.js` | async | Swaps collected URLs for data URLs. Removes `<source>` siblings inside `<picture>` (otherwise the external source keeps winning), clears `srcset`/`sizes`, freezes rendered dimensions. |
| `extract_background_urls.js` | async | Collects CSS `background-image` URLs, **including `::before`/`::after` pseudo-elements**; shadow-DOM aware. |
| `inject_background_images.js` | async | Rewrites background URLs to data URLs. Element backgrounds via inline `!important` style; pseudo-elements via generated rules keyed on a `data-gb-bg` marker attribute. |
| `embed_images.js` | async | In-browser alternate: fetch each `<img>` with page credentials and inline it. |
| `embed_background_images.js` | async | In-browser alternate for backgrounds. |
| `rasterize_font_icons.js` | sync | Replaces icon-font glyphs with rendered `<img>`s so icons survive without the font. Only touches empty `<i>`/`<span>` whose `::before` content is a Private Use Area glyph — regular italic text is never affected. DPI-aware. |
| `inject_css.js` | sync | `(id, css)`: upserts a `<style>` element by id. |
| `remove_unused_css.js` | async | Strips CSS rules whose selectors match nothing. Not in the default recipe (SingleFile's `removeUnusedStyles` covers this); available for MHT workflows. |
| `remove_unused_fonts.js` | async | Same idea for `@font-face` blocks. |
| `inline_iframes.js` | async | Replaces same-origin iframes with divs containing their HTML. Experimental: inlined styles can leak into the parent page. Not in the default recipe. |

## Extraction (`extract/`)

| Script | Shape | Purpose |
|---|---|---|
| `article.js` | sync | Heuristic main-article extraction (`@article`). |
| `links.js` | sync | Page links with text (`@links`). |
| `text.js` | sync | Simple visible-text dump (`@text`). |
| `text_layer.js` | sync | The parchmint text layer (format contract: parchmint's TEXTLAYER.md): rendered text with word-level geometry. Blocks from *computed* display via the anonymous-box rule; words merged across inline elements; whitespace per CSS; DOM order (= select-all/copy order); composed-tree walk (shadow roots, slots, same-origin iframes with coordinate offsets); `Intl.Segmenter` for CJK; clip-chain + offscreen filtering excludes sr-only/skip-link text a human can't see. Self-checks: per-leaf-block `innerText` comparison (flagged as `oracle:"mismatch"`) and a whole-page `Selection.toString()` Dice similarity in stats. |

## Utilities (`util/`)

| Script | Shape | Purpose |
|---|---|---|
| `detect_language.js` | sync | Returns declared page languages (html `lang`, Content-Language meta; comma lists split; empty values ignored); falls back to `en-US`. |
| `extract_links.js` | sync | All `<a href>` values. |
| `extract_text.js` | sync | Visible text extraction. |
| `convert_to_grayscale.js` | sync | CSS grayscale filter (testing aid). |

## Adding a new script

1. Put the file in the right subdirectory; pick sync or async shape (async if
   the work involves any waiting — the Go side then awaits it).
2. Add a `ScriptFile` constant in `scripts/embed.go`.
3. Wrap it in an action (`actions/`) and, if it belongs in default page prep,
   add a named step in `pipeline/staticize.go` (and register it).
4. Return a small stats object — it lands in the step logs.
