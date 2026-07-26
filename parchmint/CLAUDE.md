# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**parchmint** is the page archiver; **parch** is its CLI. It captures a URL as a single self-contained file that renders faithfully with no network. Browser driving and page preparation come from [pscription](../pscription) (`runner`, `pipeline.Staticize()`, `actions`, `scripts`); this repo owns everything about the resulting **file**: the capture backends and output formats, the embedded text layer, search (`find`), OCR (`ocr`), and marking/rendering (`-M`, `mark`, `render`). The line between the repos: **pscription = live pages, parchmint = files.**

**Primary Language:** Go 1.24
**Runtime requirement:** a Chrome/Chromium installation (not bundled).

Local development uses the shared `go.work` one directory up, so parch always builds against the sibling pscription checkout; the go.mod pin only matters for builds outside the workspace.

## Build & Run

```bash
go build -o bin/ ./cmd/...
./bin/parch https://example.com                   # → example.com.html (SingleFile backend)
./bin/parch -f mht|pdf|png|jpeg|webp https://…
./bin/parch --width 1280 https://…                # layout width — picks the responsive breakpoint
./bin/parch --dpr 2 https://…                     # device pixel ratio: 2x output pixels, same layout
./bin/parch -f pdf --paper letter https://…       # sheet size, INDEPENDENT of --width (scaled to fit)
./bin/parch --links new-tab|disable https://…
./bin/parch --rx login.rx https://…               # custom pscription as the prep
./bin/parch -v https://…                          # per-step debug logs from both repos
./bin/parch text page.html                        # plain text back out of an archive
./bin/parch text --json page.html                 # the raw embedded text layer
./bin/parch text --blocks page.html               # NDJSON, one paragraph block per line —
                                                  # the export feed for external indexers
./bin/parch lines page.html                       # one plain-text line per paragraph block
./bin/parch find "some phrase" page.html          # block-scoped phrase search (grep-like exit codes)
./bin/parch find --json "wild*card" page.html     # hits with boxes, machine-readable
./bin/parch find -l -0 phrase *.html | xargs -0 … # multi-file grep over a corpus: -l/-c/-q/-m,
                                                  # -r DIR recursive, NDJSON --json, «match» colored
                                                  # on a TTY; also -e regex, -z fuzzy, -w/-s/-F,
                                                  # --in td,th, --source ocr, `-` stdin, `--`
./bin/parch -M "phrase" https://…                 # mark at capture time (repeatable; ALL backends,
                                                  # including pdf/png — marks are pixels there).
                                                  # --mark is the long form; --highlight is deprecated
./bin/parch ocr page.html                         # OCR the archive's images into the text layer —
                                                  # required only to mark text INSIDE images
./bin/parch ocr --engine tesseract page.html      # engine choice (default: apple on macOS)
./bin/parch mark "phrase" page.html               # mark-on-read → page.marked.html copy:
                                                  # <mark>s for DOM text, highlights BAKED into
                                                  # images for OCR hits (--grayscale mutes them)
./bin/parch mark -f pdf "phrase" page.html        # …or render into any format. PDF adds invisible
./bin/parch mark -f png --dpr 0.4 "phrase" p.html # OCR overlay text + the attached JSON layer;
                                                  # rasters bake every mark into pixels
./bin/parch render -f pdf --paper a4 page.html    # same rendering, no marking, no `ocr` needed
./bin/parch render -f png --width 1024 page.html  # mark/render take --width/--dpr to RE-RENDER an
                                                  # archive at a different size (0 = as captured)
./bin/parch mark phone page.html --width 800      # flags may follow positionals (pflag)
```

`mark` and `render` are one operation: mark REQUIRES phrases and exits 1 when nothing matches; render takes none and only converts. Both accept `-f html|pdf|png|jpeg|webp`; `mark` defaults to the archive's own container.

Config file precedence: CLI flags → `./.parch/config` (nearest ancestor) → `~/.parch/config` → defaults. Logs → stderr; content → stdout when piped.

## Architecture

- **`capture/`** — `CaptureWithOptions(ctx, url, cfg, recipe, backend, opts) → *Snapshot{URL, Title, MIME, Bytes, ViewportWidth, TextLayer}`; runs prep, text-layer extraction/highlighting, and serialization in one browser session, with layout-defect retry. Backends implement `Backend` (`Name`, `Ext`, `Action(snap)`): `SingleFile{}` (vendored single-file-core bundle — preferred), `MHT{}` (Chrome MHTML + quoted-printable data-URI repair), `PDF{}` (printToPDF, screen media, single tall page), `Screenshot{}`/`JPEG{}`/`WebP{}` (full-page rasters). `RenderArchive` (capture/render.go) is the one path behind both `parch mark` and `parch render`: it loads an existing archive, optionally marks it, and hands it to a backend — or serializes the marked DOM when no backend is given. Marking is an option on rendering, not a separate operation, which is why there is no longer a `pdf` command. Its OCR strategy forks on the target: `PDF{}` gets invisible overlay text (`overlay_ocr_text.js`) plus the layer attached as an embedded file; everything else gets marks baked into image pixels (`bake_image_marks.js`), which survive any reflow or viewer.
- **`cmd/parch/`** — CLI: flag/config layering, output naming (`{title}`/`{now}` templates), format selection. Subcommands (`text`/`find`/`ocr`/`mark`/`render`) dispatch off `os.Args[1]` before flag parsing. Flags use **[spf13/pflag](https://github.com/spf13/pflag)**, not Go's `flag`: GNU-style parsing, so `-s`/`--long` are one flag with one canonical name, flags may follow positionals (`parch mark phone report.html --width 800`), `--` ends options, and `--help` output renders `-f, --format string` with no hand-rolled formatter. Register aliases as shorthands (`StringVarP`), never as a second flag; retire spellings with `MarkDeprecated`, which keeps them parsing while hiding them from help and warning on use.
  **Flags are looked up by their canonical LONG name** — `explicitFlags`/`pickStr` key on `"output"`, not `"o"`. Using a shorthand as the key silently reads as "not set", which once sent a whole archive to stdout instead of the output file.
- **`textlayer/`** — the library half of `parch text`/`parch find` and the future catalog's foundation: layer parsing (`FromFile`), the versioned normalizer (`NormVersion` — NFKD accent folding, case, quote/dash unification, invisible-char bridging, hyphen splitting, CJK runes as single tokens), and block-scoped Ctrl-F-style phrase matching: a query word matches anywhere INSIDE a page word ("phone" finds iPhone — deliberately loose; keep it that way), consecutive query words match consecutive page words, and `*` bridges up to `maxGap` words ("apple*card" finds both "Apple Gift Card" and "AppleGiftCard" — internal stars carry both readings). Hits cover whole page words. Offsets are UTF-16 code units throughout (the extractor writes JavaScript string offsets). Known normalizer frontier for a future version: Traditional↔Simplified Chinese folding (書 vs 书 do not match today).
- **`internal/ocr`** — pluggable OCR: `Engine` interface, `apple/` (macOS Vision via cgo/ObjC — darwin-only build tags with a stub elsewhere) and `tesseract/` (CLI wrapper over TSV output; a line's identity is the block/par/line triple and the TSV columns are left/top/WIDTH/HEIGHT, not x2/y2). `ocr.Default()` picks apple on darwin, tesseract elsewhere. Engines return normalized 0..1 top-left boxes. `windows/` is a deliberate NON-implementation of `Windows.Media.Ocr` (which would make `ocr` install-free on Windows as Vision does on macOS): selectable as `--engine windows` so the wiring is exercised, erroring on every call, and never returned by `Default()` — flip that only once it recognizes text. Its package doc holds the API sketch and the open questions.
- **`internal/config`** — TOML config loading and filename templating.
- **pflag** (`github.com/spf13/pflag`, BSD-3) — GNU-style flag parsing for the CLI; see `cmd/parch/` above for why and for the canonical-name rule.
- **pdfcpu** (`github.com/pdfcpu/pdfcpu`, pure-Go Apache-2.0) — attaches/extracts the text-layer embedded file for the PDF container (`textlayer/pdf.go`). Read via `ExtractAttachmentsRaw` (NOT `Attachments`/`ListAttachments`, which return metadata with a nil `Reader`); relaxed validation, since `printToPDF` output occasionally trips strict mode.
- **`internal/log`** — logging seam: interface + logos-delegating default, mirroring pscription's. Binaries use goodblaster/logos as the implementation; configuring `logos.SetDefaultLogger` (as parch's main does) routes both repos' library logs.

Backends read per-capture settings from the `Snapshot` (`ViewportWidth`, `DevicePixelRatio`, `PaperWidthIn`/`PaperHeightIn`) — never from globals.

**Three independent size axes** — keep them separate, they were conflated once:
- **`--width`** = CSS layout width. Decides the responsive breakpoint, so changing it *reflows* the page. On `mark`/`render` it re-renders an existing archive at a new width (default: the width it was captured at, from the layer). A reflow can change the MATCH COUNT — a wider layout renders content a narrower one hides — because marking runs against a fresh extraction, not the embedded layer.
- **`--dpr`** = device pixel ratio (`runner.Config.DeviceScaleFactor`). Output pixels per CSS pixel; changes resolution, never layout. Recorded in the text layer's `viewport.dpr`. **This — not `--width` — is how you shrink an image while keeping the captured layout** (`--dpr 0.4` on a 2000px capture → an 800px-wide image of the same page).
- **`--paper`** = PDF sheet size in inches, independent of layout width: the page keeps its layout and `printToPDF` *scales* it onto the sheet (`scale = paperWidthIn × 96 ÷ layoutWidthPx`, clamped to Chrome's [0.1, 2.0]). Default `auto` derives the sheet from the layout, which is why an unconfigured 1600px capture yields a 16.67in-wide page. Named sizes (letter, a4, …) carry a height and paginate normally; a bare number sets width only and keeps the single tall page.

Raster encoder limits are in **device** pixels while layout metrics are in **CSS** pixels — `captureFullImage` takes `dpr` and converts, or a tall page at `--dpr 2` silently blows the WebP limit despite a safe-looking CSS height.

**Favicons** (`capture/favicon.go`, `--favicon`, on by default for html/mht): the page's icon links are fetched server-side, inlined as data URIs, and written into the SERIALIZED document by the HTML and MHT backends — the live DOM is deliberately untouched, because SingleFile drops icon links during serialization and the MHT path rewrites the document part regardless. Two things are load-bearing: icons are decoration, so each fetch gets a short budget (`faviconTimeout`) and they run concurrently — a host that accepts a connection and never answers once turned a 5.8s capture into 35.7s; and the serialized HTML is edited with a quote-aware scanner (`tagEnd`/`tagAttrValue`), never a regex, because `[^>]*` cannot skip a `>` inside an attribute value and a truncated match leaves fragments behind as visible text in the archive.

## Hard-won invariants (violating these caused real bugs)

- SingleFile capture must pass `blockScripts: true` (this vendored bundle's option name — it has no `removeScripts`). Keeping site JS means age gates re-run when the snapshot is opened.
- Chrome's WebP encoder hard-fails (empty bytes, no error) above 16383px per dimension; `captureFullImage` clamps height and warns. Zero-byte screenshots are treated as errors, never written.
- Chrome's print pipeline paints `position:fixed` elements on EVERY page and drops fixed (parallax) backgrounds beyond the first viewport — hence `flattenFixedPositioning` and `normalizeBackgroundAttachment` before PDF/raster capture.
- **`--profile` is single-writer.** Chrome takes a singleton lock on a user-data-dir, so concurrent parch runs sharing one `--profile` fail to start (`process_singleton_posix`) — measured: 3 parallel captures, 2 died, 1 survived. Parallel batches (`xargs -P`) must either drop `--profile` or give each worker its own copy of it. `--cache` has no such problem (3/3 concurrent runs fine), and without `--profile` every run gets a throwaway profile, so plain parallel capture is safe.
- Prep-side invariants (dialog layering, WebGL buffer preservation, OOPIF freezing, CSP bypass) live in pscription's CLAUDE.md — read it before touching the capture flow.

## Verification

`go test ./...` covers the pure-Go core: the `textlayer` normalizer/matcher (substring-with-gaps semantics, UTF-16 offset provenance, quotes/hyphens/accents/CJK, container sniffing and HTML/MHT embed round-trips) and the tesseract TSV parser. Browser-driven behavior (capture, mark, pdf) is verified end to end against the reference sites below.

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
- **Flags: long name canonical, short as a pflag shorthand** (`fs.StringVarP(&v, "output", "o", …)`). One meaning per letter across every subcommand — `-c` must not be *count* in one command and *color* in another. Multi-character flags take two dashes; single-character take one.

## License note

`capture/vendor/single-file-bundle.js` is [single-file-core](https://github.com/gildas-lormeau/single-file-core) (AGPL-3.0-or-later), used as the final HTML serializer — relevant if binaries are distributed or offered as a network service; the escape hatch is a native HTML serializer backend built on pscription's fetch/embed machinery. All other code is original.

## Text layer

HTML and MHT archives embed the **parchmint-text/1 layer** — the page's rendered text with word-level geometry, extracted from the live page just before serialization. Format contract: [TEXTLAYER.md](TEXTLAYER.md) (read it before touching anything layer-related; blocks-as-phrase-boundary and raw-text-no-normalization are load-bearing). The extraction script lives in pscription (`scripts/extract/text_layer.js`); this repo owns the header composition, embedding (`capture/textlayer.go` for HTML's script element, `textlayer/mht.go` for MHT's base64 MIME part), and read-back (`parch text <file>`, `--json` for the raw layer — the container is sniffed from the bytes, never the extension). On by default for the HTML and MHT backends (`--text=false` to disable); graphical backends have no layer. Extraction failure degrades to a warning — a capture without a layer beats no capture. MHT quirks: index-time image pairing scans the QP-DECODED text/html part (`textlayer.MHTDocument` — the data URIs inside are identical to the HTML case after decoding), and `parch mark` keeps the source format: on an .mht it writes a .marked.mht via `textlayer.ReplaceMHTDocument` (marked document QP-encoded back into the container, every other part kept so cid: references resolve; linked stylesheets are inlined from the CSSOM first).

Gotchas: SingleFile's `compressHTML` output has NO closing `</body>` tag (the splice appends in that case — anything else post-processing archive bytes must not assume the tag exists), and the embedded JSON escapes `</` as `<\/` so it can't terminate its own script element (`json.Unmarshal` undoes it for free).

**Capture-time marking** (`-M`/`--mark`, deprecated alias `--highlight`; `capture.Options.Highlight`) runs in `captureOnce` before ANY backend serializes, so marks appear in every format — as `<mark data-parchmint>` elements in HTML, as yellow pixels in pdf/png/jpeg/webp. Matching uses the same `textlayer` Go matcher as `parch find` (one matcher, by design). Applying the marks is the load-bearing subtlety: the extraction runs with `cacheRuns=true` (stashing each block's text-node segment map on `window.__pmRunCache`), and `apply_text_marks.js` wraps the matched ranges against THAT cached extraction — NOT a second walk. An earlier design re-walked to apply marks and gated per block on a text-length guard; on a page whose fonts/layout are still settling (a freshly loaded bare archive), the second walk yields different block boundaries and the guard silently dropped almost every mark (a 3.9 MB test-site-7 archive marked 5 of 383). Never reintroduce a second walk for marking. The EMBEDDED layer is the pre-mark payload — marks are inline and layout-neutral, and the layer stays authoritative. A phrase spanning inline elements is wrapped one text-node segment at a time, back-to-front, because `Range.surroundContents` cannot cross element boundaries and node splits must never invalidate pending offsets.

`parch mark` (highlight-on-read) loads the archive file:// at its recorded viewport and writes a marked COPY: DOM matches via a FRESH extraction walk + the marks pass (self-consistent ids — the embedded layer's ids are NOT trusted for DOM marking, only for OCR blocks, which a fresh walk cannot see), OCR matches baked into image pixels (canvas redraw, image-relative coords, per-line merged bands) so they survive any reflow/viewer/re-render. The original archive is never modified; the marked copy's baked images no longer hash-match its embedded layer (it's a viewing artifact, not a source archive).

## Planned

- Normalizer v2: Traditional↔Simplified Chinese folding (query-side, so existing archives benefit without recapture).
- Markdown export built on the text layer (blocks are already clean typed paragraphs; OCR blocks become image captions).
- Cryptographic timestamping of archives (hash page and layer separately — OCR output is not byte-deterministic).
- A separate catalog application over many archives: rebuildable index built from embedded layers via the `textlayer` package (cached normalized forms must record `NormVersion` and rebuild when it changes), cross-page and cross-version search, block-shingle dedup between date-stamped versions of a URL, and per-block embeddings for semantic search (in the index, never in archives).
