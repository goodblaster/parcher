#!/usr/bin/env bash
#
# Extract the main article text from a page using the built-in @article
# script, and print just the clean text (nav/ads/boilerplate stripped) —
# the kind of thing you'd feed to an LLM.
#
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
pscribe="$root/bin/pscribe"
[ -x "$pscribe" ] || (cd "$root" && go build -o bin/pscribe ./cmd/pscribe)

url="${1:-https://test-site-7/wiki/Web_archiving}"

# @article auto-captures under the name "article"; --only prints that value.
# Pipe through jq for the text field if jq is installed.
if command -v jq >/dev/null; then
	"$pscribe" "$url" @article --only article | jq -r '.title, "", .text'
else
	"$pscribe" "$url" @article --only article
fi
