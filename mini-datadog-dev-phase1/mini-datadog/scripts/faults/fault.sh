#!/usr/bin/env bash
# fault.sh latency <svc> <ms> [secs] | error <svc> <rate 0..1> [secs] | refuse | restore | surge | baseline
# MODE=docker for Compose (default: local processes). HOST defaults to localhost.
H=${HOST:-localhost}; MODE=${MODE:-local}; C=${COMPOSE:-services/docker-compose.services.yml}
port(){ case $1 in api-gateway) echo 8000;; orders) echo 8001;; payments) echo 8002;; inventory) echo 8003;; esac; }
put(){ curl -sX PUT "$H:$(port $1)/admin/fault" -H 'content-type: application/json' -d "$2"; echo; }
d=${4:-null}
case $1 in
  latency) put $2 "{\"type\":\"latency\",\"latency_ms\":$3,\"duration_seconds\":$d}";;
  error)   put $2 "{\"type\":\"error\",\"error_rate\":$3,\"duration_seconds\":$d}";;
  refuse)  if [ $MODE = docker ]; then docker compose -f $C stop payments; else pkill -f "services.payments.main"; fi;;
  restore) for s in api-gateway orders payments inventory; do curl -sX DELETE "$H:$(port $s)/admin/fault" >/dev/null; done
           if [ $MODE = docker ]; then docker compose -f $C start payments
           elif ! curl -sf $H:8002/health >/dev/null; then SERVICE_NAME=payments PORT=8002 HTTP_KEEPALIVE=false nohup uvicorn services.payments.main:app --port 8002 --log-level warning >/dev/null 2>&1 & fi;;
  surge|baseline) curl -sX POST $H:8010/profile -H 'content-type: application/json' -d "{\"profile\":\"$1\"}"; echo;;
  *) sed -n 2,3p $0;;
esac
