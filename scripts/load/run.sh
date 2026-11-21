#!/usr/bin/env bash
# make load: the load test for NFR-01 to NFR-04 (docs/load-test.md).
#
# Starts a throwaway copy of deploy/compose.prod.yml under its own project name
# (samnak-load) with two server instances behind Caddy, seeds it, runs the k6
# scripts from the grafana/k6 image on the same network, and tears the stack down
# with its volumes. Exits non-zero when a threshold or a check fails.
#
# Thresholds are the NFR targets unless REST_P95_MS or WS_P95_MS are set, which
# is how a missed threshold is shown to fail the run.
set -euo pipefail

cd "$(dirname "$0")/../.."

# The machine must stay awake: a sleep freezes every process for minutes, which
# shows up as latency (docs/load-test.md). caffeinate holds an idle-sleep
# assertion until this script exits; it changes no system setting.
if command -v caffeinate >/dev/null; then
  caffeinate -i -w $$ &
fi

project=samnak-load
out=$(mktemp -d "${TMPDIR:-/tmp}/samnak-load.XXXXXX")
export APP_ENV_FILE=../scripts/load/load.env
export LOAD_CADDYFILE=../scripts/load/Caddyfile
export LOAD_OUT_DIR=$out
rest_p95=${REST_P95_MS:-200}
ws_p95=${WS_P95_MS:-300}
failed=()
sampler=

compose() {
  docker compose -p "$project" --env-file scripts/load/load.env -f deploy/compose.prod.yml "$@"
}

instance_ips() {
  compose ps -q server | xargs docker inspect \
    -f '{{(index .NetworkSettings.Networks "samnak-load_default").IPAddress}}' | sort | paste -sd, -
}

# host_snapshot appends the machine's state to host.log: CPU and memory from top,
# the load average, and the stack's containers.
host_snapshot() {
  {
    echo "== $(date '+%H:%M:%S') $1"
    uptime
    top -l 1 -n 0 | sed -n '1,8p' | grep -E 'Load Avg|CPU usage|PhysMem|Processes' || true
    docker stats --no-stream --format '{{.Name}} cpu={{.CPUPerc}} mem={{.MemUsage}}' \
      $(compose ps -q) 2>/dev/null || true
    echo
  } >>"$out/host.log"
}

# sample_host runs in the background during the test, one line every 10 s.
sample_host() {
  while sleep 10; do
    echo "$(date '+%H:%M:%S') $(uptime)" >>"$out/load-average.log"
  done
}

# metrics_snapshot keeps the server counters of each instance after a phase.
metrics_snapshot() {
  local ips
  ips=$(instance_ips)
  compose --profile load run --rm --no-deps -T --entrypoint sh k6 -c "
    for ip in \$(echo '$ips' | tr , ' '); do
      echo \"-- \$ip\"
      wget -qO- http://\$ip:8080/metrics | grep -E '^(ws_|http_request_duration_seconds_count|go_goroutines|process_resident_memory_bytes|process_cpu_seconds_total)'
    done" >"$out/metrics-$1.txt" 2>&1 || true
}

# k6run runs one script and keeps its log and summary in $out. A failing run is
# recorded, not fatal, so the other phases still run.
k6run() {
  local name=$1 script=$2
  shift 2
  local args=()
  local kv
  for kv in "$@"; do args+=(-e "$kv"); done
  local rc=0
  compose --profile load run --rm --no-deps -T "${args[@]}" \
    -e "INSTANCES=$instances" -e "REST_P95_MS=$rest_p95" -e "WS_P95_MS=$ws_p95" \
    k6 run --no-color --summary-export "/out/$name.json" "/scripts/$script" 2>&1 \
    | tee "$out/$name.log" || rc=$?
  if [ "$rc" -ne 0 ]; then
    failed+=("$name (exit $rc)")
  fi
  return 0
}

cleanup() {
  kill "$sampler" 2>/dev/null || true
  compose --profile load down -v --remove-orphans >/dev/null 2>&1 || true
  rm -f "$out/fixture.json"
  echo "results: $out"
}
trap cleanup EXIT

echo "== stack $project: two server instances behind Caddy"
host_snapshot "before start"
compose up -d --build --wait --scale server=2

instances=$(instance_ips)
echo "server instances: $instances"

sample_host &
sampler=$!

echo "== seed"
k6run seed seed.js REST_USERS=40 SENDERS=10 DIRECT_SOCKETS=1000 FANOUT_SOCKETS=200
if [ ! -s "$out/fixture.json" ]; then
  echo "seed failed; see $out/seed.log" >&2
  exit 1
fi

echo "== REST mix (NFR-01), 100 requests/s for 90 s"
host_snapshot "before rest"
k6run rest rest.js REST_RPS=100 REST_DURATION_MS=90000
host_snapshot "after rest"
metrics_snapshot rest

echo "== 1,000 sockets on one instance (NFR-03), send to receive (NFR-02)"
host_snapshot "before ws-direct"
k6run ws-direct ws.js PHASE=direct SOCKETS=1000 MSGS_PER_SENDER=30 SEND_INTERVAL_MS=2000 RAMP_MS=20000 SEND_START_MS=40000
host_snapshot "after ws-direct"
metrics_snapshot ws-direct

echo "== 200 sockets through Caddy on two instances (NFR-04)"
host_snapshot "before ws-fanout"
k6run ws-fanout ws.js PHASE=fanout SOCKETS=200 MSGS_PER_SENDER=30 SEND_INTERVAL_MS=2000 RAMP_MS=20000 SEND_START_MS=40000
host_snapshot "after ws-fanout"
metrics_snapshot ws-fanout

kill "$sampler" 2>/dev/null || true

echo "== done"
if [ "${#failed[@]}" -gt 0 ]; then
  printf 'FAILED: %s\n' "${failed[@]}"
  exit 1
fi
echo "all thresholds and checks passed"
