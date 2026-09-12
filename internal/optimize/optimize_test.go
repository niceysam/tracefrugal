package optimize

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/niceysam/tracefrugal/internal/claude"
	"github.com/niceysam/tracefrugal/internal/ledger"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		v := os.Getenv("TRACEFRUGAL_TEST_VERSION")
		if v == "" {
			v = ReviewedVersion
		}
		fmt.Printf("%s (Claude Code)\n", v)
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func setup(t *testing.T, initial string) (Config, Diagnosis, time.Time) {
	t.Helper()
	root, _ := filepath.EvalSymlinks(t.TempDir())
	project := filepath.Join(root, "my project")
	os.MkdirAll(filepath.Join(project, ".claude"), 0700)
	if initial != "" {
		os.WriteFile(filepath.Join(project, ".claude", "settings.local.json"), []byte(initial), 0600)
	}
	c := Config{Project: project, Store: filepath.Join(root, "store"), State: filepath.Join(root, "state"),
		Binary: os.Args[0], Self: filepath.Join(root, "Trace Frugal")}
	now := time.Now().UTC()
	d := Diagnose(c, []Document{{URL: "a"}, {URL: "b"}, {URL: "c"}, {URL: "d"}}, nil, now)
	return c, d, now
}
func TestApplyReceiptRestoreAndReapply(t *testing.T) {
	original := "{\n \"env\":{\"EXISTING\":\"retained\"}, \"permissions\":{\"deny\":[\"Bash(rm *)\"]}, \"hooks\":{\"Stop\":[{\"hooks\":[]}]}\n}\n"
	c, d, now := setup(t, original)
	baseline := []claude.Request{{ID: "before", Time: now.Add(-time.Hour), Model: "claude-sonnet-4-6", Tokens: ledger.Tokens{Input: 100, Output: 10}}}
	trial, err := Apply(c, d, d.Plan, 4, baseline, now)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(target(c))
	if envValue(after, Setting) != Limit || !bytes.Contains(after, []byte("Bash(rm *)")) || !bytes.Contains(after, []byte("SessionStart")) {
		t.Fatal("lost existing configuration or missing actual change")
	}
	event := fmt.Sprintf(`{"session_id":"actual-session","cwd":%q,"hook_event_name":"SessionStart","source":"startup","transcript_path":"/must-not-read","prompt":"PRIVATE_PROMPT"}`, c.Project)
	if err := Record(c.State, trial.ID, strings.NewReader(event), Limit, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	paths, _ := filepath.Glob(filepath.Join(c.State, trial.ID, "receipts", "*.json"))
	if len(paths) != 1 {
		t.Fatal("no activation receipt")
	}
	receipt, _ := os.ReadFile(paths[0])
	if bytes.Contains(receipt, []byte("PRIVATE_PROMPT")) || bytes.Contains(receipt, []byte("actual-session")) || bytes.Contains(receipt, []byte(c.Project)) {
		t.Fatal("receipt leaked content or raw identity")
	}
	if err := Restore(c, trial.ID, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(target(c))
	if !bytes.Equal(restored, []byte(original)) {
		t.Fatal("restore was not byte exact")
	}
	d = Diagnose(c, d.Documents, nil, now.Add(2*time.Minute))
	next, err := Apply(c, d, d.Plan, 4, baseline, now.Add(2*time.Minute))
	if err != nil || next.ID == trial.ID {
		t.Fatal("reapply must create independent history", err)
	}
	j, _ := load(c)
	if len(j.Trials) != 2 || j.Trials[0].Status != "restored" || j.Trials[1].Status != "active" {
		t.Fatal("history lost")
	}
}

func TestMissingFileRestoreDriftAndPreviewGuards(t *testing.T) {
	c, d, now := setup(t, "")
	if _, err := Apply(c, d, "stale", 4, nil, now); err == nil {
		t.Fatal("stale plan accepted")
	}
	x, err := Apply(c, d, d.Plan, 4, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(target(c))
	os.WriteFile(target(c), append(after, '\n'), 0600)
	if Restore(c, x.ID, now) == nil {
		t.Fatal("external edits overwritten")
	}
	os.WriteFile(target(c), after, 0600)
	if err := Restore(c, x.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target(c)); !os.IsNotExist(err) {
		t.Fatal("new settings file not removed")
	}
}

func TestInterruptedApplyRecoveryAndTamperedBackup(t *testing.T) {
	c, d, now := setup(t, `{"env":{}}`)
	x, err := Apply(c, d, d.Plan, 3, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	j, _ := load(c)
	j.Trials[0].Status = "prepared" // crash after atomic settings replacement
	save(c, j)
	backup := filepath.Join(c.State, x.ID, "before.json")
	before, _ := os.ReadFile(backup)
	os.WriteFile(backup, []byte("tampered"), 0600)
	if Restore(c, x.ID, now) == nil {
		t.Fatal("tampered backup accepted")
	}
	os.WriteFile(backup, before, 0600)
	if err := Restore(c, x.ID, now); err != nil {
		t.Fatal("cannot recover interrupted apply", err)
	}
}

func TestUnsupportedVersionAndAlreadySmallerSetting(t *testing.T) {
	c, d, now := setup(t, `{"env":{"MAX_MCP_OUTPUT_TOKENS":"2000"}}`)
	if d.Ready {
		t.Fatal("would increase an already smaller setting")
	}
	t.Setenv("TRACEFRUGAL_TEST_VERSION", "9.0.0")
	d = Diagnose(c, d.Documents, nil, now)
	if d.Ready || !strings.Contains(d.Reason, "compatibility") {
		t.Fatal("unknown CLI version accepted")
	}
	t.Setenv("TRACEFRUGAL_TEST_VERSION", ReviewedVersion)
	d = Diagnose(c, nil, fmt.Errorf("offline"), now)
	if d.Ready {
		t.Fatal("unverified documents accepted")
	}
}

func TestProjectAndReceiptScopedUsageSurvivesLogRemoval(t *testing.T) {
	c, d, now := setup(t, `{}`)
	baseline := []claude.Request{{ID: "b", Time: now.Add(-time.Hour), Tokens: ledger.Tokens{Input: 1000}}}
	x, err := Apply(c, d, d.Plan, 4, baseline, now)
	if err != nil {
		t.Fatal(err)
	}
	event := fmt.Sprintf(`{"session_id":"verified","cwd":%q,"hook_event_name":"SessionStart","source":"startup"}`, c.Project)
	if err := Record(c.State, x.ID, strings.NewReader(event), Limit, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(c.Store, "projects", "fixture")
	os.MkdirAll(dir, 0700)
	log := ""
	for i, pair := range [][2]string{{"verified", c.Project}, {"unverified", c.Project}, {"verified", filepath.Join(c.Project, "other")}, {"verified", c.Project}} {
		row := map[string]any{"type": "assistant", "version": ReviewedVersion, "sessionId": pair[0], "cwd": pair[1], "timestamp": now.Add(2 * time.Second).Format(time.RFC3339Nano),
			"message": map[string]any{"id": fmt.Sprintf("r%d", i), "model": "claude-sonnet-4-6", "usage": map[string]any{"input_tokens": 250, "output_tokens": 20}}}
		if i == 3 {
			row["version"] = "2.1.236"
		}
		b, _ := json.Marshal(row)
		log += string(b) + "\n"
	}
	p := filepath.Join(dir, "log.jsonl")
	os.WriteFile(p, []byte(log), 0600)
	reader := &claude.Reader{}
	report, err := Snapshot(c, d, reader, now.Add(time.Hour))
	if err != nil || len(report.Trials) != 1 {
		t.Fatal(err)
	}
	v := report.Trials[0]
	if v.After.Requests != 1 || v.After.Tokens.Input != 250 || v.Baseline.Tokens.Input != 1000 {
		t.Fatal("usage crossed project/session boundary", v.After, v.Baseline)
	}
	os.Remove(p)
	report, err = Snapshot(c, d, reader, now.Add(2*time.Hour))
	if err != nil || report.Trials[0].After.Requests != 1 {
		t.Fatal("observed usage lost after log retention", err)
	}
	if err := Rate(c, x.ID, 2, "regression", now); err != nil {
		t.Fatal(err)
	}
	report, _ = Snapshot(c, d, reader, now.Add(3*time.Hour))
	if !strings.Contains(report.Trials[0].Assessment, "Quality regression") {
		t.Fatal("quality regression hidden by token reduction")
	}
}

func TestKeepHistoryAndRestoreWithMissingUsage(t *testing.T) {
	c, d, now := setup(t, `{"custom":true}`)
	trial, err := Apply(c, d, d.Plan, 0, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := Rate(c, trial.ID, 4, "keep", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	r, err := Snapshot(c, d, &claude.Reader{}, now.Add(time.Minute))
	if err != nil || !r.Health.Missing || !r.Trials[0].CanRestore || r.Trials[0].Events[2].Action != "kept" {
		t.Fatal("missing logs hid history or restore", err)
	}
	if err := Restore(c, trial.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if Rate(c, trial.ID, 4, "keep", now) == nil {
		t.Fatal("restored settings cannot be kept")
	}
}

func TestWrongEffectiveValueAndWrongProjectDoNotActivate(t *testing.T) {
	c, d, now := setup(t, `{}`)
	x, _ := Apply(c, d, d.Plan, 4, nil, now)
	event := fmt.Sprintf(`{"session_id":"wrong-value","cwd":%q,"hook_event_name":"SessionStart","source":"startup"}`, c.Project)
	if err := Record(c.State, x.ID, strings.NewReader(event), "25000", now); err != nil {
		t.Fatal(err)
	}
	report, err := Snapshot(c, d, &claude.Reader{}, now.Add(time.Minute))
	if err != nil || !strings.Contains(report.Trials[0].Assessment, "Waiting for a Claude SessionStart") {
		t.Fatal("wrong effective value treated as active", err)
	}
	event = strings.ReplaceAll(event, c.Project, filepath.Dir(c.Project))
	if Record(c.State, x.ID, strings.NewReader(event), Limit, now) == nil {
		t.Fatal("wrong project accepted")
	}
}
