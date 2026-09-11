package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDashboardRefreshAndInvalidTrace(t *testing.T) {
	trace := filepath.Join(t.TempDir(), "run.jsonl")
	h := dashboard(trace, "../../examples/prices.json")
	get := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
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
	if w.Code != 200 || !strings.Contains(w.Body.String(), "$0.660000") || w.Header().Get("Refresh") != "3" {
		t.Fatalf("report: %d %s", w.Code, w.Body.String())
	}
	data, err = os.ReadFile("../../examples/candidate-good.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trace, data, 0600); err != nil {
		t.Fatal(err)
	}
	if w = get(); !strings.Contains(w.Body.String(), "$0.210000") {
		t.Fatal("stale report")
	}
	if err := os.WriteFile(trace, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if w = get(); w.Code != 422 || strings.Contains(w.Body.String(), "$0.210000") {
		t.Fatal("invalid trace presented as valid")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/", nil))
	if w.Code != 405 {
		t.Fatal("accepted write")
	}
}
