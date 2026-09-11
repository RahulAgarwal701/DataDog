"""
Logs service (port 5002)
Ingests structured log lines (service, level, message, timestamp, trace_id)
and stores them in a plain in-memory list. This is the "LogStream" pillar.
Runs completely independently of the metrics/traces services.
"""
import time
import logging
import threading
from flask import Flask, request, jsonify

logging.getLogger("werkzeug").setLevel(logging.WARNING)

app = Flask(__name__)
lock = threading.Lock()
logs = []


@app.route("/logs", methods=["POST"])
def ingest_log():
    data = request.get_json(force=True)
    entries = data if isinstance(data, list) else [data]
    with lock:
        for e in entries:
            level = e.get("level", "INFO").upper()
            logs.append({
                "ts": float(e.get("timestamp") or time.time()),
                "service": e["service"],
                "level": level,
                "message": e.get("message", ""),
                "trace_id": e.get("trace_id"),
            })
            print(f"[{time.strftime('%H:%M:%S')}] LOG     [{level}] {e['service']}: {e.get('message','')}",
                  flush=True)
    return jsonify({"ingested": len(entries)})


@app.route("/logs/query")
def query_logs():
    service = request.args.get("service")
    level = request.args.get("level")
    trace_id = request.args.get("trace_id")
    start = float(request.args.get("start", 0))
    end = float(request.args.get("end", 1e18))
    with lock:
        result = [
            l for l in logs
            if start <= l["ts"] <= end
            and (not service or l["service"] == service)
            and (not level or l["level"] == level.upper())
            and (not trace_id or l["trace_id"] == trace_id)
        ]
    return jsonify(sorted(result, key=lambda l: l["ts"]))


@app.route("/health")
def health():
    return jsonify({"status": "ok", "log_count": len(logs)})


if __name__ == "__main__":
    print("logs_service listening on :5002 (independent process, in-memory store)", flush=True)
    app.run(port=5002)
