#!/usr/bin/env bash
# NFR-12: test coverage of the domain and usecase packages of every module.
#
#   scripts/coverage.sh [minimum percent, default 80]
#
# Runs the whole server test suite with coverage counted in every */domain and
# */usecase package (-coverpkg), so a usecase exercised by its module's API
# tests or the acceptance tests counts as covered. Prints the share of
# statements covered per package and for all of them together, and exits 1
# when the combined share is below the minimum.
# Needs Go, awk and the server's dependencies (npm is not involved).
set -euo pipefail

min=${1:-80}
root=$(cd "$(dirname "$0")/.." && pwd)
profile=$(mktemp)
trap 'rm -f "$profile" "$profile.raw"' EXIT

cd "$root/apps/server"
pkgs=()
while IFS= read -r pkg; do
  pkgs+=("$pkg")
done < <(go list ./... | grep -E '/(domain|usecase)$')
if [ ${#pkgs[@]} -eq 0 ]; then
  echo "coverage: no domain or usecase packages found" >&2
  exit 2
fi

coverpkg=$(IFS=,; echo "${pkgs[*]}")
go test -count=1 -coverpkg="$coverpkg" -coverprofile="$profile.raw" ./... >/dev/null
# Every test binary reports every block of the -coverpkg packages, so merge the
# blocks first: a block is covered when any binary ran it.
awk 'NR == 1 { print; next }
  { key = $1 " " $(NF - 1); if (!(key in seen)) order[++n] = key; seen[key] = 1; if ($NF > 0) ran[key] = 1 }
  END { for (i = 1; i <= n; i++) print order[i], (order[i] in ran ? 1 : 0) }' "$profile.raw" >"$profile"
rm -f "$profile.raw"

# A profile line is "file:start.col,end.col statements count" after the mode
# line. Count the statements per package directory, and the ones that ran.
packages=$(awk '
  NR == 1 { next }
  {
    file = $1
    sub(/:.*/, "", file)
    sub(/\/[^\/]*$/, "", file)
    sub(/^.*\/apps\/server\//, "", file)
    stmts[file] += $(NF - 1)
    if ($NF > 0) hit[file] += $(NF - 1)
  }
  END {
    for (p in stmts) printf "%6.1f%%  %6d statements  %s\n", 100 * hit[p] / stmts[p], stmts[p], p
  }' "$profile" | sort -k4)

total=$(awk '
  NR == 1 { next }
  {
    all += $(NF - 1)
    if ($NF > 0) covered += $(NF - 1)
  }
  END { printf "%.1f %d", (all ? 100 * covered / all : 0), all }' "$profile")

printf '%s\n' "$packages"
pct=${total% *}
statements=${total#* }
echo "total: ${pct}% of ${statements} statements"

if ! awk -v p="$pct" -v m="$min" 'BEGIN { exit !(p + 0 >= m + 0) }'; then
  echo "coverage: ${pct}% of domain and usecase statements is below the ${min}% minimum (NFR-12)" >&2
  exit 1
fi
echo "coverage: ${pct}% meets the ${min}% minimum (NFR-12)"
