package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/niceysam/tracefrugal/internal/claude"
	"github.com/niceysam/tracefrugal/internal/webui"
)

type claudeApp struct {
	mu          sync.Mutex
	reader      claude.Reader
	root, state string
	token       string
	days        int
}

func (a *claudeApp) snapshot(days int) (claude.Report, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	requests, health, err := a.reader.Read(a.root)
	if err != nil {
		return claude.Report{}, err
	}
	report := claude.Build(requests, health, now.Add(-time.Duration(days)*24*time.Hour), now)
	events := a.reader.Evidence()
	report.Diagnostics = claude.Diagnose(events, report.Summary, report.From, report.Until)
	report.Changes, err = claude.Observe(a.root, a.state, requests, events, now)
	if err != nil {
		return report, err
	}
	// Bound response size without changing any aggregate.
	for i := range report.Sessions {
		if len(report.Sessions[i].Timeline) > 200 {
			report.Sessions[i].Timeline = report.Sessions[i].Timeline[len(report.Sessions[i].Timeline)-200:]
		}
	}
	return report, nil
}

func (a *claudeApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	if r.Method == http.MethodPost && (r.URL.Path == "/api/try" || r.URL.Path == "/api/restore" || r.URL.Path == "/api/rate") {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if r.Header.Get("Origin") != "http://"+r.Host || r.ParseForm() != nil || subtle.ConstantTimeCompare([]byte(r.PostForm.Get("token")), []byte(a.token)) != 1 {
			http.Error(w, "Reload the dashboard before changing a trial.", 403)
			return
		}
		a.mu.Lock()
		requests, _, readErr := a.reader.Read(a.root)
		err = readErr
		now := time.Now()
		if err == nil {
			events := a.reader.Evidence()
			rating, _ := strconv.Atoi(r.PostForm.Get("rating"))
			if r.URL.Path == "/api/try" {
				err = claude.Apply(a.root, a.state, r.PostForm.Get("recipe"), rating, requests, events, now)
			} else {
				_, err = claude.Observe(a.root, a.state, requests, events, now)
				if err == nil && r.URL.Path == "/api/restore" {
					err = claude.Restore(a.root, a.state, r.PostForm.Get("id"), now)
				} else if err == nil {
					err = claude.Rate(a.root, a.state, r.PostForm.Get("id"), rating)
				}
			}
		}
		a.mu.Unlock()
		if err != nil {
			// Errors are not allowed to echo settings content or raw paths.
			http.Error(w, "Trial update refused. Check the 1–5 rating and recorded usage. An existing, edited, linked, or read-only trial rule is preserved. Refresh and inspect the local state directory before retrying.", 409)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		webui.WriteClaude(w, webui.Options{Mode: "claude", Token: a.token})
		return
	}
	if r.URL.Path == "/api/state" {
		days := a.days
		if value := r.URL.Query().Get("days"); value != "" {
			days, err = strconv.Atoi(value)
			if err != nil || (days != 1 && days != 7 && days != 30) {
				http.Error(w, "Choose 1, 7, or 30 days.", 400)
				return
			}
		}
		report, err := a.snapshot(days)
		if err != nil {
			http.Error(w, "Cannot read local history or its state journal. Check terminal paths and permissions. Last displayed data may be stale.", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(report)
		return
	}
	http.NotFound(w, r)
}

func claudeCommand(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("claude", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("dir", "", "Claude config directory; defaults to CLAUDE_CONFIG_DIR or ~/.claude")
	state := fs.String("state", "", "private change-history directory; defaults to the OS user config directory")
	noOpen := fs.Bool("no-open", false, "print URL without opening a browser")
	days := fs.Int("days", 7, "rolling period: 1, 7, or 30 days")
	port := fs.Int("port", 8765, "local port; 0 chooses a free port")
	export := fs.Bool("json", false, "export usage metadata and exit; no settings or state files are written")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 || (*days != 1 && *days != 7 && *days != 30) || *port < 0 || *port > 65535 {
		fmt.Fprintln(stderr, "Use --days 1, 7, or 30 and --port 0–65535.")
		return 2
	}
	if *root == "" {
		*root = os.Getenv("CLAUDE_CONFIG_DIR")
	}
	if *root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(stderr, "Cannot find your home directory. Supply --dir.")
			return 2
		}
		*root = filepath.Join(home, ".claude")
	}
	absolute, err := filepath.Abs(*root)
	if err != nil {
		fmt.Fprintln(stderr, "Invalid Claude config directory.")
		return 2
	}
	*root = absolute
	if resolved, err := filepath.EvalSymlinks(*root); err == nil {
		*root = resolved
	}
	a := &claudeApp{root: *root, days: *days}
	if *export {
		requests, health, err := a.reader.Read(*root)
		if err != nil {
			fmt.Fprintln(stderr, "Cannot read Claude usage.")
			return 2
		}
		now := time.Now()
		report := claude.Build(requests, health, now.Add(-time.Duration(*days)*24*time.Hour), now)
		report.Diagnostics = claude.Diagnose(a.reader.Evidence(), report.Summary, report.From, report.Until)
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			return 2
		}
		return 0
	}
	if *state == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			fmt.Fprintln(stderr, "Cannot find a state directory. Supply --state.")
			return 2
		}
		sum := sha256.Sum256([]byte(*root))
		*state = filepath.Join(dir, "tracefrugal", hex.EncodeToString(sum[:])[:16])
	}
	a.state = *state
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		fmt.Fprintln(stderr, "Cannot initialize local controls.")
		return 2
	}
	a.token = hex.EncodeToString(random)
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil && *port == 8765 {
		listener, err = net.Listen("tcp4", "127.0.0.1:0")
	}
	if err != nil {
		fmt.Fprintln(stderr, "Cannot open that local port. Try --port 0.")
		return 2
	}
	defer listener.Close()
	url := "http://" + listener.Addr().String() + "/"
	fmt.Fprintf(stdout, "TraceFrugal — your Claude Code usage\nDashboard: %s\nReading: %s\nPrivate history: %s\nNo API key needed. Ctrl-C to stop.\n", url, *root, *state)
	if !*noOpen {
		var command *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			command = exec.Command("open", url)
		case "windows":
			command = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			command = exec.Command("xdg-open", url)
		}
		if err := command.Start(); err != nil {
			fmt.Fprintln(stderr, "Open the dashboard URL above in your browser.")
		} else {
			go command.Wait()
		}
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if _, err := a.snapshot(*days); err != nil {
					fmt.Fprintln(stderr, "Hourly observation could not update. Check state permissions.")
				}
			case <-done:
				return
			}
		}
	}()
	server := &http.Server{Handler: a, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(stderr, "Dashboard stopped:", err)
		return 2
	}
	return 0
}
