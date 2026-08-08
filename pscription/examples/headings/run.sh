#!/usr/bin/env bash
#
# Run a .rx pscription file that combines a built-in step (dismiss-popups)
# with a custom script (collect-headings.js), and print the captured result.
#
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
# Real hosts for the reference sites live in a gitignored .sites.env
# (SITE-MAP.md explains why); without it this falls back to example.com.
[ -f "$root/.sites.env" ] && . "$root/.sites.env"
pscribe="$root/bin/pscribe"
[ -x "$pscribe" ] || (cd "$root" && go build -o bin/pscribe ./cmd/pscribe)

url="${1:-${TEST_URL_ARTICLE:-https://example.com}}"

"$pscribe" -r "$here/headings.rx" "$url" --only headings
