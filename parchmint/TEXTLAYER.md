# The parchmint text layer

An HTML archive produced by `parch` carries an embedded **text layer**: the
page's rendered text with word-level geometry, extracted at capture time —
the one moment the rendering is deterministic (fixed viewport, settled
layout, fonts loaded). It makes the archive searchable and highlightable
without re-running a browser, and is the substrate for `parch find`,
capture-time highlighting, OCR merging (`parch ocr`), and any future
catalog/indexing application.

HTML and MHT archives carry it at capture time. A **PDF** carries it too,
but PDF is a *rendered view* produced downstream from an archive (`parch
render -f pdf <archive>`), not a capture format — see "PDF" below. The pixel raster
formats (png/jpeg/webp) have no text layer.

## Placement

**HTML archives**: a single `<script
type="application/x-parchmint-text">…</script>` element just before
`</body>`. Inert to rendering, invisible to CSS, extractable with a
string scan. Inside the JSON every `</` is escaped as `<\/` (a no-op
JSON string escape) so the payload cannot terminate its own script tag;
consumers unescape after slicing.

**MHT archives**: an extra MIME part before the closing boundary —
`Content-Type: application/x-parchmint-text`, base64-encoded (sidestepping
quoted-printable entirely), `Content-Location: parchmint:text-layer`.
multipart/related readers render only referenced parts, so browsers carry
the layer along silently. Same capabilities either way: `parch
text`/`find`/`ocr`/`mark` sniff the container and behave identically,
and a marked copy keeps its source's format — `mark` on an .mht writes a
.marked.mht by putting the marked document back into the container
(stylesheets inlined from the CSSOM; all other parts kept, so cid:
references keep resolving).

**PDF** (`parch render -f pdf <archive>`, or `parch mark -f pdf` — a rendered export, not a capture): the
layer is attached as an embedded file (`/EmbeddedFiles`, name
`parchmint-text-layer.json`); viewers ignore it, `parch text`/`find` pull
it back out. A PDF actually carries *four* searchable/visible layers:
1. **native selectable text** — Chrome's `printToPDF` emits the DOM text
   as real PDF text (searchable in any viewer with no help from us);
2. **invisible OCR overlay text** — for each `source:"ocr"` block,
   transparent-color `<span>`s are positioned over the image at the word
   boxes before printing, so image text becomes selectable/searchable in
   any viewer (our own `find` uses the attachment and is exact; native
   viewers may merge some adjacent image words);
3. **visible highlights** — phrase matches render as `<mark>` on page text
   and translucent-yellow overlay backgrounds on image words;
4. **the attached JSON layer** — full `parchmint-text/1`, so `parch
   text`/`find` on a `.pdf` behave exactly as on `.html`/`.mht`.
Because a `printToPDF` bakes images as XObjects (not data URIs), `parch
ocr`/`mark` do NOT operate on a `.pdf` — OCR and highlight the source
archive, then (re-)export.

## Format: `parchmint-text/1`

```json
{
  "format": "parchmint-text/1",
  "url": "https://…",
  "capturedAt": "2026-07-19T13:24:51Z",
  "viewport": { "width": 1600, "dpr": 1 },
  "generator": "parch",
  "stats": { "blocks": 120, "words": 4321, "hiddenBlocks": 9, "hiddenWords": 240,
             "oracleMismatches": 0, "pageSimilarity": 0.998 },
  "blocks": [
    {
      "id": 42,
      "type": "p",
      "node": "1/3/0/7",
      "frame": "1/2/4",
      "box": [64, 980, 720, 88],
      "text": "Hello, world — one paragraph as a human reads it.",
      "words": [[0, 6, 64, 980, 52, 22], [7, 12, 120, 980, 46, 22]],
      "oracle": "mismatch"
    }
  ],
  "hidden": [
    {
      "id": 120, "type": "p", "node": "1/3/2/0", "why": "collapsed",
      "box": [0, 0, 0, 0],
      "text": "Returns accepted within thirty days.",
      "words": [[0, 7, 0, 0, 0, 0], [8, 16, 0, 0, 0, 0]]
    }
  ]
}
```

### Principles (load-bearing — future phases assume these)

- **A block is the phrase boundary.** Phrase matching never bridges
  blocks. `text` is the block's complete string, exactly as a select-all
  copy would render it (DOM order, visibility-filtered, whitespace
  collapsed per CSS, `<br>` as `\n` — a line break is not a new block).
- **Words annotate the string.** Each entry is
  `[charStart, charEnd, x, y, w, h]` — offsets into `text` (UTF-16 code
  units, JavaScript string semantics), box in page-space CSS pixels at the
  recorded viewport, integers. Search operates on `text`; a matched
  character range maps to the covering words for geometry.
- **Raw text, no normalization.** Accents, soft hyphens, punctuation,
  curly quotes are stored as found. Normalization (case/accent folding,
  punctuation handling) is the *query side's* job and is versioned there,
  so archives get smarter retroactively.
- **Two addressing schemes.** Pixel boxes are valid only at
  `viewport`/`dpr` — use them for overlays on rasters and thumbnails. For
  reflow-safe in-page highlighting, re-run the (deterministic) extraction
  walker on the archived DOM and map block ids/offsets to live ranges;
  `node` (child-element-index path from the document element, `frame`
  prefix for same-origin iframes) is an advisory shortcut, not the
  contract.
- **Blocks are extracted from the composed, rendered tree**: open shadow
  roots pierced, slots resolved, same-origin iframes walked (coordinates
  offset to the top page), cross-origin iframes excluded (the capture
  pipeline froze them to images). Block boundaries come from *computed*
  display (the anonymous-box rule), never tag names.
- **`blocks` is what a human sees — nothing else.** Text the document
  carries but does not show is not dropped; it is filed under `hidden`
  (next section). A reader that only knows `blocks` therefore indexes
  exactly the visible page.
- `type` is the container's tag when meaningful
  (`p`, `h1`–`h6`, `li`, `td`, `th`, `caption`, `pre`, `blockquote`),
  else `other` (includes anonymous inline runs).
- `oracle` appears only on blocks where extracted text disagreed with the
  browser's own `innerText` for a clean leaf element — a self-check flag,
  counted in `stats.oracleMismatches`. `stats.pageSimilarity` is a Dice
  token similarity between the layer's top-document text and the
  browser's programmatic select-all (`Selection.toString()`).
  A nonzero `oracleMismatches` does not imply lost text: `innerText`
  includes screen-reader-only content (1px-clipped elements) that our
  visibility filtering correctly drops. On reference-heavy pages this is
  the bulk of it — e.g. every mismatch on the test-site-7 *Trilobite* page
  (42 of them) was a citation's `cite-accessibility-label` ("Jump up
  to:"), with the citation text itself complete. Treat the count as a
  pointer to *investigate*, not evidence of a defect.

## Hidden text

A page carries more text than it shows: the collapsed FAQ answer, the tab
that is not selected, the "read more" body, a screen-reader label. Someone
keeping a page may well want to find those words later, so the layer
**classifies** hidden text rather than discarding it.

`hidden` is a second top-level array with the same block shape as `blocks`
plus `why`:

| `why` | The text was hidden by | Typical content |
|---|---|---|
| `display` | `display:none`, the `hidden` attribute | inactive tab panels, closed dialogs, menus |
| `visibility` | `visibility:hidden` | |
| `opacity` | `opacity:0` on the element or an ancestor (not mid-animation) | faded-out panels |
| `collapsed` | a closed `<details>`, `content-visibility:hidden`, `hidden="until-found"` | accordions the browser's own find-in-page would open |
| `clipped` | `overflow`/`clip`/`clip-path` leaving nothing visible | sr-only labels, `max-height:0` accordions |
| `offscreen` | parked above/left of the page origin | skip links, `text-indent:-9999px` |
| `zero-size` | `font-size:0` and friends | |
| `prep` | parch's own `hide-overlays` step | popup and backdrop chrome — **not page content** |

Rules:

- **It is a separate array on purpose.** A consumer that follows the
  "ignore unknown fields" rule — an older `parch`, an external indexer —
  never sees it, and keeps indexing visible text only. Tagging blocks in
  place would have made every such reader index hidden text as visible.
- **One id sequence.** Visible blocks are `0..n-1` (an id is its index);
  hidden blocks continue from `n`. OCR blocks are numbered after both.
- **Same text rules** as visible blocks: raw text, UTF-16 offsets, the
  block is the phrase boundary. A hidden subtree is always its own
  block(s) — it never splits or joins the visible run around it, so the
  visible blocks of a page are identical with and without hidden text
  recorded.
- **Geometry is best-effort.** Where the engine still lays the text out
  (`visibility`, `opacity`, `clipped`, `offscreen`) words carry real boxes;
  where it does not (`display`, a closed `<details>`) every box is zero.
  Never position an overlay from a hidden block's boxes.
- **Search is opt-in.** `find`, `text`, `lines` and `mark` see visible text
  only unless given `--hidden` (optionally `--hidden=collapsed,display`).
  `collapsed` content is hidden like the rest even though a browser's
  find-in-page would open it: the layer describes what was on screen.
  `--hidden` never includes `prep`; name it explicitly to audit it.
- **Revealing happens only in a marked copy.** `parch mark --hidden`
  un-hides each matched block in the COPY (opens the `<details>`, shows the
  panel), outlines it, and labels it "hidden on the original page". The
  archive is never altered, and capture-time `-M` never reveals — it only
  reports how many matches sit in hidden text. Revealing reflows the copy,
  an inactive tab appears alongside the active one, and a responsive page
  that carries both a desktop and a mobile menu yields both hits; narrow
  with `--hidden=<reasons>` when that is noise.
- `stats.hiddenBlocks`/`hiddenWords` count the array. `pageSimilarity` and
  the oracle compare visible text only.

Archives captured before this existed have no `hidden` array, and their
`blocks` include some text that was never visible (`visibility:hidden`,
`opacity:0`, closed `<details>`, clip-path) — the old extractor only tested
for client rects. `parch mark` is unaffected either way: it re-walks the
loaded archive, so `mark --hidden` works on old archives too. `find
--hidden` needs the array, so it needs a recapture.

### What an archive could not hold, and now does

Chrome's snapshot serializer drops every element carrying a `hidden`
attribute — by attribute, even when CSS makes it visible. Capture now
trades the attribute for `data-parchmint-hidden` just before the snapshot
(pscription's `preserve-hidden-attr`); the HTML backend restores the real
attribute in the output, an `.mht` keeps the `data-` form (it renders the
same). Archives captured earlier simply do not contain that content.

## Text that is not in text nodes

A form control shows text that lives in its *value*, and SVG `<text>` is
laid out by a different engine. Both are on screen, so both are visible
blocks, tagged `via`:

```json
{ "id": 7, "type": "other", "via": "control", "node": "1/12",
  "box": [8, 400, 240, 22], "text": "search the archive",
  "words": [[0, 6, 8, 400, 240, 22], [7, 10, 8, 400, 240, 22]] }
```

`via` is `control` (an `<input>` value or placeholder, a `<textarea>`'s
text, a `<select>`'s chosen option — never a password) or `svg`. Every word
carries the **element's** box: there is no per-word geometry. A mark on
such a block outlines the control, or puts a rect behind the SVG text,
instead of wrapping words. Generated content (`::before`/`::after`) is
not recorded; it is not selectable in a browser either.

## Images and OCR

The layer records every visible image at capture time:

```json
"images": [
  { "node": "1/17/1", "box": [450, 838, 520, 130], "natural": [520, 130], "hash": "1f6d1725" }
]
```

`hash` is FNV-1a (32-bit, over the src string's UTF-16 code units) of the
image's src — by extraction time the prep pipeline has inlined images as
data URIs, so the hash identifies the same bytes the archive carries.
This lets `parch ocr` OCR the archive's images and place words in page
coordinates **without a browser**: it re-hashes the archive's img srcs to
pair bytes with entries, OCRs each distinct image once, and appends
blocks like:

```json
{
  "id": 35, "type": "img", "source": "ocr", "image": "1f6d1725",
  "node": "1/17/1", "box": [450, 838, 520, 130], "confidence": 0.96,
  "text": "The quick brown fox jumps over the lazy dog",
  "words": [[0, 3, 470, 858, 42, 20], …]
}
```

OCR block rules: the text is a SINGLE paragraph — OCR lines joined with
spaces, because web images carry no canonical structure and a wrapped
sentence must stay matchable across its line break. Word boxes are page
coordinates derived from the image's recorded box; `image` links the
block to its source image by hash so a viewer can re-anchor highlights
to the element (image-relative positions = (word box − image box) /
image box, which survives reflow when positioned inside a wrapper around
the img). Re-running `parch ocr` replaces all `source:"ocr"` blocks.
OCR never runs at capture time — it is resource-hungry and explicitly
opt-in.

### Reserved for later versions

Per-word `source` tagging (finer than the block-level `source`),
block-level language tags, and sentence boundaries. A consumer of
`parchmint-text/1` must ignore unknown trailing array elements and
unknown object fields.
