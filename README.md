# parcher

Copy a website and keep it — a perfect copy, in one self-contained file,
that needs nothing from the network to render.

Two tools, one repository. They ship together because they are built
together: prep is only meaningful when something serializes the result.

| | | |
|---|---|---|
| [**parchmint**](parchmint) | `parch` | page archiver — captures a URL as a single file that renders faithfully offline, with a searchable text layer |
| [**pscription**](pscription) | `pscribe` | browser script runner — executes a *pscription* (an ordered list of prep steps and scripts) against a live page |

```bash
go install github.com/goodblaster/parcher/parchmint/cmd/parch@latest
go install github.com/goodblaster/parcher/pscription/cmd/pscribe@latest
```

Or from a checkout — `make` stamps the version from git, which `go install`
cannot do:

```bash
make          # both commands into ./bin
make install  # both into GOBIN, version-stamped
make test     # every test
make help     # the rest
```

**parch** turns a live page into a file: ordinary popups dismissed, access gates preserved for an explicit decision,
lazy content loaded, videos/canvases/cross-origin iframes frozen to stills,
every resource embedded. **pscribe** is the preparation half on its own,
driving a page and returning what your scripts extracted.

Start with [parchmint/README.md](parchmint/README.md) or
[pscription/README.md](pscription/README.md).

## Capture defaults

`parch URL` creates a self-contained HTML archive by default. It prepares the
page, embeds its text layer, and downsizes supported raster images to their
displayed size in the chosen capture layout, multiplied by `--dpr` (default 1).
OCR runs only when you invoke `parch ocr`.

Access gates are detected and preserved by default; eligibility declarations
are not automatically accepted. With `-o page.html`, an unresolved gate produces
`page.gated.html` and matching metadata in `page.json`, and exits **3**.
A non-gated or successfully cleared capture uses `page.html` and exits **0**.

```bash
parch -o page.html https://example.com       # normal capture; exit 3 if gated
parch --clear page.json                     # approve the recorded gate, then recapture
parch --auto-clear https://example.com       # approve supported gates as they appear
parch --gates=preserve https://example.com   # keep the gated view; exit 0 on success

parch --no-downsize-images -o page.html https://example.com # retain original pixels
parch ocr --downsize-images page.html       # recognize first, then downsize in place
```

`--clear` and `--auto-clear` explicitly authorize the gate's declarations and
terms. Clearing leaves the original gated archive and JSON in place; scripts
should check the latest exit status, not just whether a JSON file exists.
Plain `parch ocr page.html` preserves image sizes. See the
[gate workflow](parchmint/README.md#access-gates) and
[image-sizing details and exceptions](parchmint/README.md#image-sizing), including
the current PDF paper-scaling limitation.

## A note on `test-site-N`

Comments and docs cite measurements against real sites — page-load timings,
byte counts, defect reproductions — but refer to them as `test-site-1`,
`test-site-2`, and so on. The evidence is what matters; the names are not
mine to publish. The mapping is kept locally and is not committed.

## License

[PolyForm Small Business 1.0.0](LICENSE.md) — source-available, not open
source.

**Free** for individuals, and for any company with fewer than 100 people and
under $1M USD annual revenue. Use it, modify it, ship it, build a product on
it, no payment and no permission needed.

**Above that threshold**, use for your company's benefit needs a commercial
license: dave@goodblaster.com. Terms are negotiable and I'd rather say yes
than have you rewrite it.

All code here is original and every dependency is permissively licensed
(MIT/BSD/ISC/Apache-2.0), so nothing in the tree carries a copyleft
obligation onto what you build with it.
