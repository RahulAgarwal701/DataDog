#!/usr/bin/env bash
# From repo root: bash services/run_local.sh
cd "$(dirname "$0")/.."; export HTTP_KEEPALIVE=false LOG_DIR=./logs
trap 'kill 0' EXIT
for s in "api_gateway 8000 api-gateway" "orders 8001 orders" "payments 8002 payments" "inventory 8003 inventory" "loadgen 8010 loadgen"; do
  set -- $s; SERVICE_NAME=$3 PORT=$2 uvicorn services.$1.main:app --port $2 --log-level warning &
done
wait
