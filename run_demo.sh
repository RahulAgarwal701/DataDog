#!/bin/bash
# Starts the 4 services as independent background processes, streams
# all of their live output into this one terminal with a [SOURCE]
# prefix so you can watch metrics/logs/traces/correlation all react at
# once, then runs the 1-request-per-second traffic generator.
set -e
cd "$(dirname "$0")"
rm -f *.log

PYTHON="$([ -x .venv/bin/python ] && echo .venv/bin/python || echo python3)"
$PYTHON metrics_service.py > metrics.log 2>&1 &
METRICS_PID=$!
$PYTHON logs_service.py > logs.log 2>&1 &
LOGS_PID=$!
$PYTHON traces_service.py > traces.log 2>&1 &
TRACES_PID=$!
$PYTHON correlation_service.py > correlation.log 2>&1 &
CORR_PID=$!

TAIL_PIDS=()
cleanup() {
    echo ""
    echo "Stopping services..."
    kill $METRICS_PID $LOGS_PID $TRACES_PID $CORR_PID 2>/dev/null
    for pid in "${TAIL_PIDS[@]}"; do kill "$pid" 2>/dev/null; done
}
trap cleanup EXIT

sleep 2
echo "All 4 services are up and running independently (metrics :5001, logs :5002, traces :5003, correlation :5004)."
echo "The correlation engine is already watching in the background."
echo ""

# Plain `tail -F`, one process each, no pipes - each Python service already
# tags its own lines (METRIC / LOG / SPAN / ANOMALY ...), so this alone is
# enough to tell the four streams apart once they're merged into one
# terminal.
tail -n0 -F metrics.log &     TAIL_PIDS+=($!)
tail -n0 -F logs.log &        TAIL_PIDS+=($!)
tail -n0 -F traces.log &      TAIL_PIDS+=($!)
tail -n0 -F correlation.log & TAIL_PIDS+=($!)

sleep 1
echo "Starting the traffic generator (1 request/second)..."
echo ""
$PYTHON mock_agent.py

echo ""
echo "Demo traffic finished. Leaving services running for a few more seconds"
echo "so the correlation engine can report the incident as resolved..."
sleep 8
