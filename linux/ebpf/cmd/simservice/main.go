// Command simservice is a tiny stand-in for the dummy microservices so the eBPF agent can be
// developed and demoed before Dev's real services exist. Launch each instance with the
// SERVICE_NAME environment variable set (the agent identifies processes by it):
//
//	SERVICE_NAME=payments ./bin/simservice -listen :8002 -downstream http://127.0.0.1:8003 -delay 50ms
//
// Outgoing calls disable HTTP keep-alive, so every call opens a new TCP connection
// (exactly what the real dummy services do, see contract section 7.1).
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	listen := flag.String("listen", "", "address to listen on, e.g. :8001 (empty = client only)")
	downstream := flag.String("downstream", "", "base URL of the service to call for every request")
	delay := flag.Duration("delay", 0, "artificial latency added to every request")
	rps := flag.Float64("rps", 0, "if > 0, generate this many requests per second to -downstream")
	flag.Parse()

	name := os.Getenv("SERVICE_NAME")
	if name == "" {
		log.Fatal("SERVICE_NAME must be set (the eBPF agent identifies processes by it)")
	}
	log.SetPrefix(fmt.Sprintf("[%s] ", name))

	client := &http.Client{
		Timeout:   3 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	call := func() (int, error) {
		resp, err := client.Get(*downstream + "/")
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, nil
	}

	if *rps > 0 && *downstream != "" {
		go func() {
			t := time.NewTicker(time.Duration(float64(time.Second) / *rps))
			defer t.Stop()
			for range t.C {
				go func() {
					if _, err := call(); err != nil {
						log.Printf("call failed: %v", err)
					}
				}()
			}
		}()
	}

	if *listen == "" {
		select {} // client-only mode: run forever
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(*delay)
		if *downstream != "" {
			status, err := call()
			if err != nil || status >= 500 {
				http.Error(w, "downstream failed", http.StatusBadGateway)
				return
			}
		}
		fmt.Fprintln(w, "ok")
	})
	log.Printf("listening on %s, downstream=%q, delay=%s", *listen, *downstream, *delay)
	log.Fatal(http.ListenAndServe(*listen, mux))
}
