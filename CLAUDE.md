# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**parcher** is one Go module (`github.com/goodblaster/parcher`) holding two components that ship together, because prep is only meaningful when something serializes the result:

| Directory | Binary | What it is |
|---|---|---|
| [`parchmint/`](parchmint) | `parch` | page archiver — captures a URL as a single self-contained file that renders faithfully offline, with a searchable text layer |
| [`pscription/`](pscription) | `pscribe` | browser script runner — executes a *pscription* (an ordered list of prep steps and scripts) against a live page |

The line between them: **pscription = live pages, parchmint = files.** parchmint imports pscription (`runner`, `pipeline.Staticize()`, `actions`, `scripts`) for browser driving and page preparation; nothing goes the other way.

**Primary language:** Go 1.26.
**Platforms:** macOS and Linux, both built and tested in CI. Linux builds are pure Go; macOS builds use cgo for Apple Vision OCR.
**Runtime requirement:** a Chrome/Chromium installation (not bundled). `parch ocr` on Linux also needs `tesseract`.

**Read the component CLAUDE.md before changing anything inside it** — [parchmint/CLAUDE.md](parchmint/CLAUDE.md) and [pscription/CLAUDE.md](pscription/CLAUDE.md) carry the architecture and the hard-won invariants (each one records a real bug), and they are not duplicated here. Capture-backend invariants — encoder limits, PDF quirks, what Chrome's MHTML writer drops — are parchmint's; prep-side invariants — dialog layering, WebGL buffer preservation, OOPIF freezing, CSP bypass, over-hiding — are pscription's. Capture work usually touches both.

## One module, one repository

parchmint and pscription were once two repositories with two module paths, published separately and joined by a `go.work` and a version pin. **That is over.** This repository is the whole of both, and the only place either is developed. What follows from that:

- **No `go.work`, no version pin, no release to coordinate.** A change in `pscription/` is picked up by `parch` on the next build. Cross-component changes are one commit and one PR.
- **One worktree covers both.** `git worktree add /tmp/baseline HEAD` gives a complete buildable baseline for before/after comparisons.
- Imports are `github.com/goodblaster/parcher/{parchmint,pscription}/…`. The old `github.com/goodblaster/parchmint` and `…/pscription` module paths appear nowhere in the tree and must not come back.
- The `rx` binary was renamed `pscribe`; `.rx` is still the pscription **file** extension, and `parch --recipe file.rx` is the flag that runs one as prep (the same flag name `pscribe` uses; `--pscribe` is its deprecated spelling).

**Ignore the sibling `parchmint/` and `pscription/` directories one level up.** They are the pre-merge checkouts, kept on disk for history only — stale, not built, not imported, and carrying their own outdated CLAUDE.md files. Never read them for current behavior and never edit them; everything live is under this repository root. Inside the component docs, a link like `../pscription` means this repo's `pscription/`, not that one.

## Build & Test

```bash
make          # both commands into ./bin (default target)
make install  # both into GOBIN, version-stamped
make test     # go test ./...
make vet
make lint     # vet + staticcheck + govulncheck (run via `go run`, so always built with this toolchain)
make probe    # browser-driven probe suite on a local page — needs Chrome, no network
make cross    # compile every release target (linux/darwin × amd64/arm64) as a build check
make dist     # release binaries into ./dist/<os>-<arch>/
make ocr-check  # tesseract OCR end to end on a local page — needs Chrome and tesseract
make check    # reference-site capture check — needs Chrome and .sites.env
make help
```

`go build ./...` and `go install ./...` work, but leave the binaries reporting version `dev`: the version is derived from `git describe` and stamped through `-ldflags`, and only the Makefile does that.

`go test ./...` covers the pure-Go core — the `textlayer` normalizer/matcher, container sniffing and embed round-trips, the html↔mht converter, the tesseract TSV parser — plus a node-based parse check over every embedded script (skipped when node is absent). Browser-driven behavior has two checks. `make probe` captures a local probe page and asserts the things that are lost SILENTLY — hidden text filed as visible, `hidden`-attribute content missing from the file, a dangling inline-style `url()`, `mark --hidden` — with no network. `make check` (`parchmint/scripts/check-archives.sh`) captures the reference sites, re-renders each archive with the network blackholed, and reports bytes, surviving external references, and script count. Counts cannot see a blank page, so look at the PNGs too. See parchmint's Verification section for how to read the output and for the trick that separates prep damage from serialization loss (`-f png` photographs the LIVE prepared page).

## Site labels: `test-site-N`, never real hostnames

Comments and docs cite measurements against real sites, and refer to them as `test-site-1`, `test-site-2`, … The evidence is what matters; the names are not published.

- **Never write a real hostname into a tracked `.go`/`.js`/`.md`/`.sh` file.** Use the label, or read the host from the environment.
- `.sites.env` (gitignored) holds the mapping and the example URLs; shell scripts source it, Go can load it with godotenv, and `examples/*/run.sh` read those variables rather than hardcoding sites.
- `SITE-MAP.md` (gitignored) is the only record of the mapping, plus the redaction rules: which strings are functional and must NOT be redacted (`AppleWebKit`, `apple-touch-icon`, `performAppleVisionOCR`, the tracker/junk-iframe host lists, YouTube), why bare `apple` can never be bulk-replaced, and why a `<word>.com` regex destroys code (`regexp.Compile` → `regexp.com`). Read it before renaming or adding a label.
- **A label change can break a test silently.** Chrome provenance markers encode the URL's character count as their prefix — `<!-- saved from url=(0024)https://www.test-site-3/ -->` — so relabelling a site changes the number. Re-check `parchmint/cmd/parch/convert.go:227` and `parchmint/cmd/parch/convert_test.go:22` after any relabel; `TestSavedFromURL/chrome_marker` is what catches it. The deliberately-wrong counts in the other fixtures are testing bad input — leave them alone.

Local-only material also stays out of git: `local-reference/` and `*-PLAN.md` are ignored by design.

**Capture outputs land in the working tree.** A bare `parch https://…` writes `<host>.html` next to you — multi-megabyte files named after real hosts. Root-level `*.html`/`*.pdf`/image outputs are gitignored for exactly that reason, but a capture written into a SUBDIRECTORY is not, so still never `git add -A` here without looking; write captures to `./captures/` or `./check-out/` (both ignored) when you can.

## Shared conventions

- Error wrapping via `errors.Wrap(err, "context")` (`github.com/goodblaster/errors`). **`Wrap(nil, msg)` returns a NON-NIL error** — never tail-return it unguarded. This has bitten both components.
- Flags use **[spf13/pflag](https://github.com/spf13/pflag)**, not Go's `flag`: long name canonical, short registered as a shorthand (`StringVarP`), so `-o`/`--output` are one flag; flags may follow positionals; retire spellings with `MarkDeprecated`. **Look flags up by their canonical long name** — keying on a shorthand silently reads as "not set". One meaning per letter across every subcommand.
- CLI: logs → **stderr**, content → stdout or the output file. Exit 0 success, 1 the operation failed, 2 the invocation was wrong — in both binaries.
- **The two binaries share flag names** for everything they both have (`--recipe`, `--profile`, `--show`, `--timeout`, `--width`, `--height`, `--dpr`, `--user-agent`, `-v`). Add an option to one with the name the other already uses.
- **The browser driver is behind `pscription/act`.** A step is an `act.Func` (`func(ctx) error`); protocol commands go through `chromedp.Call(ctx, page.X, page.XParams{…})`, page script through `act.Evaluate`. chromedp's own action type used to BE this module's action type, so its v0.17+ API rewrite reached 44 files; nothing outside `act` and `runner` should name a chromedp action type again.
- **`go.mod` pins the Go patch release, and that is a security control.** Standard-library fixes ship only in patch releases: a binary built with go1.26.0 carried 14+ known net/http, crypto/tls and crypto/x509 vulnerabilities that go1.26.11 fixes, with no dependency out of date. The `go` line makes every build fetch a patched toolchain. Raise it when `make lint` (govulncheck) reports the standard library; CI runs govulncheck on every push.
- Each component has an `internal/log` seam that delegates to the package-level `goodblaster/logos` logger, so configuring logos once (as `parch`'s main does) routes both components' library logs.
- New prep behavior is a named pipeline step in pscription (JS file → `ScriptFile` constant → action → step + `Register`), not inline workflow code. New capture format is a `capture.Backend` in parchmint registering itself from its own file's `init` — the CLI needs no edit.

## License

[PolyForm Small Business 1.0.0](LICENSE.md) — source-available, not open source. Free below 100 people / $1M revenue; above that, commercial licensing is dave@goodblaster.com.

**Every line here is original and every dependency is permissively licensed** (MIT/BSD/ISC/Apache-2.0), so nothing carries a copyleft obligation onto what users build. This was not always true: `capture/vendor/single-file-bundle.js` (single-file-core, AGPL-3.0-or-later) was the HTML serializer before the native backend replaced it. **Do not reintroduce it, or any third-party serializer, without deciding the licensing question first** — an AGPL dependency reaches the whole combined work, and "it's only a build tag" does not help while the bytes ship in the binary. The backend registry exists partly to keep that swap a one-file operation.
