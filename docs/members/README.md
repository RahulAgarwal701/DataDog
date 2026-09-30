# Supervisor Q&A guides — Phase 1

One file per team member. Each answers the questions a supervisor is likely to ask about
**that person's component**: what it does, why it was built that way, how to demo it, what
its limitations are, and what will go wrong if you are not careful.

Read your own file before the demo. Read `00-common-questions.md` once — it covers the
architecture and the cross-cutting questions everybody gets.

| File | Owns | Ports |
|---|---|---|
| [`00-common-questions.md`](00-common-questions.md) | shared / architecture | — |
| [`darsan.md`](darsan.md) | contracts, Platform API, dashboard | 9000, 3000 |
| [`rahul.md`](rahul.md) | metrics pipeline, Metrics API | 9001, 9090 |
| [`dev.md`](dev.md) | four services, loadgen, fault injection | 8000–8003, 8010 |
| [`diya.md`](diya.md) | log shipper, Elasticsearch, Logs API | 9002, 9200 |
| [`brajesh.md`](brajesh.md) | eBPF collector, Topology API | 9003 |

## How to use these

The full driving script is `demo/DRIVE.md` — that is the demo, in order, with timings. These
docs are the **Q&A prep** behind it: the "why" behind each step, so you can answer a follow-up
instead of just clicking through.

Three habits these docs push deliberately:

1. **Volunteer the limitation.** Every file has a "things that will trip you up" section.
   Naming a known weakness first is much stronger than being caught by it.
2. **Distinguish real from simulated.** The network data is either real eBPF or a labelled
   simulator. Never let the simulator be mistaken for the kernel.
3. **Do not overclaim the measurement window.** Prometheus runs on a 30s `rate()` window, so
   graphs lag and can appear to fall when they should rise. Know why before you are asked.

## Current known gaps

Found while writing these docs, all real, all unfixed:

- **Logs free-text search only covers `message`**, not `fields`. Searching `ECONNREFUSED` or
  `Errno` returns 0 hits even though the data is indexed. Detail is in `diya.md`.
- **The dashboard filters request traces client-side** over the newest 1000 logs, so it only
  reaches back ~45s at 5 rps. The API supports server-side filtering; the UI does not use it.
- **No unit tests in `mini-datadog-dev-phase1/`** — covered end-to-end by smoke/check, but no
  per-service pytest suite. Detail in `dev.md`.
- **Metrics tests are not in the runtime image.** `pytest` is dev-only for
  `metrics-api` (`requirements-dev.txt`), so the running container cannot run them; the logs
  container can, because its `requirements.txt` includes pytest. Runnable commands are in
  each doc.
- **eBPF refused-vs-timeout is a timing heuristic** (<1s ⇒ refused), because the tracepoint
  does not expose `sk_err`. Detail in `brajesh.md`.
- **Topology buffer is in-memory**, so restarting the Topology API clears the graph.
