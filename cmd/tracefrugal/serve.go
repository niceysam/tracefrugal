package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/niceysam/tracefrugal/internal/experiment"
	"github.com/niceysam/tracefrugal/internal/ledger"
)

// dashboard rereads the files on each refresh. A partially appended JSONL line
// fails validation rather than showing a plausible but incomplete cost total.
func dashboard(trace, prices string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Refresh", "3")
		pf, err := os.Open(prices)
		if err != nil {
			http.Error(w, "Price book unavailable. Check the configured file.", 503)
			return
		}
		p, err := ledger.ReadPrices(pf)
		pf.Close()
		if err != nil {
			http.Error(w, "Invalid price book. Validate it with tracefrugal report.", 422)
			return
		}
		tf, err := os.Open(trace)
		if err != nil {
			http.Error(w, "Waiting for usage. Connect your recorder to the configured trace file. Refreshes every 3 seconds.", 503)
			return
		}
		report, err := ledger.Analyze(tf, p)
		tf.Close()
		if err != nil {
			http.Error(w, "Trace is incomplete or invalid, or a model has no price. Retrying in 3 seconds. Run tracefrugal report for details.", 422)
			return
		}
		var body bytes.Buffer
		if err := ledger.WriteReportHTML(&body, report); err != nil {
			http.Error(w, "Could not render report", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(body.Bytes())
	})
}

func historyDashboard(state string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "Method not allowed", 405)
			return
		}
		if r.URL.Path == "/" {
			var b bytes.Buffer
			if err := experiment.WriteHistory(&b, state); err != nil {
				http.Error(w, "Cannot read history; inspect the local state directory.", 503)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Refresh", "3")
			w.Write(b.Bytes())
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/runs/")
		if strings.HasPrefix(r.URL.Path, "/runs/") && experiment.ValidID(id) {
			b, err := os.ReadFile(filepath.Join(state, "runs", id, "report.html"))
			if err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(b)
				return
			}
		}
		http.NotFound(w, r)
	})
}

func serve(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	trace := fs.String("trace", "", "JSONL file written by your recorder")
	prices := fs.String("prices", "", "explicit price book")
	state := fs.String("state", "", "experiment history directory (instead of trace/prices)")
	port := fs.Int("port", 8765, "local dashboard port")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	usageMode := *state == "" && *trace != "" && *prices != "" && *trace != "-" && *prices != "-"
	historyMode := *state != "" && *trace == "" && *prices == ""
	if fs.NArg() != 0 || (!usageMode && !historyMode) || *port < 1 || *port > 65535 {
		fmt.Fprintln(stderr, "serve requires either --trace and --prices, or --state, and a port from 1 to 65535")
		return 2
	}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		fmt.Fprintln(stderr, "dashboard:", err)
		return 2
	}
	fmt.Fprintf(stderr, "Dashboard: http://%s/ (refreshes every 3 seconds; Ctrl-C to stop)\n", addr)
	handler := dashboard(*trace, *prices)
	if historyMode {
		handler = historyDashboard(*state)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(stderr, err)
		return 2
	}
	return 0
}
