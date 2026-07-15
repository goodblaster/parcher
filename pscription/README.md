# pscription

Browser script-running tech. **rx** executes a *pscription* — an ordered
list of preparation steps and scripts — against a URL in a headless
browser, like a doctor's ℞ (Latin *recipe*: "take thou").

The built-in `staticize` pscription turns a live page into something a
capture backend can serialize faithfully: dialogs and age gates dismissed,
lazy content loaded, videos frozen to stills, every resource embedded as a
data URI. [parchmint](../parchmint)'s `parch` is that pscription fused to a
serializer.

## Usage

```bash
go build -o bin/ ./cmd/...

./bin/rx https://example.com                  # staticize + capture → example.com.html
./bin/rx -backend mht|screenshot ...
./bin/rx -links keep|new-tab|disable ...
./bin/rx https://example.com > page.html      # content on stdout when piped
```

Requires a Chrome/Chromium installation (launched headless via the DevTools
protocol). Everything else is compiled into the binary.

**Roadmap:** `.rx` pscription files (built-in steps by name + your own
scripts, shebang-executable) with a JSON report on stdout —
`./prices.rx https://shop.com | jq .outputs.prices`.

## Architecture

Flow: **runner** (browser session) → **pipeline** (pscription steps) →
**capture** (optional serialization).

- `runner/` — browser lifecycle, config, pre-navigation hooks
- `pipeline/` — named steps composed into recipes; `Staticize()`; a step
  registry that will back the `.rx` file format
- `capture/` — backends: `SingleFile{}` (self-contained HTML, preferred),
  `MHT{}`, `Screenshot{}`
- `actions/` — composable chromedp building blocks
- `scripts/` — embedded page-context JavaScript ([SCRIPTS.md](SCRIPTS.md))

Dialog clearing is layered, strongest first: consent/age-gate state is
pre-seeded before navigation (cookies + localStorage), known consent
platforms are dismissed by exact selector, text heuristics handle the rest,
and a geometry-based demolition pass runs just before capture.

## License note

`capture/vendor/single-file-bundle.js` is
[single-file-core](https://github.com/gildas-lormeau/single-file-core)
(AGPL-3.0-or-later), used as the final HTML serializer. All page
preparation is original code.
