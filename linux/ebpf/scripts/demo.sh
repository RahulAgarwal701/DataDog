#!/usr/bin/env bash
# Local stand-in for the dummy microservices (api-gateway -> orders -> payments -> inventory + loadgen),
# built from cmd/simservice, so the agent can be exercised before Dev's real services exist.
#
#   ./scripts/demo.sh start     start the chain + a 5 rps load generator
#   ./scripts/demo.sh slow      restart payments with 800 ms latency        (latency-fault demo)
#   ./scripts/demo.sh refuse    stop payments                               (connection-refused demo)
#   ./scripts/demo.sh restore   payments back to normal (50 ms)
#   ./scripts/demo.sh status | stop
set -euo pipefail
cd "$(dirname "$0")/.."

BIN=./bin/simservice
RUN=/tmp/minidd-sim
mkdir -p "$RUN"
[[ -x "$BIN" ]] || go build -o "$BIN" ./cmd/simservice

start_one() { # name port downstream delay
  local name=$1 port=$2 down=${3:-} delay=${4:-0s}
  local args=(-listen ":$port" -delay "$delay")
  [[ -n "$down" ]] && args+=(-downstream "$down")
  SERVICE_NAME="$name" "$BIN" "${args[@]}" >"$RUN/$name.log" 2>&1 &
  echo $! >"$RUN/$name.pid"
}

stop_one() {
  local f="$RUN/$1.pid"
  if [[ -f "$f" ]]; then
    kill "$(cat "$f")" 2>/dev/null || true
    rm -f "$f"
  fi
}

case "${1:-}" in
  start)
    start_one inventory   8003 ""                      10ms
    start_one payments    8002 http://127.0.0.1:8003   50ms
    start_one orders      8001 http://127.0.0.1:8002   20ms
    start_one api-gateway 8000 http://127.0.0.1:8001   5ms
    sleep 1
    SERVICE_NAME=loadgen "$BIN" -downstream http://127.0.0.1:8000 -rps 5 >"$RUN/loadgen.log" 2>&1 &
    echo $! >"$RUN/loadgen.pid"
    echo "started: api-gateway:8000 -> orders:8001 -> payments:8002 -> inventory:8003, loadgen at 5 rps"
    ;;
  slow)
    stop_one payments; sleep 0.5
    start_one payments 8002 http://127.0.0.1:8003 800ms
    echo "payments now adds 800 ms per request"
    ;;
  refuse)
    stop_one payments
    echo "payments stopped: orders -> payments will now be refused"
    ;;
  restore)
    stop_one payments; sleep 0.5
    start_one payments 8002 http://127.0.0.1:8003 50ms
    echo "payments restored (50 ms)"
    ;;
  stop)
    for n in loadgen api-gateway orders payments inventory; do stop_one "$n"; done
    echo "stopped"
    ;;
  status)
    for n in loadgen api-gateway orders payments inventory; do
      if [[ -f "$RUN/$n.pid" ]] && kill -0 "$(cat "$RUN/$n.pid")" 2>/dev/null; then echo "$n: running"; else echo "$n: stopped"; fi
    done
    ;;
  *)
    sed -n '2,10p' "$0"; exit 1 ;;
esac
