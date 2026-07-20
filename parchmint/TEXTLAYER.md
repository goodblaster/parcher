# The parchmint text layer

An HTML archive produced by `parch` carries an embedded **text layer**: the
page's rendered text with word-level geometry, extracted at capture time —
the one moment the rendering is deterministic (fixed viewport, settled
layout, fonts loaded). It makes the archive searchable and highlightable
without re-running a browser, and is the substrate for `parch find`,
capture-time highlighting, OCR merging (`parch index`), and any future
catalog/indexing application.

Applies to the HTML backend only; graphical formats (pdf/png/jpeg/webp)
have no DOM to annotate (PDF has its own native text).

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
text`/`find`/`index`/`mark` sniff the container and behave identically,
and a marked copy keeps its source's format — `mark` on an .mht writes a
.marked.mht by putting the marked document back into the container
(stylesheets inlined from the CSSOM; all other parts kept, so cid:
references keep resolving).

## Format: `parchmint-text/1`

```json
{
  "format": "parchmint-text/1",
  "url": "https://…",
  "capturedAt": "2026-07-19T13:24:51Z",
  "viewport": { "width": 1600, "dpr": 1 },
  "generator": "parch",
  "stats": { "blocks": 120, "words": 4321, "oracleMismatches": 0, "pageSimilarity": 0.998 },
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
  display (the anonymous-box rule), never tag names. Text with no client
  rects is not rendered and is not in the layer.
- `type` is the container's tag when meaningful
  (`p`, `h1`–`h6`, `li`, `td`, `th`, `caption`, `pre`, `blockquote`),
  else `other` (includes anonymous inline runs).
- `oracle` appears only on blocks where extracted text disagreed with the
  browser's own `innerText` for a clean leaf element — a self-check flag,
  counted in `stats.oracleMismatches`. `stats.pageSimilarity` is a Dice
  token similarity between the layer's top-document text and the
  browser's programmatic select-all (`Selection.toString()`).

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
This lets `parch index` OCR the archive's images and place words in page
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
the img). Re-running `parch index` replaces all `source:"ocr"` blocks.
OCR never runs at capture time — it is resource-hungry and explicitly
opt-in.

### Reserved for later versions

Per-word `source` tagging (finer than the block-level `source`),
block-level language tags, and sentence boundaries. A consumer of
`parchmint-text/1` must ignore unknown trailing array elements and
unknown object fields.
