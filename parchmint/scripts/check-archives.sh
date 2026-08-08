#!/usr/bin/env bash
#
# check-archives.sh — capture each URL and prove the archive stands alone.
#
# This automates the verification flow in CLAUDE.md: capture, then render the
# result with the network blackholed, so anything that failed to embed shows
# up as a hole instead of quietly fetching itself from the live site.
#
# Read the two halves together. The counts catch what the eye cannot: a good
# capture leaves at most a handful of external references (tracking pixels, a
# CDN script tag, media the page never loaded), so a jump from 0 to dozens
# means resources went missing. The screenshots catch what the counts cannot:
# a page can be structurally perfect and still render blank, because prep
# damage looks exactly like an embed failure from here — `parch -f png`
# photographs the LIVE prepared page and tells the two apart (CLAUDE.md).
#
#     scripts/check-archives.sh                     # the reference sites
#     scripts/check-archives.sh https://example.com # or your own
#
# Was scripts/compare-serializers.sh, which A/B'd the native HTML backend
# against the vendored SingleFile one. That backend is gone and so is the
# comparison; what is left is the half that never depended on it.

set -uo pipefail

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/.." && pwd)
# Real hosts for the reference sites live in a gitignored .sites.env
# (SITE-MAP.md explains why); without it this falls back to example.com.
[ -f "$root/.sites.env" ] && . "$root/.sites.env"

PARCH=${PARCH:-./bin/parch}
OUTDIR=${OUTDIR:-./check-out}
WIDTH=${WIDTH:-1600}
HEIGHT=${HEIGHT:-3000}
CHROME=${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}

# The reference sites from CLAUDE.md: videos and product tiles, an age gate
# and a hero carousel, CJK with logo SVGs, WebGL under a strict CSP, 2D chart
# canvases, and cross-origin sample iframes inside shadow roots.
# The reference corpus comes from .sites.env (TEST_URL_CORPUS). MDN is public
# documentation, not a test subject, so it stays inline.
if [ -n "${TEST_URL_CORPUS:-}" ]; then
  read -r -a DEFAULT_URLS <<< "$TEST_URL_CORPUS"
else
  DEFAULT_URLS=(https://example.com)
fi
DEFAULT_URLS+=(https://developer.mozilla.org/en-US/docs/Web/API/Canvas_API/Tutorial/Basic_animations)

urls=("$@")
[ ${#urls[@]} -eq 0 ] && urls=("${DEFAULT_URLS[@]}")

if [ ! -x "$PARCH" ]; then
  echo "no binary at $PARCH — build it with: go build -o bin/ ./cmd/..." >&2
  exit 1
fi

mkdir -p "$OUTDIR"

# The virtual time budget is not optional: sites with entry animations
# (test-site-5 fades in from opacity:0) screenshot blank without it — the archive
# is fine, the screenshot just fired too early.
shoot() {
  "$CHROME" --headless=new --proxy-server="http://127.0.0.1:9" --hide-scrollbars \
    --window-size="$WIDTH,$HEIGHT" --virtual-time-budget=8000 \
    --screenshot="$2" "file://$(cd "$(dirname "$1")" && pwd)/$(basename "$1")" >/dev/null 2>&1
}

status=0
printf '%-46s %10s %9s %8s  %s\n' URL BYTES EXT-REFS SCRIPTS RESULT

for url in "${urls[@]}"; do
  slug=$(printf '%s' "$url" | sed -e 's|https\{0,1\}://||' -e 's|[^A-Za-z0-9._-]|_|g' | cut -c1-60)
  archive="$OUTDIR/$slug.html"

  "$PARCH" -f html -o "$archive" "$url" >"$OUTDIR/$slug.log" 2>&1
  if [ ! -s "$archive" ]; then
    printf '%-46s %10s %9s %8s  %s\n' "$slug" FAILED - - "see $OUTDIR/$slug.log"
    status=1
    continue
  fi

  shoot "$archive" "$archive.png"

  bytes=$(stat -f%z "$archive")
  # A reference still pointing at the network is a resource the archive does
  # not carry. Scripts should be the embedded text layer and ld+json only.
  refs=$(grep -o 'src="http\|url(http' "$archive" | wc -l | tr -d ' ')
  scripts=$(grep -o '<script' "$archive" | wc -l | tr -d ' ')

  # A handful of surviving references is normal and not a failure: tracking
  # pixels, a CDN script tag, media the page never actually loaded. What
  # matters is the TREND — a page that used to archive at 0 and now sits at
  # 40 has lost something. Only a capture that produced nothing is an error,
  # so this stays usable as a gate without crying wolf.
  result=ok
  if [ "$refs" -ne 0 ]; then result="$refs external ref(s) — check they are junk"; fi
  if [ ! -s "$archive.png" ]; then result="$result (no screenshot)"; fi

  printf '%-46s %10s %9s %8s  %s\n' "$slug" "$bytes" "$refs" "$scripts" "$result"
done

echo
echo "archives and offline screenshots in $OUTDIR — look at the PNGs; counts alone cannot see a blank page"
exit $status
