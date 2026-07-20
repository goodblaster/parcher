# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**parchmint** is the page archiver; **parch** is its CLI. It captures a URL as a single self-contained file that renders faithfully with no network. Browser driving and page preparation come from [pscription](../pscription) (`runner`, `pipeline.Staticize()`, `actions`, `scripts`); this repo owns everything about the resulting **file**: the capture backends and output formats, the embedded text layer, search (`find`), OCR indexing (`index`), and highlighting (`-highlight`, `mark`). The line between the repos: **pscription = live pages, parchmint = files.**

**Primary Language:** Go 1.24
**Runtime requirement:** a Chrome/Chromium installation (not bundled).

Local development uses the shared `go.work` one directory up, so parch always builds against the sibling pscription checkout; the go.mod pin only matters for builds outside the workspace.

## Build & Run

```bash
go build -o bin/ ./cmd/...
./bin/parch https://example.com                  # → example.com.html (SingleFile backend)
./bin/parch -f mht|pdf|png|jpeg|webp https://…
./bin/parch -width 1280 https://…                # layout width (also sets PDF paper width)
./bin/parch -links new-tab|disable https://…
./bin/parch -rx login.rx https://…               # custom pscription as the prep
./bin/parch -v https://…                         # per-step debug logs from both repos
./bin/parch text page.html                       # plain text back out of an archive
./bin/parch text -json page.html                 # the raw embedded text layer
./bin/parch find "some phrase" page.html         # block-scoped phrase search (grep-like exit codes)
./bin/parch find -json "wild*card" page.html     # hits with boxes, machine-readable
./bin/parch -highlight "phrase" https://…        # pre-highlight at capture (repeatable; ALL backends,
                                                 # including pdf/png — marks are pixels there)
./bin/parch index page.html                      # OCR the archive's images into the text layer
./bin/parch index -engine tesseract page.html    # engine choice (default: apple on macOS)
./bin/parch mark "phrase" page.html              # highlight-on-read → page.marked.html copy:
                                                 # <mark>s for DOM text, highlights BAKED into
                                                 # images for OCR hits (-grayscale mutes them)
```

Config file precedence: CLI flags → `./.parch/config` (nearest ancestor) → `~/.parch/config` → defaults. Logs → stderr; content → stdout when piped.

## Architecture

- **`capture/`** — `CaptureWithOptions(ctx, url, cfg, recipe, backend, opts) → *Snapshot{URL, Title, MIME, Bytes, ViewportWidth, TextLayer}`; runs prep, text-layer extraction/highlighting, and serialization in one browser session, with layout-defect retry. Backends implement `Backend` (`Name`, `Ext`, `Action(snap)`): `SingleFile{}` (vendored single-file-core bundle — preferred), `MHT{}` (Chrome MHTML + quoted-printable data-URI repair), `PDF{}` (printToPDF, screen media, single tall page), `Screenshot{}`/`JPEG{}`/`WebP{}` (full-page rasters). `MarkArchive` implements highlight-on-read over an existing archive.
- **`cmd/parch/`** — CLI: flag/config layering, output naming (`{title}`/`{now}` templates), format selection.
- **`textlayer/`** — the library half of `parch text`/`parch find` and the future catalog's foundation: layer parsing (`FromFile`), the versioned normalizer (`NormVersion` — NFKD accent folding, case, quote/dash unification, invisible-char bridging, hyphen splitting, CJK runes as single tokens), and block-scoped Ctrl-F-style phrase matching: a query word matches anywhere INSIDE a page word ("phone" finds iPhone — deliberately loose; keep it that way), consecutive query words match consecutive page words, and `*` bridges up to `maxGap` words ("apple*card" finds both "Apple Gift Card" and "AppleGiftCard" — internal stars carry both readings). Hits cover whole page words. Offsets are UTF-16 code units throughout (the extractor writes JavaScript string offsets). Known normalizer frontier for a future version: Traditional↔Simplified Chinese folding (書 vs 书 do not match today).
- **`internal/ocr`** — pluggable OCR: `Engine` interface, `apple/` (macOS Vision via cgo/ObjC — darwin-only build tags with a stub elsewhere) and `tesseract/` (CLI wrapper over TSV output; a line's identity is the block/par/line triple and the TSV columns are left/top/WIDTH/HEIGHT, not x2/y2). `ocr.Default()` picks apple on darwin, tesseract elsewhere. Engines return normalized 0..1 top-left boxes.
- **`internal/config`** — TOML config loading and filename templating.
- **`internal/log`** — logging seam: interface + logos-delegating default, mirroring pscription's. Binaries use goodblaster/logos as the implementation; configuring `logos.SetDefaultLogger` (as parch's main does) routes both repos' library logs.

Backends read per-capture settings from the `Snapshot` (e.g. `ViewportWidth` for PDF paper sizing) — never from globals.

## Hard-won invariants (violating these caused real bugs)

- SingleFile capture must pass `blockScripts: true` (this vendored bundle's option name — it has no `removeScripts`). Keeping site JS means age gates re-run when the snapshot is opened.
- Chrome's WebP encoder hard-fails (empty bytes, no error) above 16383px per dimension; `captureFullImage` clamps height and warns. Zero-byte screenshots are treated as errors, never written.
- Chrome's print pipeline paints `position:fixed` elements on EVERY page and drops fixed (parallax) backgrounds beyond the first viewport — hence `flattenFixedPositioning` and `normalizeBackgroundAttachment` before PDF/raster capture.
- Prep-side invariants (dialog layering, WebGL buffer preservation, OOPIF freezing, CSP bypass) live in pscription's CLAUDE.md — read it before touching the capture flow.

## Verification

Reference sites: `https://www.test-site-1` (videos, product tiles), `https://test-site-2` (age gate, hero video, carousel), `https://test-site-8/wiki/test-site-7:%E9%A6%96%E9%A1%B5` (CJK, logo SVGs). For canvas/WebGL/iframe work: `https://test-site-4` (WebGL cube, strict CSP), `https://www.test-site-5` (2D chart canvases), and the MDN canvas-animations tutorial (cross-origin sample iframes in shadow DOM). Capture, then screenshot **offline** so missing embeds fail visibly:

```bash
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new \
  --proxy-server="http://127.0.0.1:9" --hide-scrollbars --window-size=1600,3000 \
  --virtual-time-budget=8000 --screenshot=out.png "file://$PWD/capture.html"
```

Good captures have ~zero `src="http` references (grep) and no `<script>` tags except `application/ld+json`. Keep `--virtual-time-budget`: sites with entry CSS animations (test-site-5 fades in from `opacity:0`) screenshot blank without it — the archive is fine, the screenshot just fired too early.

## Conventions

- New capture format: implement `capture.Backend` in `capture/`, register it in `backendFor` in cmd/parch.
- Error wrapping via `errors.Wrap(err, "context")` (github.com/goodblaster/errors). **`Wrap(nil, msg)` returns a NON-NIL error** — never tail-return it unguarded.
- CLI: logs → stderr, content → stdout/file.

## License note

`capture/vendor/single-file-bundle.js` is [single-file-core](https://github.com/gildas-lormeau/single-file-core) (AGPL-3.0-or-later), used as the final HTML serializer — relevant if binaries are distributed or offered as a network service; the escape hatch is a native HTML serializer backend built on pscription's fetch/embed machinery. All other code is original.

## Text layer

HTML archives embed the **parchmint-text/1 layer** — the page's rendered text with word-level geometry, extracted from the live page just before serialization. Format contract: [TEXTLAYER.md](TEXTLAYER.md) (read it before touching anything layer-related; blocks-as-phrase-boundary and raw-text-no-normalization are load-bearing). The extraction script lives in pscription (`scripts/extract/text_layer.js`); this repo owns the header composition, embedding (`capture/textlayer.go`), and read-back (`parch text <file>`, `-json` for the raw layer). On by default for the HTML backend (`-text=false` to disable); graphical backends have no layer. Extraction failure degrades to a warning — a capture without a layer beats no capture.

Gotchas: SingleFile's `compressHTML` output has NO closing `</body>` tag (the splice appends in that case — anything else post-processing archive bytes must not assume the tag exists), and the embedded JSON escapes `</` as `<\/` so it can't terminate its own script element (`json.Unmarshal` undoes it for free).

**Capture-time highlighting** (`-highlight`, `capture.Options.Highlight`) runs in `captureOnce` before ANY backend serializes, so marks appear in every format — as `<mark data-parchmint>` elements in HTML, as yellow pixels in pdf/png/jpeg/webp. Matching uses the same `textlayer` Go matcher as `parch find` (one matcher, by design); the wrap pass is a second run of the same deterministic extraction walk with a `marks` parameter, so block ids agree across runs by construction (a per-block text-length guard skips blocks the page mutated in between). The EMBEDDED layer is the pre-mark payload — marks are inline and layout-neutral, and the layer stays authoritative. A phrase spanning inline elements is wrapped one text-node segment at a time, back-to-front, because `Range.surroundContents` cannot cross element boundaries and node splits must never invalidate pending offsets.

`parch mark` (highlight-on-read) loads the archive file:// at its recorded viewport and writes a marked COPY: DOM matches via a FRESH extraction walk + the marks pass (self-consistent ids — the embedded layer's ids are NOT trusted for DOM marking, only for OCR blocks, which a fresh walk cannot see), OCR matches baked into image pixels (canvas redraw, image-relative coords, per-line merged bands) so they survive any reflow/viewer/re-render. The original archive is never modified; the marked copy's baked images no longer hash-match its embedded layer (it's a viewing artifact, not a source archive).

## Planned

- The text layer as an extra MIME part in MHT. MHT is a first-class format: Chrome-only is acceptable for this project, and MHT's multipart container dedupes repeated images that SingleFile HTML re-embeds per use.
- Normalizer v2: Traditional↔Simplified Chinese folding (query-side, so existing archives benefit without recapture).
- Markdown export built on the text layer (blocks are already clean typed paragraphs; OCR blocks become image captions).
- Cryptographic timestamping of archives (hash page and layer separately — OCR output is not byte-deterministic).
- A separate catalog application over many archives: rebuildable index built from embedded layers via the `textlayer` package (cached normalized forms must record `NormVersion` and rebuild when it changes), cross-page and cross-version search, block-shingle dedup between date-stamped versions of a URL, and per-block embeddings for semantic search (in the index, never in archives).
