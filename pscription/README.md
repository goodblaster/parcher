# webrun

Browser script-running and page-capture tech. webrun drives headless Chrome
through a pipeline of named preparation steps — dismiss dialogs and age
gates, defeat lazy loading, freeze videos to stills, embed every resource as
a data URI — then captures the page as a single self-contained file that
renders faithfully with no network at all.

## Usage

```bash
go build -o bin/ ./cmd/...

./bin/webrun https://example.com                  # → example.com.html
./bin/webrun -backend mht https://example.com     # Chrome MHTML instead
./bin/webrun -backend screenshot https://example.com
./bin/webrun -links new-tab https://example.com   # external links open in new tab
./bin/webrun -links disable https://example.com   # links kept but unclickable
./bin/webrun https://example.com > page.html      # content on stdout when piped
```

Requires a Chrome/Chromium installation (webrun launches it headless via the
DevTools protocol). Everything else is compiled into the binary.

## Architecture

Capture flow: **runner** (browser session) → **pipeline** (preparation
recipe) → **capture** (serialize snapshot).

- `runner/` — browser lifecycle, config, pre-navigation hooks
- `pipeline/` — named steps composed into recipes; `Staticize()` is the
  default page-preparation recipe; a step registry enables data-defined
  recipes later
- `capture/` — backends: `SingleFile{}` (self-contained HTML, preferred),
  `MHT{}`, `Screenshot{}`
- `actions/` — composable chromedp building blocks
- `scripts/` — embedded page-context JavaScript ([SCRIPTS.md](SCRIPTS.md))
- `agents/` — user-agent strings

Dialog clearing is layered, strongest first: consent/age-gate state is
pre-seeded before navigation (cookies + localStorage), known consent
platforms are dismissed by exact selector, text heuristics handle the rest,
and a geometry-based demolition pass runs just before capture.

## License note

`capture/vendor/single-file-bundle.js` is
[single-file-core](https://github.com/gildas-lormeau/single-file-core)
(AGPL-3.0-or-later) and is used as the final HTML serializer. All page
preparation is original code.
