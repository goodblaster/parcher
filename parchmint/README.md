# parchmint

**parch** — page archiver. Captures a URL as a single self-contained file
that renders faithfully with no network: dialogs and age gates dismissed,
lazy content loaded, videos/canvases/cross-origin iframes frozen to
stills, every resource embedded.

Cross your eyes and it's a parchment; but it's mint perfect.

```bash
parch https://www.test-site-1            # → test-site-1.html — one file, renders offline
parch index test-site-1.html             # OCR the images in the file (cover art, charts…)
parch find "music" test-site-1.html      # search the FILE — no browser, no network;
                                       #   finds "music" even inside music-service tiles
parch mark -grayscale "music" "iphone" test-site-1.html
                                       # → test-site-1.marked.html: every match highlighted —
                                       #   image matches painted into the image itself
```

Archive now, find later: each HTML archive carries a **text layer** —
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
parch -links new-tab https://example.com   # external links open in a new tab
parch -links disable https://example.com   # links kept but unclickable
parch https://example.com > page.html      # content on stdout when piped
parch text page.html                       # rendered text back out of an archive
parch find "some phrase" page.html         # search an archive, no browser needed
parch -highlight "phrase" https://…        # capture with matches pre-highlighted
parch index page.html                      # OCR the archive's images (charts, frozen
                                           # canvases/iframes) into the text layer
parch mark "phrase" page.html              # → page.marked.html with matches highlighted,
                                           # including matches inside images (-grayscale)
```

More runnable demos in [examples/](examples) — each is a `run.sh` that
builds parch and narrates what it does.

HTML archives embed a **text layer** ([TEXTLAYER.md](TEXTLAYER.md)): the
page's rendered text with word-level coordinates, extracted at capture
time. `parch text` reads it back (`-json` for the raw layer, with block
structure and word geometry). `parch find` searches it Ctrl-F style —
a query word matches anywhere inside a page word ("phone" finds iPhone),
phrases match within one paragraph (never across), insensitive to case,
accents, punctuation, and typographic quotes; `*` bridges words
("apple*card" finds "Apple Gift Card"). Hits come with pixel coordinates
(`-json`) and grep-like exit codes for scripting. Disable at capture
with `-text=false`.

Built on [pscription](https://github.com/goodblaster/pscription), which does the
browser driving and page preparation; the capture backends (SingleFile
HTML, MHT, PDF, screenshots) live here. Requires a Chrome/Chromium
installation.

**License note:** `capture/vendor/single-file-bundle.js` is
[single-file-core](https://github.com/gildas-lormeau/single-file-core)
(AGPL-3.0-or-later), used as the final HTML serializer. All other code is
original.

Planned: markdown export; a catalog application over many archives.
