#!/usr/bin/env bash
#
# Show the three link policies. In a saved page, links still point at the
# live web — this controls what happens when you click one:
#   keep      unchanged (default) — behaves like the original page
#   new-tab   external links open in a new tab (never replaces the archive)
#   disable   links kept but unclickable (an evidence/record snapshot)
#
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
parch="$root/bin/parch"
[ -x "$parch" ] || (cd "$root" && go build -o bin/parch ./cmd/parch)

url="${1:-https://example.com}"
out=$(mktemp -d)

for policy in keep new-tab disable; do
	"$parch" -links "$policy" -o "$out/$policy.html" "$url" 2>/dev/null
	# SingleFile's compressHTML strips attribute quotes, so match href with
	# an optional quote; tolerate no match under `set -e`.
	link=$(grep -oiE '<a [^>]*href="?http[^>]*>' "$out/$policy.html" | head -1 || true)
	printf "  %-8s %s\n" "$policy" "${link:-<no external link found>}"
done
echo "(note target=_blank for new-tab; pointer-events disabled via CSS for disable)"
