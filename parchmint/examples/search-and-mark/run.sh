#!/usr/bin/env bash
#
# The full search loop: archive a page, OCR its images, search the file,
# and produce a copy with every match highlighted — including matches
# that only exist as PIXELS inside images.
#
#   1. parch <url>          → one self-contained file with an embedded
#                             "text layer": every rendered word plus its
#                             page coordinates (TEXTLAYER.md)
#   2. parch ocr <file>   → OCR the archive's images (cover art,
#                             charts, frozen canvases…) into that layer.
#                             Works on the file alone — no browser, and
#                             explicitly opt-in because OCR is expensive
#   3. parch find  <file>   → search the file: no browser, no network,
#                             grep-like exit codes. `img` hits are text
#                             that lives inside an image
#   4. parch mark  <file>   → write a .marked.html copy: page-text
#                             matches wrapped in <mark>, image matches
#                             painted INTO the image (-grayscale mutes
#                             the image so the highlight pops). The
#                             original archive is never modified
#
# On test-site-1, "music" typically appears both as page text AND inside
# music-service cover-art tiles — step 2 is what makes the tiles findable.
# This is the archive→index→retrieve→view pipeline in miniature: store
# the .html files anywhere (the text layer travels inside them), index
# the layers in your search system, and when a document is found, mark
# it for display.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
parch="$root/bin/parch"
[ -x "$parch" ] || (cd "$root" && go build -o bin/parch ./cmd/parch)

url="${1:-https://www.test-site-1}"
out=$(mktemp -d)
archive="$out/page.html"

echo "1) Capturing $url (text layer embeds automatically) ..."
"$parch" -o "$archive" "$url" 2>/dev/null

echo
echo "2) OCR-indexing the archive's images ..."
"$parch" ocr "$archive"

echo
echo "3) Searching the file for \"music\" — img hits are inside images:"
"$parch" find "music" "$archive" || true

echo
echo "4) Highlighting several phrases at once (grayscale makes image hits pop) ..."
"$parch" mark --grayscale -o "$out/page.marked.html" "music" "iphone" "trade in" "$archive" 2>&1 |
	grep -vE '^[0-9]{4}-' || true

echo
echo "Archive:     $archive"
echo "Marked copy: $out/page.marked.html   ← open this one"
