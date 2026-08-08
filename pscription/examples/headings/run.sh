#!/usr/bin/env bash
#
# Run a .rx pscription file that combines a built-in step (dismiss-popups)
# with a custom script (collect-headings.js), and print the captured result.
#
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
pscribe="$root/bin/pscribe"
[ -x "$pscribe" ] || (cd "$root" && go build -o bin/pscribe ./cmd/pscribe)

url="${1:-https://test-site-7/wiki/Web_archiving}"

"$pscribe" -r "$here/headings.rx" "$url" --only headings
