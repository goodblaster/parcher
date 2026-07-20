#!/usr/bin/env bash
#
# Capture-time highlighting: -highlight marks matching phrases BEFORE the
# page is serialized, so the highlights appear in EVERY output format —
# <mark> elements in the HTML archive, yellow PIXELS in the PDF and the
# screenshots. Useful when you know at capture time what the snapshot is
# meant to show (evidence, monitoring, review).
#
# Matching is the same loose, Ctrl-F-style engine as `parch find`:
# case/accent/punctuation-insensitive, never across paragraphs, `*`
# bridges words. Repeat -highlight for multiple phrases.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
parch="$root/bin/parch"
[ -x "$parch" ] || (cd "$root" && go build -o bin/parch ./cmd/parch)

url="${1:-https://example.com}"
out=$(mktemp -d)

for fmt in html pdf png; do
	echo "Capturing $url as $fmt with highlights ..."
	"$parch" -f "$fmt" \
		-highlight "example domain" -highlight "more information" \
		-o "$out/page.$fmt" "$url" 2>/dev/null
done

echo
echo "Compare the three — the same highlights, as markup and as pixels:"
ls -l "$out"
