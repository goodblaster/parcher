# pscription

Browser script-running tech. **pscribe** executes a *pscription* — an ordered
list of preparation steps and scripts — against a URL in a headless
browser, like a doctor's ℞ (Latin *recipe*: "take thou").

The built-in `staticize` pscription turns a live page into something a
capture backend can serialize faithfully: dialogs and age gates dismissed,
lazy content loaded, videos/canvases/cross-origin iframes frozen to
stills, every resource embedded as a data URI.
[parchmint](../parchmint)'s `parch` is that pscription fused to a
serializer, plus a text layer extracted here (rendered text with
word-level geometry) that makes the archives searchable.

## Usage

`pscribe` runs a pscription against a URL and prints a JSON report of what the
scripts returned. Steps are positional and run in order.

```bash
go build -o bin/ ./cmd/...

# built-in article extractor; @article auto-captures under "article"
./bin/pscribe https://example.com/article @article --only article

# your own page-context scripts, in order, capturing named outputs
./bin/pscribe https://shop.com dismiss.js load-more.js prices.js:prices | jq .outputs.prices

# crawler without a crawler
./bin/pscribe https://blog.site @links --only links | jq -r '.[].url' | xargs -n1 parch

# a .rx pscription file (or make it executable with a #!/usr/bin/env pscribe line)
./bin/pscribe --recipe login-and-extract.rx https://portal.site
./login-and-extract.rx https://portal.site

# log in by hand in a visible browser, then extract
./bin/pscribe --show https://portal.site wait:user @article
```

**Steps:** `<name>` a built-in step (`dismiss-popups`, `wait-quiet`, …) ·
`@name[:out]` a built-in script (`@article`, `@links`) · `path.js[:out]`
your own script · `wait:user` / `wait:2s`. The `:out` suffix captures a
script's return value into the report's `outputs`.

**Options** are named as `parch` names them: `--recipe`, `--profile`,
`--show`, `--timeout`, `--width`, `--height`, `--dpr`, `--user-agent`, `-v`.
The browser announces its own (Chrome) user agent unless `--user-agent`
says otherwise.

**Prep** (`--prep`): `none`, `minimal` (default — dialogs cleared, content
loaded), or `staticize` (full self-contained preparation).

**Scripts** are parenthesized functions receiving the pscription's `vars` as
their argument — `((vars) => ({...}))` or the async forms. Output on stdout
is the JSON report (or one value with `--only`); logs go to stderr; exit
status is non-zero if a step failed.

Requires a Chrome/Chromium installation (launched via the DevTools
protocol). Everything else is compiled into the binary.

See [examples/](examples) for `.rx` files including the credential-separation
pattern (secrets in a gitignored JSON, applied by committed logic).

## Architecture

Flow: **runner** (browser session) → **pipeline** (pscription steps) →
scripts/extraction. Serialization to archive formats lives in
[parchmint](../parchmint), which consumes this library.

- `runner/` — browser lifecycle, config, pre-navigation hooks
- `pipeline/` — named steps composed into recipes; `Staticize()`; a step
  registry that will back the `.rx` file format
- `pscription/` — the `.rx` file format: parsing, prep selection, outputs
- `act/` — the action type and browser helpers everything else is built on
- `actions/` — composable building blocks
- `scripts/` — embedded page-context JavaScript ([SCRIPTS.md](SCRIPTS.md))

Dialog clearing is layered, strongest first: consent/age-gate state is
pre-seeded before navigation (cookies + localStorage), known consent
platforms are dismissed by exact selector, text heuristics handle the rest,
and a geometry-based demolition pass runs just before capture.
