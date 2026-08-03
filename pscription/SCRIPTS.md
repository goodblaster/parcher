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
| `wait_for_stable_layout.js` | async | Polls page height until steady for `quietMs` (default 700, cap `maxMs` 8000) — lets carousels/tab panels finish initializing before media freezing and capture (uninitialized sliders render stacked, tripling page height). |
| `wait_for_sliders.js` | async | Waits for slider-library init markers (slick/owl/swiper), and — because those markers only exist AFTER init — also waits while any carousel-classed container is still stacked (>3x viewport tall). Used by the capture retry path, not the default recipe. Param: `maxMs`. |
| `detect_slider_defect.js` | sync | Reports whether a slider rendered in the broken stacked state — drives the capture-level retry with in-session recovery. Detects by SHAPE first (a carousel-classed container >3x viewport tall with 3+ children), which catches libraries it cannot name; the per-library marker checks are the fallback. Signature `stacked:<n>:<px>:<pageH>`. |
| `normalize_viewport_units.js` | sync | Rewrites `100vh` (and `calc(...100vh...)`) heights in stylesheets and inline styles to prevent huge blank regions in full-page captures. Not in the default recipe; layout-altering, use per-site. |
| `hide_overlays.js` (note) | — | Its CSS lives in a JS template literal, so **prose must never go inside it** — a backtick ends the literal and the script dies with a SyntaxError. Because prep steps are `stepIgnoreError`, that failure is logged once and swallowed, and the page silently gets no overlay hiding at all. `go test ./scripts/` parses every script with node to catch this. |
| `list_crossorigin_sheets.js` | sync | Lists stylesheets whose `cssRules` throws (cross-origin, no CORS headers). They render but are invisible to anything reading the CSSOM — which silently cost the archive its web fonts. Feeds `InlineCrossOriginStylesheets`. |
| `inline_sheet_text.js` | sync | Inlines a fetched stylesheet's text as a `<style>` immediately after its `<link>`, so the rules become readable. Position matters: document order is the cascade. The `<link>` is left in place. |
| `materialize_form_state.js` | sync | Writes runtime-only form state back as attributes (`value`, `checked`, `selected`) so a serializer that walks markup preserves it; passwords are never baked in. Runs at the very end of the recipe. |
| `flatten_adopted_stylesheets.js` | sync | Emits `document.adoptedStyleSheets` as a real `<style>` — constructed sheets are CSSOM-only and every serializer drops them. Shadow roots are handled by `link_shadow_styles.js` instead, which is why this one deliberately stops at the document. |
| `link_shadow_styles.js` | sync | Re-expresses each shadow root's CSS as a `<link>` to a `data:` URL. Chrome serializes shadow roots but drops `<style>` children while keeping a `<link>`, so this is the form that survives. Keeps the original `<style>` so the live page never goes unstyled mid-capture. Closed roots are unreachable and keep their content but lose their styling. |
| `rewrite_links.js` | sync | Optional link policy, applied only when requested (default: links untouched). `"new-tab"`: external links get `target=_blank rel=noopener`. `"disable"`: hrefs stay (they're archived information and removing them changes `:link` styling) but pointer-events CSS makes them unclickable; form actions parked in `data-gb-action`. Fragment/mailto/tel links stay live in every mode. `rx -links keep\|new-tab\|disable`. |

## Media Handling (`media/`)

| Script | Shape | Purpose |
|---|---|---|
| `capture_video_frame.js` | async | Replaces every `<video>` with a still `<img>`: real frame via canvas (loading/seeking with bounded waits) → poster fallback → hide the element (no black boxes). Replaces YouTube embed iframes with their thumbnail. **Must run before the embed steps** — the images it creates use external URLs. Returns `{frames, posters, hidden, youtube}`. |
| `warm_canvases.js` | async | Gives animated canvases their first frames: Chrome throttles rAF outside the viewport, so a canvas animation that initialized offscreen may never have painted. Sweeps the viewport down the page in bands, dwelling only where a reachable canvas has no ink yet, leaving as soon as it paints. Pierces open shadow roots and same-origin iframes. Runs before `rasterize_canvases.js`. Returns `{bands, dwelled, warmed, stillBlank}`. |
| `rasterize_canvases.js` | sync | Replaces every visible `<canvas>` with a still `<img>` (JPEG when fully opaque, PNG when transparent pixels are involved), so charts, games, and WebGL scenes survive archiving. Pierces open shadow roots and same-origin iframes. Fully transparent canvases are left alone (an empty box is the faithful capture). Canvases page JS can't read (tainted, OffscreenCanvas) are marked `data-gb-canvas-shot=<n>` for the Go side's CDP element-screenshot fallback. Returns `{replaced, transparent, skipped, unreadable, marked}`. |
| `replace_canvas_with_image.js` | sync | `(id, dataURL)`: swaps the canvas marked `data-gb-canvas-shot=<id>` for an `<img>`. Companion to the CDP screenshot fallback in `actions.RasterizeCanvases`. |
| `bake_image_marks.js` | sync | `(specs, opts)`: paints highlight bands INTO images (canvas redraw + src swap) for text OCR found inside them — specs map an image-src hash (same FNV-1a as `text_layer.js`) to image-relative fraction rects; opts: grayscale/color/stroke. Baked pixels survive any reflow, viewer, or re-render, with no wrappers or scripts. Pierces shadow roots and same-origin iframes. |
| `overlay_ocr_text.js` | sync | `(specs, opts)`: overlays invisible (`color:transparent`) selectable text over images at their OCR word boxes, so image text is searchable/selectable once the page is printed to PDF; matched words also get a translucent-yellow background (visible highlight + selectable in one). specs map an image-src hash to words `{t,x,y,w,h,hl}` (box as fractions of the image). Spans are absolute-positioned in page pixels in one container, disturbing no layout. Used by parchmint's `parch pdf`. |
| `remove_junk_iframes.js` | sync | `(minSize, hosts)`: deletes iframes that carry nothing archivable — invisible, smaller than `minSize` in either dimension (default 32), or served by an ad/analytics/tag host (suffix-matched, removed at any size). Visible sized non-tracker frames are kept as content. A **cost** step: every surviving frame costs a DOM backend a full recursive serialization pass. Runs before `locate_opaque_iframes.js` so the freeze budget goes to real embeds. Returns `{removed, hidden, tiny, tracker, kept}`. |
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
| `inject_images.js` | async | Swaps collected URLs for data URLs. Removes `<source>` siblings inside `<picture>` (otherwise the external source keeps winning), clears `srcset`/`sizes`. Size freezing is deliberately narrow: pixel `width`/`height` attributes only for images laid out at their bitmap's own size (or not laid out at all), an inline `aspect-ratio` when CSS drives the box but dictates no ratio, and nothing at all when the page already sets one — because those attributes are presentational hints that pin whichever axis the page leaves `auto`. |
| `extract_background_urls.js` | async | Collects CSS `background-image` URLs, **including `::before`/`::after` pseudo-elements**; shadow-DOM aware. |
| `inject_background_images.js` | async | Rewrites background URLs to data URLs. Element backgrounds via inline `!important` style; pseudo-elements via generated rules keyed on a `data-gb-bg` marker attribute. |
| `extract_font_faces.js` | async | Collects loaded `@font-face` rules (awaits `document.fonts.ready`, filters to faces actually used — pages can declare hundreds of subsetted variants) so fonts can be fetched server-side and inlined. Used by the HTML and MHT backends, both of which build on Chrome's snapshot and get no fonts from it. Pair it with `inline-crossorigin-css`: a sheet this cannot READ contributes no faces, silently. |
| `embed_images.js` | async | In-browser alternate: fetch each `<img>` with page credentials and inline it. |
| `embed_background_images.js` | async | In-browser alternate for backgrounds. |
| `rasterize_font_icons.js` | sync | Replaces icon-font glyphs with rendered `<img>`s so icons survive without the font. Only touches empty `<i>`/`<span>` whose `::before` content is a Private Use Area glyph — regular italic text is never affected. DPI-aware. |
| `inject_css.js` | sync | `(id, css)`: upserts a `<style>` element by id. |
| `remove_unused_css.js` | async | Strips CSS rules whose selectors match nothing. Not in the default recipe; available for HTML/MHT workflows, where nothing prunes today — measured, that is worth ~7% of archive size against a serializer that does prune. |
| `remove_unused_fonts.js` | async | Same idea for `@font-face` blocks. |
| `inline_iframes.js` | async | Replaces same-origin iframes with divs containing their HTML. Experimental: inlined styles can leak into the parent page. Not in the default recipe. |

## Extraction (`extract/`)

| Script | Shape | Purpose |
|---|---|---|
| `article.js` | sync | Heuristic main-article extraction (`@article`). |
| `links.js` | sync | Page links with text (`@links`). |
| `text.js` | sync | Simple visible-text dump (`@text`). |
| `text_layer.js` | sync | The parchmint text layer (format contract: parchmint's TEXTLAYER.md): rendered text with word-level geometry. Optional `cacheRuns` param stashes each block's text-node segment map on `window.__pmRunCache` so `apply_text_marks.js` can wrap ranges against this exact extraction. Blocks from *computed* display via the anonymous-box rule; words merged across inline elements; whitespace per CSS; DOM order (= select-all/copy order); composed-tree walk (shadow roots, slots, same-origin iframes with coordinate offsets); `Intl.Segmenter` for CJK; clip-chain + offscreen filtering excludes sr-only/skip-link text a human can't see. Self-checks: per-leaf-block `innerText` comparison (flagged as `oracle:"mismatch"`) and a whole-page `Selection.toString()` Dice similarity in stats. |
| `apply_text_marks.js` | sync | `(marks)`: wraps `{blockId: {ranges}}` in `<mark data-parchmint>`, using the segment maps `text_layer.js` cached (call it first with `cacheRuns=true`). Applying against the SAME extraction the Go matcher ran on — not a fresh re-walk — means every hit lands; there is no "the two walks disagreed" class of miss. One continuous highlight per phrase (segments merged per text node, whitespace bridged at node boundaries), wrapped back-to-front so node splits don't invalidate offsets. |

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
