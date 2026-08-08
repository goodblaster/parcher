#!/usr/bin/env bash
#
# Log in interactively in a VISIBLE browser, then extract the page behind the
# login. A Chrome window opens — log in by hand, then press Enter in this
# terminal at the "Paused..." prompt. No credentials are stored anywhere.
#
# For automated login instead, see the comments in login.rx and
# secrets.example.json.
#
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
pscribe="$root/bin/pscribe"
[ -x "$pscribe" ] || (cd "$root" && go build -o bin/pscribe ./cmd/pscribe)

url="${1:-https://github.com/login}"

# --show launches a visible browser so you can complete the login.
"$pscribe" --show -r "$here/login.rx" "$url" --only content
