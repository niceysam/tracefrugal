package main

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
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
	"github.com/niceysam/tracefrugal/internal/webui"
)

const dashboardCSP = "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self'; connect-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'none'"

// dashboard rereads the files on each poll. A partially appended JSONL line
// fails validation rather than showing a plausible but incomplete cost total.
func dashboard(trace, prices string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", dashboardCSP)
		if webui.Asset(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			webui.Write(w, webui.Options{Mode: "usage"})
			return
		}
		if r.URL.Path != "/api/state" {
			http.NotFound(w, r)
			return
		}
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
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(struct {
			Report  ledger.Report      `json:"report"`
			Entries []experiment.Entry `json:"entries"`
		}{report, []experiment.Entry{}})
	})
}

func historyDashboard(state string) http.Handler {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "Cannot initialize local controls", 500) })
	}
	token := hex.EncodeToString(random)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", dashboardCSP)
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil || (host != "127.0.0.1" && host != "localhost") {
			http.Error(w, "Local host required", 403)
			return
		}
		if webui.Asset(w, r) {
			return
		}
		if r.Method == http.MethodPost && (r.URL.Path == "/rollback" || r.URL.Path == "/resume") {
			r.Body = http.MaxBytesReader(w, r.Body, 8192)
			if r.Header.Get("Origin") != "http://"+r.Host || r.ParseForm() != nil || subtle.ConstantTimeCompare([]byte(r.PostForm.Get("token")), []byte(token)) != 1 {
				http.Error(w, "Invalid local form. Reload the dashboard and retry.", 403)
				return
			}
			if r.URL.Path == "/rollback" {
				_, err = experiment.Rollback(state, r.PostForm.Get("id"))
			} else {
				_, err = experiment.Resume(state)
			}
			if err != nil {
				http.Error(w, "Action refused: state changed, is locked, or snapshot is invalid. Reload and inspect the local journal.", 409)
				return
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			http.Error(w, "Method not allowed", 405)
			return
		}
		if r.URL.Path == "/" {
			var b bytes.Buffer
			if err := webui.Write(&b, webui.Options{Mode: "history", Token: token}); err != nil {
				http.Error(w, "Cannot render dashboard.", 500)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(b.Bytes())
			return
		}
		if r.URL.Path == "/api/state" {
			snapshot, err := experiment.Snapshot(state)
			if err != nil {
				http.Error(w, "Cannot read history; inspect the local state directory.", 503)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(snapshot)
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
