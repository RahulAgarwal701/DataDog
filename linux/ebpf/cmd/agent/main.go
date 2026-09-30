// Command agent is the eBPF network observer. It watches TCP connections on the host,
// resolves them to service-to-service events and ships them to the Topology API.
//
//	sudo ./bin/agent --pretty
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/minidd/ebpf-agent/internal/convert"
	"github.com/minidd/ebpf-agent/internal/loader"
	"github.com/minidd/ebpf-agent/internal/pretty"
	"github.com/minidd/ebpf-agent/internal/resolve"
	"github.com/minidd/ebpf-agent/internal/sink"
	"github.com/minidd/topology/pkg/registry"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type stats struct {
	seen, emitted, dropPort, dropSrc, dropKind atomic.Uint64
}

func main() {
	var (
		topologyURL = flag.String("topology-url", envOr("TOPOLOGY_URL", "http://localhost:9003"), "Topology API base URL")
		registryP   = flag.String("registry", envOr("REGISTRY_PATH", "../contracts/registry.yaml"), "path to contracts/registry.yaml")
		host        = flag.String("host", "", "host name put in every event (default: os.Hostname())")
		outPath     = flag.String("out-file", envOr("EVENTS_OUT_FILE", "./out/events.jsonl"), "also append every event as JSON Lines here (empty = off)")
		prettyOut   = flag.Bool("pretty", false, "print a readable line per event on stdout")
		jsonOut     = flag.Bool("json", false, "print every event as JSON on stdout")
		noColor     = flag.Bool("no-color", false, "disable ANSI colours")
		dryRun      = flag.Bool("dry-run", false, "do not send anything to the Topology API")
		emitUnres   = flag.Bool("emit-unresolved", false, "also emit events whose source/destination cannot be mapped to a service (debug)")
		flushEvery  = flag.Duration("flush-interval", time.Second, "how often batches are sent")
		batchSize   = flag.Int("batch-size", 200, "max events per POST (API limit 500)")
		maxQueue    = flag.Int("max-queue", 10000, "max events buffered while the API is unreachable")
		statsEvery  = flag.Duration("stats-interval", 15*time.Second, "print counters this often (0 = never)")
		procRoot    = flag.String("proc-root", "/proc", "procfs mount (use /host/proc when running in a container with pid: host)")
	)
	flag.Parse()
	log.SetFlags(log.Ltime)

	if os.Geteuid() != 0 {
		log.Println("warning: not running as root; loading eBPF and reading /proc/<pid>/environ will probably fail (use sudo)")
	}
	hostName := *host
	if hostName == "" {
		h, err := os.Hostname()
		if err != nil {
			h = "unknown-host"
		}
		hostName = h
	}

	reg, err := registry.Load(*registryP)
	if err != nil {
		log.Fatalf("service registry: %v", err)
	}
	base, err := loader.MonoBase()
	if err != nil {
		log.Fatal(err)
	}
	conv := &convert.Converter{
		Host:           hostName,
		Res:            resolve.New(reg, *procRoot),
		MonoBase:       base,
		EmitUnresolved: *emitUnres,
	}

	var (
		outEnc  *json.Encoder
		outFile *os.File
	)
	if *outPath != "" {
		if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
			log.Fatalf("create output dir: %v", err)
		}
		outFile, err = os.OpenFile(*outPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			log.Fatalf("open %s: %v", *outPath, err)
		}
		defer outFile.Close()
		outEnc = json.NewEncoder(outFile)
	}
	stdoutEnc := json.NewEncoder(os.Stdout)
	var printer *pretty.Printer
	if *prettyOut {
		fi, statErr := os.Stdout.Stat()
		isTTY := statErr == nil && fi.Mode()&os.ModeCharDevice != 0
		printer = pretty.New(os.Stdout, isTTY && !*noColor)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var sender *sink.Sender
	senderDone := make(chan struct{})
	if *dryRun {
		close(senderDone)
	} else {
		sender = sink.New(*topologyURL, *batchSize, *flushEvery, *maxQueue, log.Printf)
		go func() {
			sender.Run(ctx)
			close(senderDone)
		}()
	}

	l, err := loader.Load()
	if err != nil {
		log.Fatalf("%v\n(hint: see docs/ebpf-setup.md; did you run `make generate`? are you root?)", err)
	}
	defer l.Close()
	go func() {
		<-ctx.Done()
		l.Close() // unblocks Read
	}()

	var st stats
	if *statsEvery > 0 {
		go func() {
			t := time.NewTicker(*statsEvery)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					printStats(&st, sender)
				}
			}
		}()
	}

	target := *topologyURL
	if *dryRun {
		target = "(dry run)"
	}
	log.Printf("agent started: host=%s registry=%s topology=%s out=%q", hostName, *registryP, target, *outPath)

	for {
		r, err := l.Read()
		if err != nil {
			if loader.IsClosed(err) {
				break
			}
			log.Printf("read error: %v", err)
			continue
		}
		st.seen.Add(1)
		ev, drop := conv.Convert(r)
		switch drop {
		case convert.Keep:
		case convert.DropUnregisteredPort:
			st.dropPort.Add(1)
			continue
		case convert.DropUnresolvedSource:
			st.dropSrc.Add(1)
			continue
		default:
			st.dropKind.Add(1)
			continue
		}
		st.emitted.Add(1)

		if printer != nil {
			printer.Print(ev)
		}
		if *jsonOut {
			_ = stdoutEnc.Encode(ev)
		}
		if outEnc != nil {
			if err := outEnc.Encode(ev); err != nil {
				log.Printf("write %s: %v", *outPath, err)
			}
		}
		if sender != nil {
			sender.Enqueue(ev)
		}
	}

	<-senderDone
	printStats(&st, sender)
	log.Println("agent stopped")
}

func printStats(st *stats, sender *sink.Sender) {
	msg := fmt.Sprintf("stats: seen=%d emitted=%d dropped(unregistered_port=%d unresolved_src=%d unknown_kind=%d)",
		st.seen.Load(), st.emitted.Load(), st.dropPort.Load(), st.dropSrc.Load(), st.dropKind.Load())
	if sender != nil {
		msg += fmt.Sprintf(" sent=%d send_failures=%d queue_dropped=%d pending=%d",
			sender.Sent.Load(), sender.Failures.Load(), sender.Dropped.Load(), sender.Pending())
	}
	log.Print(msg)
}
