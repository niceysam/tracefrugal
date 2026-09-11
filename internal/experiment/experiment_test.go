package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The test binary is a portable evaluator subprocess on all supported OSes.
func TestEvaluatorHelper(t *testing.T) {
	path := os.Getenv("TRACEFRUGAL_PROFILE")
	if path == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		os.Exit(2)
	}
	var p struct {
		Cap  int64 `json:"max_output_tokens"`
		Fail bool  `json:"fail"`
	}
	if json.Unmarshal(b, &p) != nil || p.Fail {
		os.Exit(3)
	}
	trace := fmt.Sprintf("{\"type\":\"request\",\"task_id\":\"one\",\"request_id\":\"r\",\"model\":\"demo/model\",\"tokens\":{\"input\":100,\"output\":%d}}\n{\"type\":\"task_result\",\"task_id\":\"one\",\"success\":%t}\n", p.Cap, p.Cap >= 100)
	if os.WriteFile(os.Getenv("TRACEFRUGAL_TRACE"), []byte(trace), 0600) != nil {
		os.Exit(4)
	}
	os.Exit(0)
}

func setup(t *testing.T, cap int) (string, string) {
	t.Helper()
	dir := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("base.json", fmt.Sprintf(`{"max_output_tokens":%d}`, cap))
	write("prices.json", `{"schema_version":1,"currency":"USD","label":"synthetic","models":{"demo/model":{"input":1,"output":10}}}`)
	factor := 0.5
	c := Config{BaselineProfile: "base.json", Prices: "prices.json", Command: []string{os.Args[0], "-test.run=^TestEvaluatorHelper$"}, TimeoutSeconds: 10, AutoApply: true, OutputTokenFactor: &factor, MinOutputTokens: 50}
	b, _ := json.Marshal(c)
	write("experiment.json", string(b))
	return filepath.Join(dir, "experiment.json"), filepath.Join(dir, "state")
}

func TestApplyRollbackPauseAndResume(t *testing.T) {
	config, state := setup(t, 1000)
	e, err := Run(context.Background(), config, state)
	if err != nil || e.Status != "applied" || !e.Applied {
		t.Fatalf("%+v %v", e, err)
	}
	after, _ := os.ReadFile(filepath.Join(state, "active.json"))
	if digest(after) != e.AfterHash {
		t.Fatal("profile not activated")
	}
	rollback, err := Rollback(state, e.ID)
	if err != nil || rollback.Status != "rolled_back" {
		t.Fatalf("%+v %v", rollback, err)
	}
	before, _ := os.ReadFile(filepath.Join(state, "active.json"))
	if digest(before) != e.BeforeHash {
		t.Fatal("not restored byte for byte")
	}
	if _, err = Run(context.Background(), config, state); err == nil {
		t.Fatal("reapplied after rollback")
	}
	if _, err = Resume(state); err != nil {
		t.Fatal(err)
	}
	h, err := History(state)
	if err != nil || len(h) != 3 {
		t.Fatalf("%+v %v", h, err)
	}
	if _, err = Run(context.Background(), config, state); err != nil {
		t.Fatal(err)
	}
}

func TestQualityRegressionRetainsProfile(t *testing.T) {
	config, state := setup(t, 100)
	e, err := Run(context.Background(), config, state)
	if err != nil || e.Status != "rejected" || e.Applied {
		t.Fatalf("%+v %v", e, err)
	}
	active, _ := os.ReadFile(filepath.Join(state, "active.json"))
	if digest(active) != e.BeforeHash {
		t.Fatal("bad candidate applied")
	}
}

func TestRollbackRefusesDriftAndTamperedSnapshot(t *testing.T) {
	config, state := setup(t, 1000)
	e, err := Run(context.Background(), config, state)
	if err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(state, "active.json")
	if err = os.WriteFile(active, []byte(`{"manual":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Rollback(state, e.ID); err == nil {
		t.Fatal("overwrote manual edit")
	}
	candidateData, err := os.ReadFile(filepath.Join(state, "runs", e.ID, "candidate.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(active, candidateData, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(state, "runs", e.ID, "before.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Rollback(state, e.ID); err == nil {
		t.Fatal("restored tampered snapshot")
	}
}

func TestErrorsAreJournaledAndDoNotPromote(t *testing.T) {
	config, state := setup(t, 1000)
	if err := os.WriteFile(filepath.Join(filepath.Dir(config), "base.json"), []byte(`{"max_output_tokens":1000,"fail":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	e, err := Run(context.Background(), config, state)
	if err == nil || e.Status != "error" || e.Applied {
		t.Fatalf("%+v %v", e, err)
	}
	h, err := History(state)
	if err != nil || len(h) != 1 || h[0].Status != "error" {
		t.Fatalf("%+v %v", h, err)
	}
	var html strings.Builder
	if err = WriteHistory(&html, state); err != nil || !strings.Contains(html.String(), "Experiment incomplete") {
		t.Fatalf("%s %v", html.String(), err)
	}
}

func TestStateLock(t *testing.T) {
	config, state := setup(t, 1000)
	unlock, err := lock(state)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err = Run(context.Background(), config, state); err == nil {
		t.Fatal("concurrent run permitted")
	}
}
