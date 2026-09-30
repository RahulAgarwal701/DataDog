# Mini Datadog: Capstone Project (Phase I / Phase II)

VIT Bhopal University, B.Tech CSE

**Team:** Rahul Agarwal, Diya Garg, Dev Vyawahare, S. Darsan, Brajesh Mohanty

---

## Individual Contributions

| Member(s) | Module | Scope |
|---|---|---|
| Rahul Agarwal, Dev Vyawahare | Metrics + anomaly detection | Metrics collection, storage and querying pipeline, plus the statistical anomaly detector. Continuously receives service-health measurements, stores them as time series, supports time-range queries, and raises anomalies when current behaviour differs significantly from the normal pattern. |
| Brajesh Mohanty | eBPF + topology | eBPF component that observes network connections and converts endpoint-level activity into service-to-service relationships. Builds and updates the live dependency graph with no tracing libraries or code changes inside the monitored services. |
| Diya Garg | Logs + correlation | Centralized logging pipeline (built on the existing LogStream work) and the correlation engine. When an anomaly fires, it searches for relevant logs and topology events around the anomaly timestamp and attaches the strongest evidence to the incident. |
| S. Darsan | Platform + dashboard | APIs, dashboard, service integration, Docker/Kubernetes setup, and making all modules work together. The dashboard provides metrics, logs, topology and incident views; the platform layer defines shared interfaces between the independently developed components. |

---

## Phase-wise Execution Plan

Two academic phases, two reviews each. Review evidence should show only work that is actually implemented; planned items must be labelled as planned.

### Phase I, Review 1: Idea Pitch and Project Definition

No finished implementation required. Goal: establish the problem, why it matters, the proposed architecture, and the work split.

- Problem statement: observability signals are fragmented across metrics, logs and service communication data.
- Proposed architecture with monitored flow: API Gateway → Orders → Payments → Inventory.
- Objectives, motivation and the expected unified incident view.
- Literature review: observability, centralized logging, distributed tracing, and the planned eBPF-based topology approach.
- Freeze module ownership: metrics/anomaly detection; eBPF/topology; logs/correlation; platform/dashboard.
- Finalize common record formats for metrics, logs, network events and incident objects.
- Finalize implementation stack and repository/module structure.
- Prepare architecture diagram, correlation workflow, project schedule and risk list.

**Deliverables**
- Problem statement and motivation slides
- Architecture and workflow diagrams
- Initial literature review and references
- Module allocation and interface specification
- Technology-stack justification
- Repository skeleton / folder plan

### Phase I, Review 2: 25% Implementation Demonstration

Goal: prove basic observability data can be generated, collected and visualized in a small working environment. Full integration is not required.

- Dummy microservice environment: API Gateway, Orders, Payments, Inventory, with basic service-to-service calls.
- Baseline metrics: request rate, latency, error rate, CPU/memory where feasible.
- Initial metrics collection path and time-series storage prototype (Prometheus recommended).
- Structured service logs (service, severity, timestamp, message); ingest a first set into Elasticsearch.
- Basic Logs screen with search/filtering; basic Metrics screen with at least one time-series graph.
- eBPF proof-of-concept on Linux capturing network connection events from at least two services.
- Define and test shared JSON/API schemas between modules.
- One controlled fault (e.g. payment delay or connection refusal) showing metrics and logs reflect it, even without automatic correlation.

**Evidence**
- Running service demo
- Metrics screenshots/graphs
- Centralized log search screenshot
- eBPF network-event sample
- API/schema examples
- Git commit history showing module progress

### Phase II, Review 1: Intermediate Integrated Prototype

Target: roughly 60% of the final system. Major modules work individually and the first end-to-end integration is visible. Judge by completed functionality, not the percentage.

- Reliable metrics ingestion and query APIs for all dummy services.
- First statistical anomaly detector with a documented baseline/window and threshold method.
- Searchable centralized logs with timestamp-based retrieval around an anomaly window.
- eBPF endpoint observations converted into service-to-service topology edges, exposed via an API.
- Dashboard with Metrics, Logs and Topology views.
- First correlation path: anomaly → nearby logs → related service/topology events.
- Preliminary incident object: service, metric, timestamp, anomaly evidence, related logs, network/topology evidence.
- Major components containerized with Docker Compose, with repeatable startup.

**Evidence**
- Live dashboard with three views
- Anomaly detection demonstration
- Topology graph updated from observed network activity
- Preliminary correlated incident JSON/view
- Dockerized multi-service demo
- Known limitations and remaining-work list

### Phase II, Review 2: Final Integrated System and Validation

Target: complete the planned system and move from prototype evidence to validated end-to-end behaviour.

- Unified incident view combining metrics, logs and eBPF-derived network evidence.
- Refine anomaly thresholds and correlation window/ranking rules using repeatable fault scenarios.
- Full dashboard navigation: incident list/detail, metrics, logs, topology.
- Error handling, configuration management, and a clear service registry mapping IP/port to service name.
- Complete Docker deployment; Kubernetes deployment if infrastructure allows, otherwise a verified manifest/deployment walkthrough.
- Controlled experiments: latency spike, error burst, connection refusal, traffic surge. Record detection and correlation results.
- System-level evaluation: anomaly detection latency, incident construction latency, API response time, false positives on baseline traffic, resource overhead where feasible.
- Final screenshots, measured results, limitations, conclusion and future scope in the report.
- Final viva demo: one baseline scenario and at least one injected-failure scenario.
