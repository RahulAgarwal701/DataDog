"""
Mock agent / traffic generator.

Plays 3 fake microservices (checkout-service, payment-service,
inventory-service). Sends ONE simulated request per second, in real
time, and prints a summary line for each tick so you can watch it
unfold. This is the "start the demo" script - run it after the four
services are already up. It doesn't call the correlation service
directly; the correlation service is watching on its own in the
background and will print its own alerts as they happen.

Phases:
  1. normal traffic      - baseline latency, ~15 ticks
  2. injected incident   - checkout-service latency spikes + errors, ~6 ticks
  3. recovery            - back to normal, ~10 ticks
"""
import os
import time
import random
import uuid

import requests

METRICS_URL = "http://localhost:5001/metrics"
LOGS_URL = "http://localhost:5002/logs"
SPANS_URL = "http://localhost:5003/spans"

# Configurable via env vars if you want a faster/slower demo, e.g.:
#   TICK_SLEEP=0.2 NORMAL_TICKS_BEFORE=5 INCIDENT_TICKS=3 NORMAL_TICKS_AFTER=5 python3 mock_agent.py
NORMAL_TICKS_BEFORE = int(os.environ.get("NORMAL_TICKS_BEFORE", 15))
INCIDENT_TICKS = int(os.environ.get("INCIDENT_TICKS", 6))
NORMAL_TICKS_AFTER = int(os.environ.get("NORMAL_TICKS_AFTER", 10))
TICK_SLEEP = float(os.environ.get("TICK_SLEEP", 1.0))


def send_tick(tick_num, total, anomaly):
    t = time.time()
    trace_id = uuid.uuid4().hex
    root_span_id = uuid.uuid4().hex[:8]
    payment_span_id = uuid.uuid4().hex[:8]
    inventory_span_id = uuid.uuid4().hex[:8]

    checkout_latency = max(5, random.gauss(80, 12))
    payment_latency = max(5, random.gauss(40, 8))
    inventory_latency = max(5, random.gauss(20, 5))
    error = False

    if anomaly:
        checkout_latency = max(5, random.gauss(950, 120))
        payment_latency = max(5, random.gauss(700, 100))
        error = random.random() < 0.6

    requests.post(METRICS_URL, json=[
        {"service": "checkout-service", "metric": "latency_ms", "value": checkout_latency, "timestamp": t},
        {"service": "payment-service", "metric": "latency_ms", "value": payment_latency, "timestamp": t},
        {"service": "inventory-service", "metric": "latency_ms", "value": inventory_latency, "timestamp": t},
    ])

    if anomaly:
        logs = [
            {"service": "checkout-service", "level": "ERROR", "message": "payment gateway timeout",
             "timestamp": t, "trace_id": trace_id},
            {"service": "payment-service", "level": "ERROR", "message": "upstream 504 from payment gateway",
             "timestamp": t, "trace_id": trace_id},
        ]
        if random.random() < 0.5:
            logs.append({"service": "checkout-service", "level": "WARN",
                         "message": "retrying payment call", "timestamp": t, "trace_id": trace_id})
    else:
        logs = [{"service": "checkout-service", "level": "INFO", "message": "order processed",
                 "timestamp": t, "trace_id": trace_id}]
    requests.post(LOGS_URL, json=logs)

    requests.post(SPANS_URL, json=[
        {"trace_id": trace_id, "span_id": root_span_id, "parent_span_id": None,
         "service": "checkout-service", "operation": "POST /checkout",
         "start_ts": t, "duration_ms": checkout_latency, "error": error},
        {"trace_id": trace_id, "span_id": payment_span_id, "parent_span_id": root_span_id,
         "service": "payment-service", "operation": "charge_card",
         "start_ts": t, "duration_ms": payment_latency, "error": error},
        {"trace_id": trace_id, "span_id": inventory_span_id, "parent_span_id": root_span_id,
         "service": "inventory-service", "operation": "reserve_stock",
         "start_ts": t, "duration_ms": inventory_latency, "error": False},
    ])

    tag = "  <-- INCIDENT" if anomaly else ""
    print(f"[{time.strftime('%H:%M:%S')}] AGENT   tick {tick_num}/{total}  "
          f"checkout={checkout_latency:.0f}ms payment={payment_latency:.0f}ms "
          f"inventory={inventory_latency:.0f}ms{tag}", flush=True)


def main():
    total = NORMAL_TICKS_BEFORE + INCIDENT_TICKS + NORMAL_TICKS_AFTER
    print(f"Traffic generator starting: {total} requests, 1/second, real time.", flush=True)
    print("checkout-service -> payment-service -> inventory-service (one trace per request)\n", flush=True)

    tick = 0
    for _ in range(NORMAL_TICKS_BEFORE):
        tick += 1
        send_tick(tick, total, anomaly=False)
        time.sleep(TICK_SLEEP)

    print("\n>>> injecting incident on checkout-service now <<<\n", flush=True)
    for _ in range(INCIDENT_TICKS):
        tick += 1
        send_tick(tick, total, anomaly=True)
        time.sleep(TICK_SLEEP)

    print("\n>>> traffic recovering <<<\n", flush=True)
    for _ in range(NORMAL_TICKS_AFTER):
        tick += 1
        send_tick(tick, total, anomaly=False)
        time.sleep(TICK_SLEEP)

    print("\nTraffic generator done. The correlation engine has been watching the "
          "whole time - scroll up (or check its own terminal) for the "
          "ANOMALY STARTED / RESOLVED alerts it printed live.", flush=True)


if __name__ == "__main__":
    main()
