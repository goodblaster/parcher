# parchmint

**parch** — page archiver. Captures a URL as a single self-contained file
that renders faithfully with no network: ordinary popups dismissed, access gates preserved for an explicit decision,
lazy content loaded, videos/canvases/cross-origin iframes frozen to
stills, every resource embedded.

Cross your eyes and it's a parchment; but it's mint perfect.

```bash
parch https://www.example.com            # → example.com.html — one file, renders offline
parch ocr example.com.html               # OCR the images in the file (cover art, charts…)
parch find "music" example.com.html      # search the FILE — no browser, no network;
                                       #   finds "music" even inside music-service tiles
parch mark --grayscale "music" "album" example.com.html
                                       # → example.com.marked.html: every match highlighted —
                                       #   image matches painted into the image itself
```

Archive now, find later: each archive (HTML or MHT) carries a **text layer** —
every rendered word with its page coordinates — so files can be stored
anywhere, indexed by any search system, and when one is found, shown
with the matches highlighted... even when the text only ever existed as
pixels. ([Full loop example](examples/search-and-mark) ·
[format spec](TEXTLAYER.md))

## Defaults

| Behavior | Default | Change it with |
| --- | --- | --- |
| Capture format | Self-contained HTML | `-f mht\|pdf\|png\|jpeg\|webp` |
| Access gates | Detect and preserve; save gated output plus JSON and exit **3** | `--clear FILE`, `--auto-clear`, or `--gates=preserve` |
| Image pixels | Downsize supported rasters to displayed size × capture DPR | `--no-downsize-images` |
| Pixel density | **1** output pixel per CSS pixel | `--dpr` |
| Text layer and favicon | Embedded in HTML/MHT | `--no-text`, `--no-favicon` |
| OCR | Never run during capture; `parch ocr` preserves image sizes | `parch ocr --downsize-images FILE` to resize after recognition |

These are the built-in defaults; supported configuration settings and explicit
CLI flags can override them. Logs go to stderr. Use `-o` to name an output file;
otherwise the name is derived from the URL/configuration, or archive bytes go to
stdout when piped.

## Access gates

By default, `--gates=detect` preserves detected access gates without accepting
their declarations. An unresolved capture writes a `.gated` archive and matching
JSON metadata, then exits **3**. Successful non-gated and cleared captures use
the regular filename and exit **0**.

```bash
parch -o page.html https://example.com
# No gate: page.html; exit 0.
# Unresolved gate: page.gated.html + page.json; exit 3.

parch --clear page.json
# Approves the recorded decisions; successful clearance writes page.html.
```

| Flag | Behavior |
| --- | --- |
| `--gates=detect` | Default: preserve unresolved gates and exit 3. |
| `--gates=preserve` | Intentionally keep the gated view and JSON; exit 0 on success. |
| `--clear FILE` | Approve the decisions recorded in a metadata file and recapture using its URL, format, and regular output name. |
| `--auto-clear` | Approve supported gates as they appear in one session, including their declarations and terms. |
| `--approve-gate ID` | Approve an exact gate fingerprint directly; repeatable. |

`--clear` verifies that the recorded gate still matches before submitting it.
Changed declarations or another unresolved gate produce an updated gated archive
and JSON with exit 3. `--auto-clear` also exits 3 if clearance cannot complete;
it supports simple entry buttons and required checkboxes, not credentials,
date-of-birth entry, or solving CAPTCHAs.

There is no `.cleared` suffix. The original gated archive and JSON remain after
successful clearance, so scripts should use the latest exit status rather than
file existence alone. Other capture options use current settings; repeat options
such as `--width`, `--dpr`, or `--profile` when they matter to the original capture.
`--clear` cannot be combined with `--auto-clear`, `--approve-gate`, or
`--gates=preserve`.

The optional `--report FILE` writes a separate capture report, including for
non-gated pages; it does not replace the automatic gate metadata file. See
[Access gates and scriptable captures](ACCESS-GATES.md) for naming rules,
detection limits, JSON contracts, and a Bash example that handles exit 3.

## Image sizing

Images are downsized **by default** after the page is laid out at the chosen
viewport dimensions. The target is each image's displayed size multiplied by
the capture's `--dpr` (default **1**): an image displayed at 300 × 200 CSS pixels
needs 300 × 200 bitmap pixels at DPR 1, or 600 × 400 at DPR 2.

Aspect ratio and responsive layout are preserved. A source used several times
retains enough pixels for its largest measured use. Images are never enlarged,
and a replacement is kept only if its encoded bytes are smaller. JPEG, PNG and
static WebP are supported, including single-layer `cover`/`contain` backgrounds.
SVG, GIF/animations, images with EXIF or ICC colour profiles, unmeasurable images
and complex CSS backgrounds are preserved. PDF `--paper` scaling is not factored
into downsizing yet; targets use the capture layout and DPR.

`--no-downsize-images` preserves the original embedded image pixels, which may
help OCR read small text or allow later zooming/re-rendering at larger sizes.
To let OCR read those pixels before reducing file size:

```bash
parch --no-downsize-images -o page.html https://example.com
parch ocr --downsize-images page.html
```

Plain `parch ocr page.html` **preserves image sizes**. With `--downsize-images`,
recognition happens first, then images are reduced using the recorded capture
geometry and density. Both HTML and MHT work without launching a browser; image
hashes used by search and highlighting are updated with the new bytes. OCR
updates the archive in place by default; add `-o smaller.html` to keep the input
unchanged.

Older archives use their text-layer image boxes as the sizing fallback. Missing
original pixels cannot be recovered from an archive that was already downsized.

## All the knobs

```bash
parch https://example.com                  # → example.com.html
parch -o page.html https://example.com
parch -f mht https://example.com           # Chrome MHTML instead of HTML
parch -f pdf https://example.com           # PDF (selectable text, opens anywhere)
parch -f png https://example.com           # full-page screenshot (lossless)
parch -f jpeg https://example.com          # full-page screenshot (smaller)
parch --links new-tab https://example.com   # external links open in a new tab
parch --links disable https://example.com   # links kept but unclickable
parch --no-downsize-images https://example.com # retain original image pixels for OCR/zoom
parch --no-favicon https://example.com   # skip embedding the page favicon
                                           #   (embedded as a data URI: shown by
                                           #    Chromium/Firefox, ignored by Safari)
parch https://example.com > page.html      # content on stdout when piped
parch text page.html                       # rendered text back out of an archive
parch lines page.html                      # one plain-text line per paragraph — pipe food
parch text --blocks page.html               # NDJSON, one paragraph per line with ids and
                                           # provenance — feed for external indexers
parch ./saved-page.html                    # a page already on disk can be captured too
parch find "some phrase" page.html         # search an archive, no browser needed
parch find -l -0 "phrase" *.html |         # …or a whole corpus: grep flags (-l -c -q -m,
  xargs -0 -n1 parch mark "phrase"         # -r into directories), file: prefixes, NDJSON
                                           # --json, colored matches on a TTY (--color)
parch find --hidden "returns" page.html    # also search what the page HID: collapsed FAQ
                                           # answers, inactive tabs, "read more" bodies —
                                           # each hit tagged [hidden:collapsed], [hidden:display]…
parch mark --hidden "returns" page.html    # …and reveal + mark them in the copy, outlined
                                           # and labelled "hidden on the original page"
parch find -e 'trilob(ite|ita)s?' p.html   # regex mode; also -z 2 fuzzy (OCR typos),
                                           # -w whole words, -s case-sensitive, -F literal,
                                           # --in td,th and --source ocr scoping
parch -M "phrase" https://…                # capture with matches already marked
parch ocr page.html                        # OCR the archive's images (charts, frozen
                                           # canvases/iframes) into the text layer —
                                           # needed only to mark text inside images
parch ocr --downsize-images page.html      # recognize first, then shrink embedded images
parch mark "phrase" page.html              # → page.marked.html with matches highlighted,
                                           # including matches inside images (--grayscale)
parch mark --mark-color gold --mark-color cyan foo bar p.html  # a color per phrase; --mark-style underline|box|bold
parch mark -f pdf "phrase" page.html       # → page.marked.pdf: selectable text (incl.
                                           # inside OCR'd images), marks, attached layer
parch mark -f png --dpr 0.4 "x" page.html   # …or any raster; --dpr resizes without reflowing
parch render -f pdf --paper a4 page.html    # convert with no marking, no `ocr` needed
cat page.html | parch find "phrase" -      # '-' pipes archives on stdin (find/text/lines/
                                           # mark/render); '--' ends options ("-foo" is
                                           # searchable); parch help <cmd> for any of this
```

HTML captures warn when snapshot resources cannot be embedded. Use
`parch --strict URL` to fail instead of writing that incomplete HTML archive.
This checks unresolved snapshot references; it does not certify visual fidelity
or detect content the page never loaded. `--strict` applies to HTML capture only.

More runnable demos in [examples/](examples) — each is a `run.sh` that
builds parch and narrates what it does.

HTML archives embed a **text layer** ([TEXTLAYER.md](TEXTLAYER.md)): the
page's rendered text with word-level coordinates, extracted at capture
time. `parch text` reads it back (`--json` for the raw layer, with block
structure and word geometry; `--blocks` for an NDJSON paragraph feed any
external indexer can ingest). `parch find` searches it Ctrl-F style —
a query word matches anywhere inside a page word ("phone" finds iPhone),
phrases match within one paragraph (never across), insensitive to case,
accents, punctuation, and typographic quotes; `*` bridges words
("apple*card" finds "Apple Gift Card"). Opt-in modes sharpen or loosen
that: `-e` regex, `-z N` fuzzy (finds what OCR *almost* read), `-w`
whole words, `-s` case-sensitive, `-F` literal, `--in`/`--source`
structural scoping — one matcher shared by `find`, `mark`, and
capture-time `-M`, so a query that finds is a query that
highlights. Hits come with pixel coordinates (`--json`) and grep-like
exit codes for scripting. Disable at capture with `--no-text`.

**Hidden text.** A page carries words it does not show — the collapsed
accordion, the tab you did not click. The layer records them separately,
with the reason each was hidden, and leaves them out of every search unless
you pass `--hidden`. `parch mark` tells you when there were more matches in
hidden text; `parch mark --hidden` opens them up in the marked copy. The
archive itself is never altered: it stays the page as it was.

Flags mean the same thing on every command (`-o` output, `-f` format, `-v`
verbose, `-h` help; `--width`/`--height`/`--dpr`/`--paper`; `--timeout`;
`--mark-color`/`--mark-style`; `--hidden`), and exit status is 0 / 1 failed
or no match / 2 bad invocation. Capture additionally exits 3 when an archive
was saved with an unresolved access gate.

Built on [pscription](../pscription), which does the
browser driving and page preparation; the capture backends (HTML, MHT, PDF,
screenshots) live here.

## Requirements

```bash
go install github.com/goodblaster/parcher/parchmint/cmd/parch@latest
```

- **Go 1.26+** to build (the module requires it; older toolchains will
  refuse unless they can auto-download a newer one).
- **Chrome or Chromium**, installed separately — it is not bundled, and
  every capture drives a real browser. Nothing else is needed for
  capture, `text`, `lines`, `find`, `mark`, or `pdf`.
- **An OCR engine, only for `parch ocr`.** Nothing to install on
  macOS or Windows; a package on Linux:

  | platform | engine | install |
  |---|---|---|
  | macOS | Apple Vision (built in) | none |
  | Linux | Tesseract | `apt install tesseract-ocr` |
  | Windows | Windows OCR (built in) | none — uses the OCR languages Windows has installed |

  `parch ocr` picks the platform default; `--engine tesseract` forces it
  anywhere. The OCR language comes from the page itself (`<html lang>`);
  `--lang` overrides it. Non-English pages need that language's data
  installed (`apt install tesseract-ocr-deu`, `tesseract-ocr-chi-sim`, …)
  — `parch ocr` says which package when one is missing, and carries on in
  English. Tesseract reads noticeably less than Apple Vision does; expect
  roughly half the words on image-heavy pages. Engines are pluggable:
  `parch ocr` hands an engine every image of the archive in one batch and
  waits (`--timeout` bounds the wait), so an external OCR service can be
  added as one more `--engine` without touching the command.

  Two cloud engines are included but **untested against the real
  services** (they were written from the API documentation without an
  account): `--engine textract` (AWS Textract; set `AWS_ACCESS_KEY_ID`,
  `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`) and `--engine google` (Google
  Cloud Vision; set `GOOGLE_VISION_API_KEY`). They **upload the archive's
  images to that provider** and bill your account per image, so they are
  never chosen automatically. Everything else in the tool
  works with no OCR engine present — `ocr` is the only command that reads
  text out of images.

- **Linux** is a first-class target: `make build` there produces pure-Go
  binaries. If Chrome refuses to start with "No usable sandbox" (containers
  running as root, Ubuntu 23.10+ with restricted user namespaces), either
  fix the sandbox for your system or set `PSCRIPTION_NO_SANDBOX=1`,
  understanding that it removes Chrome's isolation from the pages you load.

- **Windows** uses `Windows.Media.Ocr` through the built-in Windows
  PowerShell 5.1 (`powershell.exe`), with one process per OCR batch.
  No separate OCR program is needed. Recognition requires an installed
  Windows OCR language pack; the engine tries the page's language, then
  your preferred languages, then any available pack. If none is installed,
  it reports what to install. PowerShell 7 (`pwsh`) is not a substitute.

Runs on macOS, Linux, and Windows; the Apple Vision engine is compiled in
on macOS only, and the rest of the tool is pure Go.

**License note:** all code here is original, and `parch` carries no copyleft
obligation however it is distributed or offered as a service. The HTML
serializer is Chrome's own DOM snapshot plus this repo's inliner
(`internal/convert/render`).

Earlier versions vendored
[single-file-core](https://github.com/gildas-lormeau/single-file-core)
(AGPL-3.0-or-later) as the HTML serializer. It has been removed outright —
not made optional — after the native backend was measured against it.

Planned: markdown export; a catalog application over many archives.
