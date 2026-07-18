#!/usr/bin/env bash
#
# Run a CUSTOM user script (page-metadata.js) against a page and print its
# JSON result. This is the "handle user-created scripts" demo: rx doesn't
# know or care what the script does — it runs it in the page and captures
# whatever it returns.
#
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
rx="$root/bin/rx"
[ -x "$rx" ] || (cd "$root" && go build -o bin/rx ./cmd/rx)

url="${1:-https://www.test-site-7.org}"

# "<path>.js:meta" runs the script and captures its return value as "meta".
"$rx" "$url" "$here/page-metadata.js:meta" -only meta
