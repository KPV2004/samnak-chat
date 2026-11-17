#!/usr/bin/env bash
# PLT-12: start the server with one MODULES_ENABLED setting and check what it
# serves: /healthz, /meta (exactly the enabled modules) and the routes of the
# optional modules (404 when disabled, reachable when enabled).
#
#   scripts/check-modules.sh <MODULES_ENABLED | unset> <enabled optional modules, comma-separated | "">
#
# Needs DATABASE_URL, REDIS_URL, S3_* and JWT_SECRET as cmd/server reads them.
# The database is migrated for this setting first (cmd/migrate). HTTP_ADDR
# picks the port; the default is :8080.
set -euo pipefail

if [ $# -ne 2 ]; then
  echo "usage: $0 <MODULES_ENABLED | unset> <enabled optional modules | \"\">" >&2
  exit 2
fi
setting=$1
enabled=$2
port=${HTTP_ADDR:-:8080}
port=${port##*:}
base="http://localhost:$port"

work=$(mktemp -d)
pid=
cleanup() {
  if [ -n "$pid" ]; then
    kill "$pid" 2>/dev/null || true
    wait "$pid" 2>/dev/null || true
  fi
  rm -rf "$work"
}
trap cleanup EXIT

fail() {
  echo "FAIL [$setting]: $*" >&2
  cat "$work/server.log" >&2 2>/dev/null || true
  exit 1
}

if [ "$setting" = unset ]; then
  unset MODULES_ENABLED
else
  export MODULES_ENABLED=$setting
fi

cd "$(dirname "$0")/../apps/server"
go build -o "$work/" ./cmd/migrate ./cmd/server
"$work/migrate"
"$work/server" > "$work/server.log" 2>&1 &
pid=$!

ready=
for _ in $(seq 60); do
  if curl -fsS "$base/healthz" > /dev/null 2>&1; then
    ready=1
    break
  fi
  kill -0 "$pid" 2>/dev/null || fail "server exited during startup"
  sleep 0.5
done
[ -n "$ready" ] || fail "no /healthz answer within 30s"

curl -fsS "$base/healthz" | jq -e '.status == "ok"' > /dev/null || fail "/healthz is not ok"

# Core modules are always listed; the rest must be exactly the enabled ones.
want=$({ printf '%s\n' auth chat workspace; printf '%s\n' "$enabled" | tr ',' '\n' | sed '/^$/d'; } | sort | paste -sd, -)
got=$(curl -fsS "$base/api/v1/meta" | jq -r '.modules | sort | join(",")')
[ "$got" = "$want" ] || fail "/meta lists [$got], want [$want]"

route() {
  case $1 in
    notify) echo /api/v1/notify/notifications ;;
    task) echo /api/v1/task/items ;;
    approval) echo /api/v1/approval/requests ;;
  esac
}
for mod in notify task approval; do
  code=$(curl -s -o /dev/null -w '%{http_code}' "$base$(route "$mod")")
  if [[ ",$enabled," == *",$mod,"* ]]; then
    # Without a token an enabled route answers 401, which proves it is mounted.
    [ "$code" = 401 ] || fail "$mod is enabled but its route answers $code, want 401"
  else
    [ "$code" = 404 ] || fail "$mod is disabled but its route answers $code, want 404"
  fi
done

echo "PASS [$setting]: /healthz ok, /meta lists [$want], module routes match"
