package claude

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(id, session string, at time.Time, output int) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"sessionId":%q,"cwd":"/private/example-project","effort":"high","version":"2.1.267","message":{"id":%q,"model":"claude-sonnet-4-6","content":[{"type":"text","text":"PRIVATE_PROMPT_SENTINEL"}],"usage":{"input_tokens":100,"cache_read_input_tokens":10000,"cache_creation_input_tokens":2000,"cache_creation":{"ephemeral_5m_input_tokens":500,"ephemeral_1h_input_tokens":1500},"output_tokens":%d}}}`+"\n", at.Format(time.RFC3339Nano), session, id, output)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestImportDedupeAndRefresh(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(root, "projects", "project", "session.jsonl")
	content := fixture("msg1", "session1", now, 10) + fixture("msg1", "session1", now.Add(time.Second), 100)
	content += `{"type":"user","message":{"content":"PRIVATE_USER_SENTINEL"}}` + "\n"
	content += "broken\n{"
	write(t, path, content)
	write(t, filepath.Join(root, "projects", "project", "session", "subagents", "agent.jsonl"), fixture("msg2", "session1", now.Add(time.Minute), 200))
	write(t, filepath.Join(root, "projects", "project", "copy.jsonl"), strings.ReplaceAll(fixture("msg1", "fork", now.Add(time.Hour), 100), "/private/example-project", "/other/example-project"))
	write(t, filepath.Join(root, "projects", "project", "old.orphaned-date.jsonl"), fixture("old", "old", now, 999))
	var reader Reader
	requests, h, err := reader.Read(root)
	if err != nil || len(requests) != 2 || h.Duplicates != 2 || h.Invalid != 1 || h.Partial != 1 {
		t.Fatalf("read: %d %+v %v", len(requests), h, err)
	}
	if requests[0].Tokens.Output != 100 || !requests[1].Subagent || requests[0].Session != requests[1].Session || requests[0].Time != now || requests[0].ProjectID != hash(filepath.Clean("/private/example-project")) {
		t.Fatalf("dedupe or ownership incorrect: %+v", requests)
	}
	report := Build(requests, h, now.Add(-time.Hour), now.Add(2*time.Hour))
	if report.Summary.Requests != 2 || report.Summary.Tokens.Output != 300 || len(report.Sessions) != 1 {
		t.Fatal("bad aggregate", report.Summary)
	}
	want := (200*3 + 20000*.3 + 1000*3.75 + 3000*6 + 300*15) / 1e6
	if math.Abs(report.Summary.KnownUSD-want) > 1e-12 {
		t.Fatalf("TTL pricing: %.12f != %.12f", report.Summary.KnownUSD, want)
	}
	b, _ := json.Marshal(report)
	for _, secret := range []string{"PRIVATE_PROMPT", "PRIVATE_USER", "/private/", "msg1", "session1", `"content"`} {
		if strings.Contains(string(b), secret) {
			t.Fatal("raw content escaped into report:", secret)
		}
	}
	var decoded map[string]any
	json.Unmarshal(b, &decoded)
	session := decoded["sessions"].([]any)[0].(map[string]any)
	if session["requests"] != float64(2) || len(session["timeline"].([]any)) != 2 {
		t.Fatal("session JSON shape drifted", session)
	}
	// Same Reader sees completed append, truncation, and deleted files.
	write(t, path, fixture("msg3", "session2", now, 300))
	if err := os.Remove(filepath.Join(root, "projects", "project", "copy.jsonl")); err != nil {
		t.Fatal(err)
	}
	requests, h, err = reader.Read(root)
	if err != nil || len(requests) != 2 || h.Invalid != 0 || h.Partial != 0 || h.Duplicates != 0 {
		t.Fatalf("stale cache: %d %+v %v", len(requests), h, err)
	}
}

func TestUnknownPricesAndMalformedUsage(t *testing.T) {
	now := time.Now()
	line := fixture("id", "session", now, 10)
	cases := []struct {
		name, line string
		invalid    bool
		unpriced   bool
	}{
		{"unknown model", strings.ReplaceAll(line, "claude-sonnet-4-6", "custom-model"), false, true},
		{"provider ID", strings.ReplaceAll(line, "claude-sonnet-4-6", "global.anthropic.claude-sonnet-4-6"), false, true},
		{"negative", strings.ReplaceAll(line, `"input_tokens":100`, `"input_tokens":-1`), true, false},
		{"fraction", strings.ReplaceAll(line, `"input_tokens":100`, `"input_tokens":1.5`), true, false},
		{"missing ID", strings.ReplaceAll(line, `"id":"id",`, ""), true, false},
		{"bad TTL", strings.ReplaceAll(line, `"ephemeral_5m_input_tokens":500`, `"ephemeral_5m_input_tokens":501`), true, false},
		{"unknown TTL", strings.ReplaceAll(line, `"cache_creation":{"ephemeral_5m_input_tokens":500,"ephemeral_1h_input_tokens":1500},`, ""), false, true},
		{"fast", strings.ReplaceAll(line, `"input_tokens":100`, `"speed":"fast","input_tokens":100`), false, true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			q, ignored, err := parse([]byte(tt.line), "session.jsonl")
			if (err != nil) != tt.invalid || ignored {
				t.Fatal("validation mismatch", err)
			}
			if !tt.invalid && (q.Cost == nil) != tt.unpriced {
				t.Fatal("price coverage mismatch")
			}
		})
	}
	var reader Reader
	_, h, err := reader.Read(t.TempDir())
	if err != nil || !h.Missing {
		t.Fatal("missing root should be an empty state")
	}
}

func TestTimeWindowsAndSignals(t *testing.T) {
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	q, _, _ := parse([]byte(fixture("id", "s", now, 100)), "a")
	q.Tokens.CachedInput = 120000
	second := q
	second.ID = "other"
	second.Time = now.Add(time.Minute)
	second.Tokens.CachedInput, second.Tokens.Input = 0, 120000
	r := Build([]Request{q, second}, Health{}, now, now.Add(time.Hour))
	if r.Summary.Requests != 2 || len(r.Findings) < 2 || len(r.Buckets) != 1 {
		t.Fatal("window/signals mismatch", r.Summary, r.Findings)
	}
	r = Build([]Request{q, second}, Health{}, now.Add(time.Minute), now.Add(time.Hour))
	if r.Summary.Requests != 1 {
		t.Fatal("window lower bound")
	}
	r = Build([]Request{q, second}, Health{}, now.Add(-time.Hour), now)
	if r.Summary.Requests != 0 {
		t.Fatal("window upper bound")
	}
}
