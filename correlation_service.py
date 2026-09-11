"""
Correlation service (port 5004)
This is the "AI layer". It runs completely independently of the other
three services and only ever talks to them over HTTP, the same way a
real Datadog-style correlation engine would.

Two modes, both available at once:
  1. A background WATCHER THREAD that polls every POLL_INTERVAL seconds,
     compares the last few seconds of each watched metric against a
     rolling baseline, and prints "ANOMALY STARTED" / "ANOMALY RESOLVED"
     the moment it notices - no one has to ask it anything.
  2. A POST /analyze endpoint for on-demand, one-shot analysis of a
     window (useful for scripting / curling directly).

Anomaly detector: median + MAD (median absolute deviation) z-score,
computed from a BASELINE window that excludes the last few seconds, so a
live spike doesn't drag its own baseline around while it's happening.
"""
import time
import logging
import threading
import statistics
from datetime import datetime

import requests
from flask import Flask, request, jsonify

logging.getLogger("werkzeug").setLevel(logging.WARNING)

app = Flask(__name__)

METRICS_URL = "http://localhost:5001"
LOGS_URL = "http://localhost:5002"
TRACES_URL = "http://localhost:5003"

# --- background watcher config ---
WATCH_TARGETS = [
    ("checkout-service", "latency_ms"),
    ("payment-service", "latency_ms"),
    ("inventory-service", "latency_ms"),
]
POLL_INTERVAL = 1.5        # how often the watcher checks in, seconds
RECENT_WINDOW = 4.0        # "just now" = last N seconds
BASELINE_LOOKBACK = 60.0   # how much history to build the baseline from
MIN_BASELINE_POINTS = 8
Z_THRESHOLD = 4.0

watch_state = {}  # (service, metric) -> {"active": bool, "start_ts": float, "peak": float}


def fmt(ts):
    return datetime.fromtimestamp(ts).strftime("%H:%M:%S")


def robust_baseline(values):
    med = statistics.median(values)
    mad = statistics.median(abs(v - med) for v in values) or 1e-9
    return med, mad


# ----------------------------------------------------------------------
# Background watcher: this is what makes the correlation engine feel like
# it's genuinely "looking over" the other three services, rather than
# only answering when asked.
# ----------------------------------------------------------------------
def check_target(service, metric):
    now = time.time()
    r = requests.get(f"{METRICS_URL}/metrics/query", params={
        "service": service, "metric": metric,
        "start": now - BASELINE_LOOKBACK, "end": now,
    })
    points = r.json()

    baseline_pts = [p for p in points if p["ts"] < now - RECENT_WINDOW]
    recent_pts = [p for p in points if p["ts"] >= now - RECENT_WINDOW]
    if len(baseline_pts) < MIN_BASELINE_POINTS or not recent_pts:
        return

    med, mad = robust_baseline([p["value"] for p in baseline_pts])
    recent_z = [(p, 0.6745 * (p["value"] - med) / mad) for p in recent_pts]
    anomalous_now = any(z > Z_THRESHOLD for _, z in recent_z)

    key = (service, metric)
    state = watch_state.setdefault(key, {"active": False, "start_ts": None, "peak": 0.0})

    if anomalous_now and not state["active"]:
        state["active"] = True
        state["start_ts"] = min(p["ts"] for p, z in recent_z if z > Z_THRESHOLD)
        state["peak"] = max(p["value"] for p in recent_pts)
        print(f"\n{'-'*72}", flush=True)
        print(f"ANOMALY STARTED  -  {service} / {metric}   "
              f"(baseline ~{med:.0f}, now {state['peak']:.0f})", flush=True)
        print(f"{'-'*72}", flush=True)

    elif anomalous_now and state["active"]:
        state["peak"] = max(state["peak"], max(p["value"] for p in recent_pts))

    elif not anomalous_now and state["active"]:
        announce_resolved(service, metric, state, med)
        watch_state[key] = {"active": False, "start_ts": None, "peak": 0.0}


def announce_resolved(service, metric, state, baseline_median):
    start_ts = state["start_ts"]
    end_ts = time.time()
    window_start, window_end = start_ts - 1, end_ts

    lr = requests.get(f"{LOGS_URL}/logs/query",
                       params={"service": service, "start": window_start, "end": window_end})
    error_logs = [l for l in lr.json() if l["level"] in ("ERROR", "WARN")]

    slow_threshold = baseline_median * 2 if baseline_median > 0 else 200
    tr = requests.get(f"{TRACES_URL}/traces/query",
                       params={"service": service, "start": window_start, "end": window_end,
                                "min_duration": slow_threshold})
    slow_traces = tr.json()

    print(f"\n{'='*72}", flush=True)
    print(f"ANOMALY RESOLVED  -  {service} / {metric}", flush=True)
    print(f"   Duration: {end_ts - start_ts:.1f}s    Window: {fmt(window_start)} - {fmt(window_end)}", flush=True)
    print(f"   Peak: {state['peak']:.1f}    Baseline: {baseline_median:.1f}", flush=True)
    print(f"   Correlated evidence: {len(slow_traces)} slow traces, {len(error_logs)} error/warn logs "
          f"in this window", flush=True)
    if slow_traces:
        print("   Sample slow trace(s):", flush=True)
        for t in slow_traces[:3]:
            print(f"     trace={t['trace_id'][:8]}  {t['service']}.{t['operation']}  "
                  f"duration={t['duration_ms']:.0f}ms  error={t['error']}", flush=True)
    if error_logs:
        print("   Sample log(s):", flush=True)
        for l in error_logs[:5]:
            print(f"     [{l['level']}] {fmt(l['ts'])} {l['service']}: {l['message']}", flush=True)
    print(f"{'='*72}", flush=True)


def watcher_loop():
    print(f"[correlation] watcher started - polling {len(WATCH_TARGETS)} target(s) "
          f"every {POLL_INTERVAL}s", flush=True)
    while True:
        for service, metric in WATCH_TARGETS:
            try:
                check_target(service, metric)
            except Exception as e:
                print(f"[correlation] watcher error for {service}/{metric}: {e}", flush=True)
        time.sleep(POLL_INTERVAL)


# ----------------------------------------------------------------------
# On-demand endpoint (unchanged behaviour from before): analyze a window
# right now and return the result, without waiting for the watcher's
# own timing.
# ----------------------------------------------------------------------
@app.route("/analyze", methods=["POST"])
def analyze():
    body = request.get_json(force=True)
    service = body["service"]
    metric = body.get("metric", "latency_ms")
    z_threshold = float(body.get("z_threshold", 3.5))
    lookback = float(body.get("lookback_seconds", 3600))
    now = time.time()
    start = now - lookback

    r = requests.get(f"{METRICS_URL}/metrics/query",
                      params={"service": service, "metric": metric, "start": start, "end": now})
    points = r.json()
    if len(points) < 5:
        return jsonify({"anomalies": [], "note": "not enough data points yet"})

    values = [p["value"] for p in points]
    med, mad = robust_baseline(values)
    zscores = [0.6745 * (v - med) / mad for v in values]
    anomaly_idxs = [i for i, z in enumerate(zscores) if z > z_threshold]
    if not anomaly_idxs:
        return jsonify({"anomalies": [], "baseline_median": round(med, 1)})

    episodes, cur = [], [anomaly_idxs[0]]
    for idx in anomaly_idxs[1:]:
        if points[idx]["ts"] - points[cur[-1]]["ts"] <= 5:
            cur.append(idx)
        else:
            episodes.append(cur)
            cur = [idx]
    episodes.append(cur)

    alerts = []
    for ep in episodes:
        ep_points = [points[i] for i in ep]
        ep_start, ep_end = ep_points[0]["ts"] - 2, ep_points[-1]["ts"] + 2
        peak = max(p["value"] for p in ep_points)

        lr = requests.get(f"{LOGS_URL}/logs/query",
                           params={"service": service, "start": ep_start, "end": ep_end})
        error_logs = [l for l in lr.json() if l["level"] in ("ERROR", "WARN")]

        slow_threshold = med * 2 if med > 0 else 200
        tr = requests.get(f"{TRACES_URL}/traces/query",
                           params={"service": service, "start": ep_start, "end": ep_end,
                                    "min_duration": slow_threshold})
        slow_traces = tr.json()

        alerts.append({
            "service": service, "metric": metric,
            "window": f"{fmt(ep_start)} - {fmt(ep_end)}",
            "peak_value": round(peak, 1), "baseline_median": round(med, 1),
            "slow_trace_count": len(slow_traces), "error_log_count": len(error_logs),
            "sample_traces": slow_traces[:3], "sample_logs": error_logs[:5],
        })

    return jsonify({"anomalies": alerts})


@app.route("/health")
def health():
    return jsonify({"status": "ok", "watching": WATCH_TARGETS})


if __name__ == "__main__":
    print("correlation_service listening on :5004 (independent process)", flush=True)
    threading.Thread(target=watcher_loop, daemon=True).start()
    app.run(port=5004)
