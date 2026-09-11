"""
Traces service (port 5003)
Ingests spans (trace_id, span_id, parent_span_id, service, operation,
start_ts, duration_ms, error) and can stitch a trace_id back into its
full span tree, or query for slow/erroring traces in a time window.
Runs completely independently of the metrics/logs services.
"""
import time
import logging
import threading
from flask import Flask, request, jsonify

logging.getLogger("werkzeug").setLevel(logging.WARNING)

app = Flask(__name__)
lock = threading.Lock()
spans = []


@app.route("/spans", methods=["POST"])
def ingest_spans():
    data = request.get_json(force=True)
    entries = data if isinstance(data, list) else [data]
    with lock:
        for s in entries:
            duration_ms = float(s["duration_ms"])
            error = bool(s.get("error", False))
            spans.append({
                "trace_id": s["trace_id"],
                "span_id": s["span_id"],
                "parent_span_id": s.get("parent_span_id"),
                "service": s["service"],
                "operation": s.get("operation", ""),
                "start_ts": float(s["start_ts"]),
                "duration_ms": duration_ms,
                "error": error,
            })
            flag = " !ERROR" if error else ""
            print(f"[{time.strftime('%H:%M:%S')}] SPAN    {s['service']}.{s.get('operation','')} "
                  f"dur={duration_ms:.0f}ms trace={s['trace_id'][:8]}{flag}", flush=True)
    return jsonify({"ingested": len(entries)})


@app.route("/traces/query")
def query_traces():
    """Find traces whose span for `service` started in [start,end] and is
    at least `min_duration` ms long (i.e. the 'slow traces' lookup used by
    the correlation layer). Returns one representative span per trace."""
    service = request.args.get("service")
    start = float(request.args.get("start", 0))
    end = float(request.args.get("end", 1e18))
    min_duration = float(request.args.get("min_duration", 0))
    with lock:
        matches = [
            s for s in spans
            if start <= s["start_ts"] <= end
            and (not service or s["service"] == service)
            and s["duration_ms"] >= min_duration
        ]
    by_trace = {}
    for s in matches:
        # keep the slowest span per trace as the representative one
        if s["trace_id"] not in by_trace or s["duration_ms"] > by_trace[s["trace_id"]]["duration_ms"]:
            by_trace[s["trace_id"]] = s
    return jsonify(list(by_trace.values()))


@app.route("/traces/<trace_id>")
def get_trace(trace_id):
    with lock:
        tspans = [s for s in spans if s["trace_id"] == trace_id]
    return jsonify(sorted(tspans, key=lambda s: s["start_ts"]))


@app.route("/health")
def health():
    return jsonify({"status": "ok", "span_count": len(spans)})


if __name__ == "__main__":
    print("traces_service listening on :5003 (independent process, in-memory store)", flush=True)
    app.run(port=5003)
