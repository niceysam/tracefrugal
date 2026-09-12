package native

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiscoveryAndMixedCoverage(t *testing.T) {
	home := t.TempDir()
	claudeRoot := filepath.Join(home, ".claude")
	codexRoot := filepath.Join(home, ".codex-sandbox")
	os.MkdirAll(filepath.Join(claudeRoot, "projects"), 0700)
	os.MkdirAll(filepath.Join(codexRoot, "sessions"), 0700)
	os.WriteFile(filepath.Join(claudeRoot, "projects/a.jsonl"), []byte(`{"type":"assistant","timestamp":"2026-09-10T10:00:00Z","sessionId":"s","message":{"id":"r","model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":20}}}`+"\n"), 0600)
	os.WriteFile(filepath.Join(codexRoot, "sessions/b.jsonl"), []byte(`{"type":"token_usage_record","timestamp":"2026-09-10T11:00:00Z","payload":{"response_id":"r","thread_id":"s","usage":{"input_tokens":1000,"cached_input_tokens":900,"output_tokens":40,"reasoning_output_tokens":10,"total_tokens":1040}}}`+"\n"), 0600)
	sources, err := Discover(home, claudeRoot, codexRoot, nil)
	if err != nil || len(sources) != 3 {
		t.Fatal(sources, err)
	} // claude, missing codex, sandbox
	r := Reader{}
	from := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	report, err := r.Read(sources, "", from, from.Add(24*time.Hour))
	if err != nil || report.Summary.Requests != 2 || report.Summary.Tokens.Input != 200 || report.Summary.Reasoning != 10 {
		t.Fatal(report.Summary, err)
	}
	if len(report.Sessions) != 2 || len(report.Sources) != 3 || len(report.Recipes) != 0 {
		t.Fatal("incorrect source isolation")
	}
	b, _ := json.Marshal(report)
	if strings.Contains(string(b), home) {
		t.Fatal("raw root path leaked")
	}
	filtered, err := r.Read(sources, sources[0].ID, from, from.Add(24*time.Hour))
	if err != nil || filtered.Summary.Requests != 1 {
		t.Fatal(filtered.Summary, err)
	}
	if _, err = r.Read(sources, "bad", from, from.Add(24*time.Hour)); err == nil {
		t.Fatal("unknown source accepted")
	}
	explicit, err := Discover(home, "", "", []Source{{Harness: "codex", Root: codexRoot}, {Harness: "codex", Root: codexRoot}})
	if err != nil || len(explicit) != 1 {
		t.Fatal("explicit root did not replace discovery")
	}
}

func TestCrossRootDuplicateAndConflict(t *testing.T) {
	home := t.TempDir()
	var roots []Source
	for i, name := range []string{"a", "b"} {
		root := filepath.Join(home, name)
		os.MkdirAll(filepath.Join(root, "sessions"), 0700)
		output := 20 + i
		row := map[string]any{"type": "token_usage_record", "timestamp": "2026-09-10T11:00:00Z", "payload": map[string]any{"response_id": "same", "thread_id": "thread", "usage": map[string]int{"input_tokens": 100, "output_tokens": output, "total_tokens": 100 + output}}}
		b, _ := json.Marshal(row)
		os.WriteFile(filepath.Join(root, "sessions/log.jsonl"), b, 0600)
		roots = append(roots, Source{Harness: "codex", Root: root})
	}
	sources, _ := Discover(home, "", "", roots)
	r := Reader{}
	now := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)
	report, err := r.Read(sources, "", now.Add(-24*time.Hour), now)
	if err != nil || report.Summary.Requests != 1 || report.Summary.Tokens.Output != 20 || report.Sources[1].CrossDuplicates != 1 || report.Health.Conflicts != 1 {
		t.Fatal(report, err)
	}
}
