#!/bin/sh
# Supervisor for the payments container, used only by the Phase 1 demo.
#
# The container's job here is to stay up (so Docker keeps its IP and its DNS
# record) while its listening socket can be closed independently. That is what
# makes the "connection refused" fault faithful: with the container stopped,
# clients fail in getaddrinfo and never call connect(), so there is no packet for
# eBPF to observe and the log says "Name or service not known" instead of
# "Connection refused". See demo/docker-compose.override.yml.
#
#   (no args)  start uvicorn in the background, then idle as PID 1
#   start      start uvicorn and wait until the port really answers
#   stop       stop uvicorn and wait until the port really refuses
#   status     report the listener state
#   logs       tail the worker's output
#
# stop and start are synchronous on purpose. A previous version backgrounded
# uvicorn and returned immediately, so a rapid fault-refuse/fault-restore cycle
# could kill the old worker, start a new one before the socket was released, and
# leave the service answering 500s with the worker's traceback written to a
# docker-exec stream nobody could read. `make -C demo verify` now catches that,
# but the fix is to not create it.
PIDFILE=/tmp/uvicorn.pid
LOGFILE=/tmp/uvicorn.log
CMD='uvicorn services.payments.main:app --host 0.0.0.0 --port 8002'

# Ask the port, not just the PID: a process can be alive and not listening.
port_open() {
  python3 - <<'PY' 2>/dev/null
import socket, sys
s = socket.socket()
s.settimeout(0.5)
sys.exit(0 if s.connect_ex(("127.0.0.1", 8002)) == 0 else 1)
PY
}

wait_for() { # wait_for <expect-open|expect-closed> <seconds>
  want=$1; i=0
  while [ "$i" -lt "$2" ]; do
    if [ "$want" = open ] && port_open; then return 0; fi
    if [ "$want" = closed ] && ! port_open; then return 0; fi
    i=$((i + 1)); sleep 0.5
  done
  return 1
}

case "${1:-serve}" in
  stop)
    if [ -f "$PIDFILE" ]; then
      kill "$(cat "$PIDFILE")" 2>/dev/null
      # uvicorn shuts down gracefully; give it a moment before forcing it.
      i=0
      while [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null && [ "$i" -lt 20 ]; do
        i=$((i + 1)); sleep 0.25
      done
      if kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
        kill -9 "$(cat "$PIDFILE")" 2>/dev/null
        sleep 0.5
      fi
      rm -f "$PIDFILE"
    fi
    if wait_for closed 10; then
      echo "payments listener stopped (port 8002 now refuses)"
    else
      echo "ERROR: port 8002 is still accepting connections; the fault did not take" >&2
      exit 1
    fi
    ;;
  start)
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null && port_open; then
      echo "payments listener already running (pid $(cat "$PIDFILE"))"
    else
      rm -f "$PIDFILE"
      # Append to the log so the worker's traceback survives the docker-exec
      # session that started it. `supervisor.sh logs` reads it back.
      $CMD >>"$LOGFILE" 2>&1 &
      echo $! > "$PIDFILE"
      if wait_for open 20; then
        echo "payments listener started (pid $(cat "$PIDFILE")) and is answering on 8002"
      else
        echo "ERROR: payments did not come up; worker output follows" >&2
        tail -20 "$LOGFILE" >&2
        exit 1
      fi
    fi
    ;;
  status)
    if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null && port_open; then
      echo "payments listener up (pid $(cat "$PIDFILE"))"
    else
      echo "payments listener DOWN"
      exit 1
    fi
    ;;
  logs)
    [ -f "$LOGFILE" ] || { echo "no worker log yet ($LOGFILE)"; exit 0; }
    tail -n "${2:-40}" "$LOGFILE"
    ;;
  serve)
    $CMD >>"$LOGFILE" 2>&1 &
    echo $! > "$PIDFILE"
    echo "payments supervisor up (uvicorn pid $(cat "$PIDFILE")); control it with: /supervisor.sh stop|start|status|logs"
    # If the worker ever dies on its own, put it back: the container is meant to
    # stay resolvable, and a silently dead listener looks like a hung demo.
    while true; do
      sleep 5
      if ! kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
        echo "payments worker died; restarting" >&2
        $CMD >>"$LOGFILE" 2>&1 &
        echo $! > "$PIDFILE"
      fi
    done
    ;;
  *) echo "usage: $0 [serve|start|stop|status|logs]"; exit 2 ;;
esac
