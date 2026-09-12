package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/niceysam/tracefrugal/internal/native"
	"github.com/niceysam/tracefrugal/internal/pack"
)

func TestNativeSourceScopeAndPackControls(t *testing.T) {
	root := t.TempDir()
	source, _ := native.Discover(root, "", "", []native.Source{{Harness: "claude", Root: filepath.Join(root, "claude")}, {Harness: "codex", Root: filepath.Join(root, "codex")}})
	logDir := filepath.Join(source[0].Root, "projects")
	os.MkdirAll(logDir, 0700)
	body := `{"type":"assistant","timestamp":"` + time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano) + `","sessionId":"s","message":{"id":"r","model":"claude-sonnet-4-6","content":"PRIVATE_TEST_MARKER","usage":{"input_tokens":100,"output_tokens":25}}}` + "\n"
	os.WriteFile(filepath.Join(logDir, "log.jsonl"), []byte(body), 0600)
	app := &nativeApp{sources: source, days: 7, token: "secret", apps: map[string]*claudeApp{source[0].ID: {root: source[0].Root, state: filepath.Join(root, "state"), token: "secret", days: 7}}}
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8765"+path, nil))
		return w
	}
	post := func(path, origin string, fields url.Values) int {
		req := httptest.NewRequest("POST", "http://127.0.0.1:8765"+path, strings.NewReader(fields.Encode()))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		return w.Code
	}
	var report native.Report
	w := get("/api/state")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &report) != nil || len(report.Sources) != 2 || len(report.Recipes) != 0 {
		t.Fatal(w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "PRIVATE_TEST_MARKER") {
		t.Fatal("content leak")
	}
	trial := url.Values{"token": {"secret"}, "recipe": {"mcp"}, "rating": {"4"}}
	if post("/api/try", "http://127.0.0.1:8765", trial) != 409 {
		t.Fatal("unscoped mutation")
	}
	if post("/api/try?source="+source[1].ID, "http://127.0.0.1:8765", trial) != 409 {
		t.Fatal("Codex settings mutated")
	}
	if post("/api/try?source="+source[0].ID, "https://evil.example", trial) != 403 {
		t.Fatal("cross origin")
	}
	if post("/api/try?source="+source[0].ID, "http://127.0.0.1:8765", trial) != 204 {
		t.Fatal("scoped trial failed")
	}
	w = get("/api/state?source=" + source[0].ID)
	if json.Unmarshal(w.Body.Bytes(), &report) != nil || len(report.Changes) != 1 || len(report.Recipes) != 3 {
		t.Fatal("history missing")
	}
	packRoot, _ := filepath.EvalSymlinks(t.TempDir())
	if _, err := pack.Open(packRoot, []string{"search"}, []string{"synthetic"}, time.Now(), time.Hour); err != nil {
		t.Fatal(err)
	}
	app.packRoot = packRoot
	w = get("/api/state")
	if json.Unmarshal(w.Body.Bytes(), &report) != nil || report.Packing == nil || report.Packing.Packed != 0 {
		t.Fatal("unverified activation")
	}
	fields := url.Values{"token": {"secret"}}
	if post("/api/pack-stop", "https://evil.example", fields) != 403 {
		t.Fatal("cross-origin pack stop")
	}
	if post("/api/pack-stop", "http://127.0.0.1:8765", fields) != 204 {
		t.Fatal("stop failed")
	}
	p, err := pack.ReadReport(packRoot, time.Now())
	if err != nil || !strings.HasPrefix(p.Status, "Stopped") {
		t.Fatal(p, err)
	}
	if get("/archives/anything.txt").Code != 404 {
		t.Fatal("archive route exposed")
	}
}

func TestNativeExportIsReadOnlyAndExplicit(t *testing.T) {
	root := t.TempDir()
	var out, stderr bytes.Buffer
	if code := run([]string{"watch", "--codex-dir", root, "--json"}, strings.NewReader(""), &out, &stderr); code != 0 {
		t.Fatal(code, stderr.String())
	}
	var r native.Report
	if json.Unmarshal(out.Bytes(), &r) != nil || len(r.Sources) != 1 || !r.Sources[0].Health.Missing {
		t.Fatal(out.String())
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatal("read-only export wrote state")
	}
}
