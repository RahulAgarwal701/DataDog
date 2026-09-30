# Mini Datadog - Phase 1 demo entry points.
#
#   make demo     build + start everything, wait until healthy, print the URLs
#   make smoke    validate every Platform API response against the contracts
#   make logs     tail the log shipper
#   make faults   exercise latency -> error -> connection-refused -> restore
#
# `make -C demo help` lists the rest. See DEMO.md for the 8-minute script.
C       := docker compose -f docker-compose.yml -f docker-compose.override.yml
PLATFORM:= http://localhost:9000
TIMEOUT := 300

.DEFAULT_GOAL := help
.PHONY: help demo up down restart ps logs smoke faults fault-latency fault-error \
	fault-refuse fault-refuse-stop fault-restore fault-status load-baseline load-surge load-stop netmon-off \
	netmon-ebpf ebpf-build ebpf-sample ebpf-agents build clean nuke check
	netmon-off netmon-on netmon-sim-stop netmon-sim-start walkthrough

help: ## show this help
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ---------- lifecycle ----------
demo: ## build + start the whole stack, wait for every container to be healthy
	$(C) up -d --build --wait --wait-timeout $(TIMEOUT)
	@$(MAKE) --no-print-directory verify
	@$(MAKE) --no-print-directory walkthrough
	@echo
	@echo "  dashboard    http://localhost:3000"
	@echo "  platform-api http://localhost:9000/api/v1/health/all"
	@echo "  prometheus   http://localhost:9090"
	@echo
	@echo "Start it >=10 min before the demo so the graphs have history."

up: demo ## alias for demo
build: ## rebuild images
	$(C) build
ps: ## container status
	$(C) ps
down: ## stop and remove the stack (keeps the log volume)
	$(C) down
restart: ## restart everything
	$(C) restart
nuke: ## stop, remove containers AND the log volume (clean slate)
	$(C) down -v

logs: ## tail every service log
	$(C) logs -f --tail=40

verify: ## check that every backend answers on the contract endpoints
	@ok=0; for u in 9000/platform-api 9001/metrics-api 9002/logs-api 9003/topology-api; do \
	  p=$${u%%/*}; n=$${u##*/}; \
	  if curl -sf $$p/health >/dev/null; then echo "  ok   $$n ($$p)"; \
	  else echo "  FAIL $$n ($$p)"; ok=1; fi; done; \
	es=$$(curl -s -o /dev/null -w '%{http_code}' localhost:9200/_cluster/health); \
	if [ "$$es" = "200" ]; then echo "  ok   elasticsearch (9200)"; else echo "  FAIL elasticsearch ($$es)"; ok=1; fi; \
	exit $$ok

# The health endpoints only prove each process is listening. This walks one real
# request down the whole chain, which is the only check that catches a service
# that is up but broken -- which is exactly the state a half-restarted payments
# worker lands in.
walkthrough: ## send one real request through api-gateway -> orders -> payments -> inventory
	@body='{"customer_id":"c7","item_id":"item3","quantity":2,"amount":42.5}'; \
	code=$$(curl -s -o /tmp/minidd-walkthrough.json -w '%{http_code}' \
	  -X POST http://localhost:8000/orders \
	  -H 'content-type: application/json' \
	  -H "X-Request-ID: req_$$(openssl rand -hex 8)" -d "$$body"); \
	if [ "$$code" = "201" ] && grep -q '"status": *"confirmed"' /tmp/minidd-walkthrough.json; then \
	  echo "  ok   full chain returned 201 confirmed  ($$(cat /tmp/minidd-walkthrough.json))"; \
	else \
	  echo "  FAIL full chain returned $$code: $$(head -c 200 /tmp/minidd-walkthrough.json)"; \
	  echo "       if payments was recently restarted: docker compose exec payments /supervisor.sh logs"; \
	  exit 1; \
	fi
	@if $(MAKE) --no-print-directory fault-status >/dev/null 2>&1; then :; else \
	  echo "  FAIL payments listener is down: make -C demo fault-restore"; exit 1; fi

# ---------- the referee ----------
smoke: ## validate every Platform API response against the pydantic contract models
	@$(C) exec -T platform-api python scripts/smoke.py

check: ## contract tests for the Go modules + the demo stack smoke test
	cd ../linux/topology && go test ./...
	cd ../linux/ebpf && go test ./internal/raw/... ./internal/resolve/... \
		./internal/convert/... ./internal/sink/... ./internal/pretty/...
	$(MAKE) --no-print-directory smoke
	$(MAKE) --no-print-directory walkthrough

# ---------- faults ----------
fault-latency: ## inject 800 ms latency into payments for 60 s
	@curl -sf -X POST $(PLATFORM)/api/v1/demo/fault -H 'content-type: application/json' \
	  -d '{"service":"payments","fault":{"type":"latency","latency_ms":800,"duration_seconds":60}}' \
	  && echo "  payments: +800 ms for 60 s"

fault-error: ## inject a 50% error rate into payments for 60 s
	@curl -sf -X POST $(PLATFORM)/api/v1/demo/fault -H 'content-type: application/json' \
	  -d '{"service":"payments","fault":{"type":"error","error_rate":0.5,"duration_seconds":60}}' \
	  && echo "  payments: 50% errors for 60 s"

fault-refuse: ## connection refusal: close payments' listener -> real TCP RST (ECONNREFUSED)
	@$(C) exec -T payments sh /supervisor.sh stop
	@echo "  -> orders -> payments now logs ConnectError: Connection refused"
	@echo "  -> the Network tab edge orders->payments turns red with ECONNREFUSED"

fault-refuse-stop: ## alternative refusal: stop the whole payments container (DNS-level failure)
	$(C) stop payments
	@echo "  payments container stopped. NOTE: this fails at DNS, not at connect(),"
	@echo "  so the log says 'Name or service not known' and kernel-level eBPF sees"
	@echo "  no event. Use fault-refuse for the faithful demo."

fault-restore: ## clear all faults and bring payments' listener back
	@for s in api-gateway orders payments inventory; do \
	  curl -s -X DELETE http://localhost:$$(case $$s in api-gateway) echo 8000;; orders) echo 8001;; \
	    payments) echo 8002;; inventory) echo 8003;; esac)/admin/fault >/dev/null; done
	@$(C) exec -T payments sh /supervisor.sh start
	@echo "  all faults cleared, payments listening again"

fault-status: ## is the payments listener up?
	@$(C) exec -T payments sh /supervisor.sh status



faults: ## the full demo fault sequence, with a pause so the effect is visible
	@echo "1/3 latency fault on payments (+800 ms, 45 s)"; $(MAKE) --no-print-directory fault-latency
	@echo "   watch: dashboard Metrics -> payments latency_p95_ms"
	@sleep 45
	@echo "2/3 error burst on payments (50%, 45 s)"; $(MAKE) --no-print-directory fault-error
	@echo "   watch: dashboard Overview -> payments error rate, Logs -> severity ERROR"
	@sleep 45
	@echo "3/3 connection refusal (payments stopped)"; $(MAKE) --no-print-directory fault-refuse
	@echo "   watch: dashboard Network -> orders->payments turns red (ECONNREFUSED)"
	@sleep 30
	@$(MAKE) --no-print-directory fault-restore
	@echo "restored"

load-baseline: ## load generator to 5 rps
	@curl -sf -X POST http://localhost:8010/profile -H 'content-type: application/json' -d '{"profile":"baseline"}' && echo "  5 rps"
load-surge:   ## load generator to 20 rps
	@curl -sf -X POST http://localhost:8010/profile -H 'content-type: application/json' -d '{"profile":"surge"}' && echo "  20 rps"
load-stop:    ## stop generating traffic
	@curl -sf -X POST http://localhost:8010/profile -H 'content-type: application/json' -d '{"profile":"stop"}' && echo "  stopped"

# ---------- eBPF (needs root) ----------
# Only one network source at a time: the simulator and the real agent would both
# post events for the same traffic and the topology edge counts would double up.
netmon-off netmon-sim-stop: ## stop the simulator so the real eBPF agent owns the Network tab
	$(C) stop netmon-sim
	@echo "  netmon-sim stopped. Now run: make netmon-ebpf"

netmon-on netmon-sim-start: ## go back to the simulator (no root needed)
	$(C) up -d netmon-sim
	@echo "  netmon-sim up. Network events are SIMULATED again (say so if asked)."

ebpf-build: ## compile the eBPF program and build the agent (needs clang)
	cd ../linux/ebpf && go generate ./internal/loader/... && go build -o bin/agent ./cmd/agent

netmon-ebpf: ## run the REAL eBPF agent against the stack (asks for root via pkexec)
	@test -x ../linux/ebpf/bin/agent || $(MAKE) --no-print-directory ebpf-build
	$(MAKE) --no-print-directory netmon-off
	@echo "  a password dialog will appear: that is the OS asking to run the agent as root."
	pkexec ../linux/ebpf/bin/agent --pretty \
	  --topology-url http://127.0.0.1:9003 \
	  --registry $(CURDIR)/../darsan/contracts/registry.yaml

ebpf-sample: ## capture 30 s of REAL kernel events to linux/docs/samples/ (demo evidence)
	@test -x ../linux/ebpf/bin/agent || $(MAKE) --no-print-directory ebpf-build
	$(MAKE) --no-print-directory netmon-off
	@echo "  stop this with Ctrl-C once enough events have been captured."
	mkdir -p ../linux/docs/samples
	pkexec ../linux/ebpf/bin/agent --pretty --dry-run --stats-interval 0 \
	  --registry $(CURDIR)/../darsan/contracts/registry.yaml \
	  --out-file $(CURDIR)/../linux/docs/samples/events.sample.jsonl

ebpf-agents: ## list any agent still running (an orphan keeps posting events as root)
	@ps -eo pid,etimes,args | grep '[b]in/agent' || echo "  no agent running"
