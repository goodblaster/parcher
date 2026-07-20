#!/usr/bin/env bash
#
# Capture the same page in every format parch supports, and show the sizes.
# All are self-contained / offline-viewable:
#   html  self-contained page (default) — editable, opens anywhere
#   mht   Chrome MHTML — smaller, Chrome-only
#   pdf   selectable text, opens anywhere, native fonts
#   png   full-page screenshot, lossless
#   jpeg  full-page screenshot, much smaller
#   webp  full-page screenshot, smaller still
#
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
parch="$root/bin/parch"
[ -x "$parch" ] || (cd "$root" && go build -o bin/parch ./cmd/parch)

url="${1:-https://example.com}"
out=$(mktemp -d)

echo "Capturing $url ..."
for fmt in html mht pdf png jpeg webp; do
	"$parch" -f "$fmt" -o "$out/capture.$fmt" "$url" 2>/dev/null
	printf "  %-4s %9d bytes  %s\n" "$fmt" "$(wc -c < "$out/capture.$fmt")" "$out/capture.$fmt"
done
echo "(open any of them: they render with no network)"
