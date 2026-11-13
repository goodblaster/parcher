#!/usr/bin/env bash
# ocr-check.sh — the tesseract OCR path, end to end, with no network.
#
# Builds a page whose only copy of a word is INSIDE an image, captures it,
# OCRs the archive with tesseract, and requires `parch find` to locate the
# word in the image. This is the path Linux uses (macOS defaults to Apple
# Vision), so it is what CI runs to prove OCR works there at all.
#
#   PARCH=./bin/parch scripts/ocr-check.sh
#
# Work files go to $OUTDIR (default ./check-out/ocr-check, which is
# gitignored) and are overwritten on the next run. Nothing is deleted.
set -euo pipefail

PARCH=${PARCH:-./bin/parch}
OUTDIR=${OUTDIR:-./check-out/ocr-check}
mkdir -p "$OUTDIR"

if ! command -v tesseract >/dev/null; then
  echo "ocr-check: tesseract is not installed (apt install tesseract-ocr / brew install tesseract)" >&2
  exit 1
fi

# The word is drawn onto a canvas and exported as a PNG <img>, so it never
# exists as page text.
cat > "$OUTDIR/page.html" <<'HTML'
<!doctype html><html lang="en"><head><meta charset="utf-8"><title>ocr check</title></head>
<body><p>caption only</p><img id="shot" width="640" height="160" alt="">
<script>
  const c = document.createElement('canvas'); c.width = 640; c.height = 160;
  const g = c.getContext('2d');
  g.fillStyle = '#fff'; g.fillRect(0, 0, 640, 160);
  g.fillStyle = '#000'; g.font = 'bold 64px Helvetica, Arial, sans-serif';
  g.fillText('PARCHMENT 2026', 30, 100);
  document.getElementById('shot').src = c.toDataURL('image/png');
</script></body></html>
HTML

if ! "$PARCH" -o "$OUTDIR/archive.html" "$OUTDIR/page.html" 2>"$OUTDIR/capture.log"; then
  cat "$OUTDIR/capture.log" >&2
  exit 1
fi

if "$PARCH" find -q parchment "$OUTDIR/archive.html" 2>/dev/null; then
  echo "ocr-check: the word is in the page text, so this would prove nothing" >&2
  exit 1
fi

"$PARCH" ocr --engine tesseract "$OUTDIR/archive.html"
"$PARCH" find --source ocr parchment "$OUTDIR/archive.html"
echo "ocr-check: ok"
