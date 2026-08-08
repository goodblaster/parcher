#!/usr/bin/env bash
#
# Run a CUSTOM user script (page-metadata.js) against a page and print its
# JSON result. This is the "handle user-created scripts" demo: pscribe doesn't
# know or care what the script does — it runs it in the page and captures
# whatever it returns.
#
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
# Real hosts for the reference sites live in a gitignored .sites.env
# (SITE-MAP.md explains why); without it this falls back to example.com.
[ -f "$root/.sites.env" ] && . "$root/.sites.env"
pscribe="$root/bin/pscribe"
[ -x "$pscribe" ] || (cd "$root" && go build -o bin/pscribe ./cmd/pscribe)

url="${1:-${TEST_URL_PORTAL:-https://example.com}}"

# "<path>.js:meta" runs the script and captures its return value as "meta".
"$pscribe" "$url" "$here/page-metadata.js:meta" --only meta
