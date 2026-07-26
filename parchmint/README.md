# parchmint

**parch** — page archiver. Captures a URL as a single self-contained file
that renders faithfully with no network: dialogs and age gates dismissed,
lazy content loaded, videos/canvases/cross-origin iframes frozen to
stills, every resource embedded.

Cross your eyes and it's a parchment; but it's mint perfect.

```bash
parch https://www.test-site-1            # → test-site-1.html — one file, renders offline
parch ocr test-site-1.html               # OCR the images in the file (cover art, charts…)
parch find "music" test-site-1.html      # search the FILE — no browser, no network;
                                       #   finds "music" even inside music-service tiles
parch mark --grayscale "music" "iphone" test-site-1.html
                                       # → test-site-1.marked.html: every match highlighted —
                                       #   image matches painted into the image itself
```

Archive now, find later: each archive (HTML or MHT) carries a **text layer** —
every rendered word with its page coordinates — so files can be stored
anywhere, indexed by any search system, and when one is found, shown
with the matches highlighted... even when the text only ever existed as
pixels. ([Full loop example](examples/search-and-mark) ·
[format spec](TEXTLAYER.md))

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
parch --favicon=false https://example.com   # skip embedding the page favicon
                                           #   (embedded as a data URI: shown by
                                           #    Chromium/Firefox, ignored by Safari)
parch https://example.com > page.html      # content on stdout when piped
parch text page.html                       # rendered text back out of an archive
parch lines page.html                      # one plain-text line per paragraph — pipe food
parch text --blocks page.html               # NDJSON, one paragraph per line with ids and
                                           # provenance — feed for external indexers
parch find "some phrase" page.html         # search an archive, no browser needed
parch find -l -0 "phrase" *.html |         # …or a whole corpus: grep flags (-l -c -q -m,
  xargs -0 -n1 parch mark "phrase"         # -r DIR recursive), file: prefixes, NDJSON
                                           # -json, colored matches on a TTY (--color)
parch find -e 'trilob(ite|ita)s?' p.html   # regex mode; also -z 2 fuzzy (OCR typos),
                                           # -w whole words, -s case-sensitive, -F literal,
                                           # --in td,th and --source ocr scoping
parch -M "phrase" https://…                # capture with matches already marked
parch ocr page.html                        # OCR the archive's images (charts, frozen
                                           # canvases/iframes) into the text layer —
                                           # needed only to mark text inside images
parch mark "phrase" page.html              # → page.marked.html with matches highlighted,
                                           # including matches inside images (-grayscale)
parch mark -c gold -c cyan foo bar p.html  # a color per phrase; --style underline|box|bold
parch mark -f pdf "phrase" page.html       # → page.marked.pdf: selectable text (incl.
                                           # inside OCR'd images), marks, attached layer
parch mark -f png --dpr 0.4 "x" page.html   # …or any raster; --dpr resizes without reflowing
parch render -f pdf --paper a4 page.html    # convert with no marking, no `ocr` needed
cat page.html | parch find "phrase" -      # '-' pipes archives on stdin (find/text/lines/
                                           # mark/render); '--' ends options ("-foo" is
                                           # searchable); parch help <cmd> for any of this
```

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
structural scoping — one matcher shared by `find`, `mark`, `pdf`, and
capture-time `-M`, so a query that finds is a query that
highlights. Hits come with pixel coordinates (`--json`) and grep-like
exit codes for scripting. Disable at capture with `--text=false`.

Built on [pscription](https://github.com/goodblaster/pscription), which does the
browser driving and page preparation; the capture backends (SingleFile
HTML, MHT, PDF, screenshots) live here.

## Requirements

```bash
go install github.com/goodblaster/parchmint/cmd/parch@latest
```

- **Go 1.26+** to build (the module requires it; older toolchains will
  refuse unless they can auto-download a newer one).
- **Chrome or Chromium**, installed separately — it is not bundled, and
  every capture drives a real browser. Nothing else is needed for
  capture, `text`, `lines`, `find`, `mark`, or `pdf`.
- **An OCR engine, only for `parch ocr`.** Nothing to install on
  macOS; a package elsewhere:

  | platform | engine | install |
  |---|---|---|
  | macOS | Apple Vision (built in) | none |
  | Linux | Tesseract | `apt install tesseract-ocr` |
  | Windows | Tesseract | `choco install tesseract` |

  `parch ocr` picks the platform default; `--engine tesseract` forces it
  anywhere. Non-English pages need that language's data too
  (`tesseract-ocr-deu` for `--lang deu`, etc.). Everything else in the
  tool works with no OCR engine present — `index` is the only command
  that reads text out of images.

  **Windows could need no install either.** Windows ships
  `Windows.Media.Ocr`, which reports the word-level boxes the text layer
  wants and reads screen-rendered text well. `internal/ocr/windows` is
  scaffolded for it — selectable as `--engine windows`, erroring until
  someone implements it — and its package doc carries the API sketch, the
  build-tag layout to copy from `internal/ocr/apple`, the box
  normalization contract, and the open question to answer first (whether
  a stock install recognizes anything without extra language packs).
  Contributions welcome; it needs a Windows machine to develop against.

Runs on macOS, Linux, and Windows; the Apple Vision engine is compiled in
on macOS only, and the rest of the tool is pure Go.

**License note:** `capture/vendor/single-file-bundle.js` is
[single-file-core](https://github.com/gildas-lormeau/single-file-core)
(AGPL-3.0-or-later), used as the final HTML serializer. All other code is
original.

Planned: markdown export; native Windows OCR via `Windows.Media.Ocr`
(scaffolded in `internal/ocr/windows`, so `index` would need no install
there); a catalog application over many archives.
