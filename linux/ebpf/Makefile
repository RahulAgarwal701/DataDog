.PHONY: setup generate build build-sim test run run-dry sample demo-start demo-stop demo-slow demo-refuse demo-restore clean

# One-time: fetch dependencies and write go.sum (needs network)
setup:
	go get github.com/cilium/ebpf@latest golang.org/x/sys@latest
	go mod tidy

# Compile bpf/netmon.c -> internal/loader/netmon_bpfel.{go,o}. COMMIT the generated files so
# teammates and Docker builds do not need clang.
generate:
	go generate ./internal/loader/...

build: generate build-sim
	go build -o bin/agent ./cmd/agent

build-sim:
	go build -o bin/simservice ./cmd/simservice

# Pure-Go unit tests: no root and no kernel needed
test:
	go test ./internal/raw/... ./internal/resolve/... ./internal/convert/... ./internal/sink/... ./internal/pretty/...

# Live demo mode: readable events on the terminal + sent to the Topology API
run: build
	sudo ./bin/agent --pretty

# Same, without sending anything to the Topology API
run-dry: build
	sudo ./bin/agent --pretty --dry-run

# Capture real evidence for the review (stop with Ctrl-C after ~30 s while traffic is running)
sample: build
	mkdir -p ../docs/samples
	sudo ./bin/agent --pretty --dry-run --out-file ../docs/samples/events.sample.jsonl

demo-start: build-sim
	bash ./scripts/demo.sh start
demo-stop:
	bash ./scripts/demo.sh stop
demo-slow:
	bash ./scripts/demo.sh slow
demo-refuse:
	bash ./scripts/demo.sh refuse
demo-restore:
	bash ./scripts/demo.sh restore

clean:
	rm -rf bin out
