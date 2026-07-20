#!/usr/bin/env bash
#
# Layered config: a .parch/config TOML file supplies defaults so you don't
# repeat flags. Precedence, high to low:
#   1. CLI flags
#   2. ./.parch/config   (nearest, found by walking up from the cwd)
#   3. ~/.parch/config   (global)
#   4. built-in defaults
#
# This demo sets up a throwaway workspace with its own .parch/config, then
# shows (a) a capture driven entirely by the config, (b) a CLI flag winning
# over the config, and (c) the shared HTTP cache directory getting populated
# so repeat runs reuse fetched resources.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
parch="$root/bin/parch"
[ -x "$parch" ] || (cd "$root" && go build -o bin/parch ./cmd/parch)

url="${1:-https://example.com}"
ws=$(mktemp -d)
mkdir -p "$ws/.parch"

cat > "$ws/.parch/config" <<EOF
# Where captures go and how they're named.
output_dir = "$ws/archives"
filename   = "{host}/{date}/{title}.{ext}"

# A shared HTTP cache reused across runs (speeds up repeat loads).
cache_dir  = "$ws/cache"

# Default flags — overridable on the command line.
[defaults]
format = "pdf"
width  = 1200
links  = "new-tab"
EOF

echo "workspace: $ws"
echo "config:"; sed 's/^/    /' "$ws/.parch/config"; echo

# parch discovers ./.parch/config by walking up from the working directory.
run() { ( cd "$ws" && "$parch" "$@" ) ; }

echo "1) Everything from config (pdf, 1200px, templated path):"
run "$url" 2>/dev/null
find "$ws/archives" -type f | sed 's/^/    /'
echo

echo "2) CLI flag wins — '-f png' overrides the config's format=pdf:"
run -f png "$url" 2>/dev/null
find "$ws/archives" -type f -name '*.png' | sed 's/^/    /'
echo

echo "3) Shared cache populated (reused on later runs):"
printf "    %s files under %s\n" "$(find "$ws/cache" -type f | wc -l | tr -d ' ')" "$ws/cache"
