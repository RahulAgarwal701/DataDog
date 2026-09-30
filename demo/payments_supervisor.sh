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
#   start      start uvicorn only if it is not already running
#   stop       kill the uvicorn child, leaving the container up
#   status     report whether the listener is up
PIDFILE=/tmp/uvicorn.pid
CMD='uvicorn services.payments.main:app --host 0.0.0.0 --port 8002'

alive() { [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; }

case "${1:-serve}" in
  stop)
    if alive; then kill "$(cat "$PIDFILE")" 2>/dev/null; echo "payments listener stopped (port 8002 now refuses)"; fi
    rm -f "$PIDFILE"
    ;;
  start)
    if alive; then echo "payments listener already running (pid $(cat "$PIDFILE"))"
    else $CMD & echo $! > "$PIDFILE"; echo "payments listener started (pid $(cat "$PIDFILE"))"; fi
    ;;
  status)
    if alive; then echo "payments listener up (pid $(cat "$PIDFILE"))"; else echo "payments listener DOWN"; fi
    ;;
  serve)
    $CMD &
    echo $! > "$PIDFILE"
    echo "payments supervisor up (uvicorn pid $(cat "$PIDFILE")); control it with: /supervisor.sh stop|start|status"
    exec sleep infinity
    ;;
  *) echo "usage: $0 [serve|start|stop|status]"; exit 2 ;;
esac
