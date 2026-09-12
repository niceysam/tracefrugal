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

	"github.com/niceysam/tracefrugal/internal/native"
	"github.com/niceysam/tracefrugal/internal/pack"
	"github.com/niceysam/tracefrugal/internal/webui"
)

type dirs []string

func (d *dirs) String() string     { return fmt.Sprint([]string(*d)) }
func (d *dirs) Set(s string) error { *d = append(*d, s); return nil }

type nativeApp struct {
	mu       sync.Mutex
	reader   native.Reader
	sources  []native.Source
	apps     map[string]*claudeApp
	token    string
	days     int
	packRoot string
}

func (a *nativeApp) snapshot(days int, source string) (native.Report, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	r, err := a.reader.Read(a.sources, source, now.Add(-time.Duration(days)*24*time.Hour), now)
	if err == nil && a.packRoot != "" {
		p, e := pack.ReadReport(a.packRoot, now)
		if e != nil {
			return r, e
		}
		r.Packing = &p
	}
	if err == nil && source != "" {
		if app := a.apps[source]; app != nil {
			report, e := app.snapshot(days)
			if e != nil {
				return r, e
			}
			r.Changes, r.Recipes = report.Changes, report.Recipes
		}
	}
	return r, err
}
func (a *nativeApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", dashboardCSP)
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil || host != "127.0.0.1" && host != "localhost" {
		http.Error(w, "Local host required", 403)
		return
	}
	if webui.Asset(w, r) {
		return
	}
	source := r.URL.Query().Get("source")
	if r.Method == http.MethodPost {
		if r.URL.Path == "/api/pack-stop" || r.URL.Path == "/api/pack-rate" {
			r.Body = http.MaxBytesReader(w, r.Body, 4096)
			if r.Header.Get("Origin") != "http://"+r.Host || r.ParseForm() != nil || subtle.ConstantTimeCompare([]byte(r.PostForm.Get("token")), []byte(a.token)) != 1 {
				http.Error(w, "Reload before changing a trial.", 403)
				return
			}
			a.mu.Lock()
			if a.packRoot == "" {
				err = fmt.Errorf("no packing trial connected")
			} else if r.URL.Path == "/api/pack-stop" {
				err = pack.Stop(a.packRoot)
			} else {
				rating, _ := strconv.Atoi(r.PostForm.Get("rating"))
				err = pack.Rate(a.packRoot, r.PostForm.Get("before") == "true", rating)
			}
			a.mu.Unlock()
			if err != nil {
				http.Error(w, "Trial update refused. Check rating, baseline timing and local state.", 409)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if app := a.apps[source]; app != nil {
			app.ServeHTTP(w, r)
			return
		}
		http.Error(w, "Select one Claude source for an advisory rule trial. Codex and all-source views are read-only.", 409)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", 405)
		return
	}
	switch r.URL.Path {
	case "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		webui.WriteClaude(w, webui.Options{Mode: "native", Token: a.token})
	case "/api/state":
		days := a.days
		if value := r.URL.Query().Get("days"); value != "" {
			days, err = strconv.Atoi(value)
			if err != nil || days != 1 && days != 7 && days != 30 {
				http.Error(w, "Choose 1, 7, or 30 days.", 400)
				return
			}
		}
		report, err := a.snapshot(days, source)
		if err != nil {
			http.Error(w, "Cannot read the selected source or trial history. Reload and check local permissions.", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(report)
	default:
		http.NotFound(w, r)
	}
}

func nativeCommand(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var claudeDirs, codexDirs dirs
	fs.Var(&claudeDirs, "claude-dir", "Claude store (repeatable); explicit roots replace automatic discovery")
	fs.Var(&codexDirs, "codex-dir", "Codex store (repeatable); explicit roots replace automatic discovery")
	days := fs.Int("days", 7, "rolling period: 1, 7, or 30 days")
	port := fs.Int("port", 8765, "loopback port; 0 chooses a free port")
	noOpen := fs.Bool("no-open", false, "print URL without opening a browser")
	export := fs.Bool("json", false, "export usage metadata and exit, without writing state")
	stateDir := fs.String("state", "", "private trial history directory")
	packRoot := fs.String("pack-state", "", "optional MCP packing trial directory for hourly receipts, ratings and undo")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 || *days != 1 && *days != 7 && *days != 30 || *port < 0 || *port > 65535 {
		fmt.Fprintln(stderr, "Invalid time range, port or arguments.")
		return 2
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(stderr, "Cannot find the home directory.")
		return 2
	}
	explicit := []native.Source{}
	for _, p := range claudeDirs {
		explicit = append(explicit, native.Source{Harness: "claude", Root: p})
	}
	for _, p := range codexDirs {
		explicit = append(explicit, native.Source{Harness: "codex", Root: p})
	}
	sources, err := native.Discover(home, os.Getenv("CLAUDE_CONFIG_DIR"), os.Getenv("CODEX_HOME"), explicit)
	if err != nil {
		fmt.Fprintln(stderr, "Cannot discover local stores:", err)
		return 2
	}
	a := &nativeApp{sources: sources, days: *days, apps: map[string]*claudeApp{}, packRoot: *packRoot}
	if *export {
		report, err := a.snapshot(*days, "")
		if err != nil || json.NewEncoder(stdout).Encode(report) != nil {
			fmt.Fprintln(stderr, "Cannot export local usage.")
			return 2
		}
		return 0
	}
	if *stateDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			fmt.Fprintln(stderr, "Supply --state for trial history.")
			return 2
		}
		*stateDir = filepath.Join(dir, "tracefrugal")
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		fmt.Fprintln(stderr, "Cannot initialize local controls.")
		return 2
	}
	a.token = hex.EncodeToString(random)
	for _, s := range sources {
		if s.Harness == "claude" {
			// Keep the v0.5 default journal path so upgrading does not hide
			// an active rule or lose its restore history.
			sum := sha256.Sum256([]byte(s.Root))
			journalID := hex.EncodeToString(sum[:])[:16]
			a.apps[s.ID] = &claudeApp{root: s.Root, state: filepath.Join(*stateDir, journalID), token: a.token, days: *days}
		}
	}
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil && *port == 8765 {
		listener, err = net.Listen("tcp4", "127.0.0.1:0")
	}
	if err != nil {
		fmt.Fprintln(stderr, "Cannot open local port. Try --port 0.")
		return 2
	}
	defer listener.Close()
	url := "http://" + listener.Addr().String() + "/"
	fmt.Fprintf(stdout, "TraceFrugal — Claude Code + Codex\nDashboard: %s\n", url)
	for _, s := range sources {
		fmt.Fprintf(stdout, "Source: %s · %s\n", s.Harness, s.Root)
	}
	fmt.Fprintln(stdout, "Local processing. No model calls. Ctrl-C to stop.")
	if !*noOpen {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", url)
		case "windows":
			cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			cmd = exec.Command("xdg-open", url)
		}
		if err := cmd.Start(); err == nil {
			go cmd.Wait()
		} else {
			fmt.Fprintln(stderr, "Open the printed dashboard URL.")
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
				for _, app := range a.apps {
					if _, err := app.snapshot(*days); err != nil {
						fmt.Fprintln(stderr, "A Claude trial observation could not update.")
					}
				}
			case <-done:
				return
			}
		}
	}()
	server := &http.Server{Handler: a, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 30 * time.Second}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(stderr, "Dashboard stopped:", err)
		return 2
	}
	return 0
}
