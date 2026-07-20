#!/usr/bin/env bash
#
# Archive a page with CUSTOM preparation: parch -rx runs a .rx pscription
# (built-in steps + your own scripts) to prepare the page, then serializes
# it. Here clean-archive.rx strips clutter before the snapshot is taken.
#
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
parch="$root/bin/parch"
[ -x "$parch" ] || (cd "$root" && go build -o bin/parch ./cmd/parch)

url="${1:-https://example.com}"
out=$(mktemp -d)/clean.html

"$parch" -rx "$here/clean-archive.rx" -o "$out" "$url"
echo "wrote $out ($(wc -c < "$out") bytes)"
