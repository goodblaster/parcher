# parch examples

Each directory is a self-contained example with an executable `run.sh` that
builds `parch` (to `../../bin/parch`) if needed, then runs the demo. Most take
an optional URL argument.

```bash
examples/formats/run.sh                      # default demo URL
examples/formats/run.sh https://your.site    # your own URL
```

| Example | Shows |
|---|---|
| [`search-and-mark/`](search-and-mark) | **The full loop**: capture test-site-1 → OCR its images (`parch ocr`) → search the file (`parch find "music"` hits music-service cover art) → write a copy with several phrases highlighted, including inside images (`parch mark --grayscale`). |
| [`mark-at-capture/`](mark-at-capture) | Capture-time `-M`: the same phrases marked in `html`, `pdf`, and `png` output — markup in the archive, pixels in the renders. |
| [`formats/`](formats) | Capture one page as `html`, `mht`, `pdf`, `png`, `jpeg`, `webp` and compare sizes. |
| [`custom-prep/`](custom-prep) | `parch --pscribe` — prepare the page with a `.rx` pscription (built-in steps + your own script) before archiving. |
| [`links/`](links) | The three link policies (`keep`, `new-tab`, `disable`). |
| [`config/`](config) | A `.parch/config` TOML file: default flags, a shared HTTP cache, an output dir, and a `{host}/{date}/{title}.{ext}` filename template. |

## Config file

parch reads defaults from a TOML config, so you don't repeat flags. Precedence,
high to low: **CLI flags → `./.parch/config`** (nearest ancestor of the working
directory) **→ `~/.parch/config`** (global) **→ built-in defaults**.

```toml
# ~/.parch/config
cache_dir  = "~/.parch/cache"          # shared Chrome HTTP cache (speeds up repeat loads)
output_dir = "~/archives"              # where captures go (unless -o gives a path)
filename   = "{host}-{date}.{ext}"     # output filename template

[defaults]                             # any of these are overridable per-run by a flag
format  = "html"                       # html | mht | pdf | png | jpeg | webp
width   = 1600
links   = "keep"                       # keep | new-tab | disable
timeout = 300
profile = "~/.parch/profile"
```

Filename template tokens: `{host}` `{path}` `{title}` `{ext}`, and timestamps
`{date}` (2026-07-18), `{time}` (15-04-05), `{datetime}`, `{unix}`, plus a
custom Go layout `{date:2006-01}` → `2026-07`. A `/` in the template creates
subdirectories. `-o <path>` still wins and is used verbatim; `-o -` forces
stdout.

## The crawler-without-a-crawler pattern

parch pairs with [pscribe](../../pscription) (the script
runner). Extract links from an index page, then archive each:

```bash
pscribe https://blog.site @links -only links | jq -r '.[].url' | xargs -n1 parch
```

## Behind a login

`parch --pscribe login.rx --show <url>` runs a headful pscription (log in by hand at
a `wait:user` step) and then archives the authenticated page. See pscribe's
`examples/login/` for the pscription and credential-separation pattern.
