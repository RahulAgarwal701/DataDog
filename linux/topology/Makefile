.PHONY: setup test build run run-mock

setup:
	go mod tidy

test:
	go test ./...

build:
	CGO_ENABLED=0 go build -o bin/topology-api ./cmd/topology-api

run:
	go run ./cmd/topology-api

# Serves synthetic live events: no Linux/eBPF host required
run-mock:
	MOCK=true go run ./cmd/topology-api
