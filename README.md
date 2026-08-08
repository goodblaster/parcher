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

**parch** turns a live page into a file: dialogs and age gates dismissed,
lazy content loaded, videos/canvases/cross-origin iframes frozen to stills,
every resource embedded. **pscribe** is the preparation half on its own,
driving a page and returning what your scripts extracted.

Start with [parchmint/README.md](parchmint/README.md) or
[pscription/README.md](pscription/README.md).

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
