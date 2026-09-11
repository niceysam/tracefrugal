package main

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/niceysam/tracefrugal/internal/ledger"
	"github.com/niceysam/tracefrugal/internal/webui"
)

func TestDashboardRefreshAndInvalidTrace(t *testing.T) {
	trace := filepath.Join(t.TempDir(), "run.jsonl")
	h := dashboard(trace, "../../examples/prices.json")
	get := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/state", nil))
		return w
	}
	if w := get(); w.Code != 503 {
		t.Fatalf("missing trace: %d", w.Code)
	}
	data, err := os.ReadFile("../../examples/baseline.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trace, data, 0600); err != nil {
		t.Fatal(err)
	}
	w := get()
	var payload struct {
		Report ledger.Report `json:"report"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &payload) != nil || payload.Report.CostUSD != .66 {
		t.Fatalf("report: %d %s", w.Code, w.Body.String())
	}
	data, err = os.ReadFile("../../examples/candidate-good.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trace, data, 0600); err != nil {
		t.Fatal(err)
	}
	if w = get(); json.Unmarshal(w.Body.Bytes(), &payload) != nil || math.Abs(payload.Report.CostUSD-.21) > 1e-12 {
		t.Fatal("stale report")
	}
	if err := os.WriteFile(trace, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if w = get(); w.Code != 422 || strings.Contains(w.Body.String(), "estimated_cost_usd") {
		t.Fatal("invalid trace presented as valid")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 405 {
		t.Fatal("accepted write")
	}
}

func TestHistoryControlsRequireSameOriginAndToken(t *testing.T) {
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(state, "paused"), []byte("paused"), 0600); err != nil {
		t.Fatal(err)
	}
	h := historyDashboard(state)
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest("GET", "http://127.0.0.1:8765/", nil))
	match := regexp.MustCompile(`<script id="bootstrap" type="application/json">(.+)</script>`).FindStringSubmatch(get.Body.String())
	if get.Code != 200 || len(match) != 2 {
		t.Fatalf("%d %s", get.Code, get.Body.String())
	}
	var options webui.Options
	if err := json.Unmarshal([]byte(match[1]), &options); err != nil || options.Token == "" {
		t.Fatal("missing bootstrap token", err)
	}
	post := func(origin, token string) int {
		req := httptest.NewRequest("POST", "http://127.0.0.1:8765/resume", strings.NewReader(url.Values{"token": {token}}.Encode()))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}
	if post("https://other.example", options.Token) != 403 {
		t.Fatal("cross-origin write permitted")
	}
	if post("http://127.0.0.1:8765", "bad") != 403 {
		t.Fatal("bad token permitted")
	}
	if _, err := os.Stat(filepath.Join(state, "paused")); err != nil {
		t.Fatal("unauthorized state change")
	}
	if post("http://127.0.0.1:8765", options.Token) != 303 {
		t.Fatal("valid resume failed")
	}
	if _, err := os.Stat(filepath.Join(state, "paused")); !os.IsNotExist(err) {
		t.Fatal("resume did not remove pause")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "http://rebind.example:8765/", nil))
	if w.Code != 403 {
		t.Fatal("untrusted host allowed")
	}
}
