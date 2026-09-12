package main

import (
	"context"
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
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/niceysam/tracefrugal/internal/claude"
	"github.com/niceysam/tracefrugal/internal/optimize"
	"github.com/niceysam/tracefrugal/internal/webui"
)

type optimizerApp struct {
	mu     sync.Mutex
	config optimize.Config
	reader claude.Reader
	doctor optimize.Diagnosis
	token  string
}

func (a *optimizerApp) diagnose() {
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	docs, err := optimize.OfficialDocuments(ctx)
	a.doctor = optimize.Diagnose(a.config, docs, err, time.Now())
}

func (a *optimizerApp) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	if r.Method == http.MethodGet && r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		webui.WriteOptimizer(w, webui.Options{Mode: "optimizer", Token: a.token})
		return
	}
	if r.URL.Path != "/api/state" && r.URL.Path != "/api/diagnose" && r.URL.Path != "/api/apply" && r.URL.Path != "/api/restore" && r.URL.Path != "/api/rate" {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path != "/api/state" {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if r.Header.Get("Origin") != "http://"+r.Host || r.ParseForm() != nil ||
			subtle.ConstantTimeCompare([]byte(r.PostForm.Get("token")), []byte(a.token)) != 1 {
			http.Error(w, "Reload this local page before changing settings.", 403)
			return
		}
	} else if r.Method != http.MethodGet {
		http.Error(w, "GET required", 405)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	switch r.URL.Path {
	case "/api/diagnose":
		a.diagnose()
	case "/api/apply":
		rating, _ := strconv.Atoi(r.PostForm.Get("rating"))
		rows, _, e := optimize.ReadProject(a.config, &a.reader, now)
		if e != nil {
			err = e
		} else {
			_, err = optimize.Apply(a.config, a.doctor, r.PostForm.Get("plan"), rating, rows, now)
		}
	case "/api/restore":
		// Freeze all currently available observations before undo.
		// A broken or unavailable log store must never prevent restoring settings.
		_, _ = optimize.Snapshot(a.config, a.doctor, &a.reader, now)
		err = optimize.Restore(a.config, r.PostForm.Get("id"), now)
		if err == nil {
			a.doctor = optimize.Diagnose(a.config, a.doctor.Documents, nil, now)
		}
	case "/api/rate":
		rating, _ := strconv.Atoi(r.PostForm.Get("rating"))
		err = optimize.Rate(a.config, r.PostForm.Get("id"), rating, r.PostForm.Get("outcome"), now)
	}
	if err != nil {
		// Errors describe validation, not settings contents.
		http.Error(w, err.Error(), 409)
		return
	}
	report, err := optimize.Snapshot(a.config, a.doctor, &a.reader, now)
	if err != nil {
		http.Error(w, "Cannot read optimizer history or logs. Check the local state directory.", 503)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func optimizeCommand(command string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	project := fs.String("project", ".", "project whose local Claude settings may be optimized")
	binary := fs.String("claude-bin", "", "Claude executable; defaults to PATH")
	store := fs.String("claude-dir", "", "Claude log/settings store; defaults to CLAUDE_CONFIG_DIR or ~/.claude")
	state := fs.String("state", "", "private backup and observation directory")
	port := fs.Int("port", 0, "loopback port; 0 chooses a free port")
	noOpen := fs.Bool("no-open", false, "print the local URL without opening a browser")
	id := fs.String("id", "", "trial ID (internal hook)")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if command == "optimizer-receipt" {
		if *state == "" || optimize.Record(*state, *id, stdin, os.Getenv(optimize.Setting), time.Now()) != nil {
			// Hook failure is non-blocking and does not inject text into context.
			return 1
		}
		return 0
	}
	if fs.NArg() != 0 || *port < 0 || *port > 65535 {
		fmt.Fprintln(stderr, "Invalid optimizer arguments.")
		return 2
	}
	home, _ := os.UserHomeDir()
	if *store == "" {
		*store = os.Getenv("CLAUDE_CONFIG_DIR")
		if *store == "" {
			*store = filepath.Join(home, ".claude")
		}
	}
	projectAbs, _ := filepath.Abs(*project)
	if canonical, err := filepath.EvalSymlinks(projectAbs); err == nil {
		projectAbs = canonical
	}
	if *state == "" {
		dir, _ := os.UserConfigDir()
		*state = filepath.Join(dir, "tracefrugal", "optimizer", optimize.Hash(projectAbs)[:16])
	}
	self, _ := os.Executable()
	c, err := optimize.Normalize(optimize.Config{Project: projectAbs, Store: *store, Binary: *binary, State: *state, Self: self})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	a := &optimizerApp{config: c}
	fmt.Fprintln(stderr, "Checking Claude version and fixed official documentation URLs. No model calls.")
	a.diagnose()
	if command == "doctor" {
		json.NewEncoder(stdout).Encode(a.doctor)
		if !a.doctor.Ready {
			return 1
		}
		return 0
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return 2
	}
	a.token = hex.EncodeToString(random)
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintln(stderr, "Cannot open local port.")
		return 2
	}
	defer listener.Close()
	url := "http://" + listener.Addr().String() + "/"
	fmt.Fprintf(stdout, "TraceFrugal local optimizer\nClaude: %s (%s)\nProject: %s\nState: %s\nOpen: %s\n", c.Binary, a.doctor.Version, c.Project, c.State, url)
	if !*noOpen {
		var opener *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			opener = exec.Command("open", url)
		case "windows":
			opener = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
		default:
			opener = exec.Command("xdg-open", url)
		}
		if opener.Start() == nil {
			go opener.Wait()
		}
	}
	server := &http.Server{Handler: a, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 30 * time.Second}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(stderr, err)
		return 2
	}
	return 0
}
