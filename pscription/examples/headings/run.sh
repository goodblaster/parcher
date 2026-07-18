#!/usr/bin/env bash
#
# Run a .rx pscription file that combines a built-in step (dismiss-popups)
# with a custom script (collect-headings.js), and print the captured result.
#
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
rx="$root/bin/rx"
[ -x "$rx" ] || (cd "$root" && go build -o bin/rx ./cmd/rx)

url="${1:-https://test-site-7/wiki/Web_archiving}"

"$rx" -f "$here/headings.rx" "$url" -only headings
