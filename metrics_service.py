"""
Metrics service (port 5001)
Ingests time-series metrics (service, metric name, value, timestamp) and
stores them in memory, keyed by (service, metric). No DB - just a dict.
Runs completely independently of the logs/traces services.
"""
import time
import logging
import threading
from collections import defaultdict
from flask import Flask, request, jsonify

logging.getLogger("werkzeug").setLevel(logging.WARNING)  # quiet the per-request access log

app = Flask(__name__)
lock = threading.Lock()
store = defaultdict(list)  # (service, metric) -> [{"ts": float, "value": float}, ...]


@app.route("/metrics", methods=["POST"])
def ingest_metric():
    data = request.get_json(force=True)
    points = data if isinstance(data, list) else [data]
    with lock:
        for p in points:
            key = (p["service"], p["metric"])
            value = float(p["value"])
            store[key].append({
                "ts": float(p.get("timestamp") or time.time()),
                "value": value,
            })
            print(f"[{time.strftime('%H:%M:%S')}] METRIC  {p['service']}.{p['metric']} = {value:.1f}",
                  flush=True)
    return jsonify({"ingested": len(points)})


@app.route("/metrics/query")
def query_metric():
    service = request.args.get("service")
    metric = request.args.get("metric")
    start = float(request.args.get("start", 0))
    end = float(request.args.get("end", 1e18))
    key = (service, metric)
    with lock:
        pts = [p for p in store.get(key, []) if start <= p["ts"] <= end]
    return jsonify(sorted(pts, key=lambda p: p["ts"]))


@app.route("/metrics/keys")
def list_keys():
    with lock:
        keys = [{"service": s, "metric": m} for (s, m) in store.keys()]
    return jsonify(keys)


@app.route("/health")
def health():
    return jsonify({"status": "ok", "series_count": len(store)})


if __name__ == "__main__":
    print("metrics_service listening on :5001 (independent process, in-memory store)", flush=True)
    app.run(port=5001)
