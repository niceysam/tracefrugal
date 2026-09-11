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

	"github.com/niceysam/tracefrugal/internal/claude"
)

func TestClaudeFirstRunAndSecurity(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	app := &claudeApp{root: root, state: state, token: "test-token", days: 7}
	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		app.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:8765"+path, nil))
		return w
	}
	w := get("/api/state")
	var report claude.Report
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &report) != nil || !report.Health.Missing || report.Summary.Requests != 0 {
		t.Fatal("first run not usable", w.Code, w.Body.String())
	}
	if !strings.Contains(get("/").Body.String(), "Smaller context. Still useful answers.") || get("/claude.js").Code != 200 {
		t.Fatal("missing embedded native dashboard")
	}
	if get("/api/state?days=999").Code != 400 || get("/settings.json").Code != 404 {
		t.Fatal("route boundary")
	}
	req := httptest.NewRequest("GET", "http://evil.example:8765/api/state", nil)
	w = httptest.NewRecorder()
	app.ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatal("DNS rebinding allowed")
	}
	for _, origin := range []string{"", "https://evil.example"} {
		req = httptest.NewRequest("POST", "http://127.0.0.1:8765/api/try", strings.NewReader(url.Values{"token": {"test-token"}, "model": {"claude-sonnet-4-6"}}.Encode()))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w = httptest.NewRecorder()
		app.ServeHTTP(w, req)
		if w.Code != 403 {
			t.Fatal("cross-origin settings mutation")
		}
	}
}

func TestClaudeExportNeedsNoStateOrPriceFile(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "example")
	os.MkdirAll(dir, 0700)
	line := `{"type":"assistant","timestamp":"` + time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano) + `","sessionId":"example","cwd":"/demo","message":{"id":"example","model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":25}}}` + "\n"
	os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte(line), 0600)
	var out, stderr bytes.Buffer
	if code := run([]string{"claude", "--dir", root, "--json"}, strings.NewReader(""), &out, &stderr); code != 0 {
		t.Fatal("export failed", code, stderr.String())
	}
	var report claude.Report
	if json.Unmarshal(out.Bytes(), &report) != nil || report.Summary.Requests != 1 || report.Summary.Unpriced != 0 || report.Summary.Tokens.Output != 25 {
		t.Fatal("wrong export", out.String())
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 || entries[0].Name() != "projects" {
		t.Fatal("read-only export wrote state")
	}
}

func TestClaudeTrialHTTPRoundTrip(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	dir := filepath.Join(root, "projects", "example")
	os.MkdirAll(dir, 0700)
	line := `{"type":"assistant","timestamp":"` + time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano) + `","sessionId":"example","message":{"id":"example","model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":25}}}` + "\n"
	os.WriteFile(filepath.Join(dir, "session.jsonl"), []byte(line), 0600)
	app := &claudeApp{root: root, state: state, token: "test-token", days: 7}
	post := func(path string, form url.Values) int {
		t.Helper()
		form.Set("token", "test-token")
		req := httptest.NewRequest("POST", "http://127.0.0.1:8765"+path, strings.NewReader(form.Encode()))
		req.Header.Set("Origin", "http://127.0.0.1:8765")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)
		return w.Code
	}
	if post("/api/try", url.Values{"recipe": {"mcp"}, "rating": {"4"}}) != 204 {
		t.Fatal("trial refused")
	}
	report, err := app.snapshot(7)
	if err != nil || len(report.Changes) != 1 || report.Changes[0].RuleState != "present" {
		t.Fatal("trial not visible", err)
	}
	id := report.Changes[0].ID
	if post("/api/rate", url.Values{"id": {id}, "rating": {"5"}}) != 409 {
		t.Fatal("rated without later usage")
	}
	later := strings.ReplaceAll(line, `"id":"example"`, `"id":"later"`)
	// Put the later record just after start and before the next HTTP observation.
	var row map[string]any
	json.Unmarshal([]byte(later), &row)
	row["timestamp"] = report.Changes[0].Started.Add(time.Nanosecond).UTC().Format(time.RFC3339Nano)
	b, _ := json.Marshal(row)
	os.WriteFile(filepath.Join(dir, "later.jsonl"), b, 0600)
	if post("/api/rate", url.Values{"id": {id}, "rating": {"2"}}) != 204 {
		t.Fatal("rating failed")
	}
	if post("/api/restore", url.Values{"id": {id}}) != 204 {
		t.Fatal("restore failed")
	}
	report, err = app.snapshot(7)
	if err != nil || report.Changes[0].AfterRating != 2 || report.Changes[0].Status != "restored" || report.Changes[0].After.Requests != 1 {
		t.Fatal("history missing")
	}
}
