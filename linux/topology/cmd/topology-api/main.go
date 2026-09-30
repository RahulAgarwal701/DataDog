// Command topology-api is the Topology API (port 9003): it ingests NetworkEvents from the
// eBPF agent and serves network events plus the derived service dependency graph.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/minidd/topology/internal/api"
	"github.com/minidd/topology/internal/mock"
	"github.com/minidd/topology/internal/store"
	"github.com/minidd/topology/pkg/contract"
	"github.com/minidd/topology/pkg/registry"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	healthcheck := hasFlag("healthcheck")
	port := env("PORT", "9003")

	// `topology-api -healthcheck` lets a distroless container probe itself (no curl available).
	if healthcheck {
		c := http.Client{Timeout: 2 * time.Second}
		resp, err := c.Get("http://127.0.0.1:" + port + "/health")
		if err != nil || resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		os.Exit(0)
	}

	level := slog.LevelInfo
	if env("LOG_LEVEL", "info") == "debug" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	regPath := env("REGISTRY_PATH", "../contracts/registry.yaml")
	reg, err := registry.Load(regPath)
	if err != nil {
		log.Error("cannot load service registry", "path", regPath, "error", err.Error())
		os.Exit(1)
	}
	maxEvents, err := strconv.Atoi(env("MAX_EVENTS", "100000"))
	if err != nil || maxEvents < 1 {
		log.Error("MAX_EVENTS must be a positive integer")
		os.Exit(1)
	}
	st := store.New(maxEvents, reg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if env("MOCK", "false") == "true" {
		mock.Seed(st, reg, time.Now(), 5*time.Minute)
		go mock.Run(ctx, st, reg)
		log.Info("MOCK mode enabled: serving synthetic network events")
	}

	srv := &api.Server{Store: st, Log: log, Now: time.Now, MaxBatch: 500, Version: contract.Version}
	httpSrv := &http.Server{
		Addr:              ":" + port,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- httpSrv.ListenAndServe() }()
	log.Info("topology-api listening", "port", port, "registry", regPath, "max_events", maxEvents)

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err.Error())
			os.Exit(1)
		}
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
}

// hasFlag reports whether -name / --name was passed (avoids pulling in the flag package for one switch).
func hasFlag(name string) bool {
	for _, a := range os.Args[1:] {
		if a == "-"+name || a == "--"+name {
			return true
		}
	}
	return false
}
