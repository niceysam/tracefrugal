package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const meta = `{"type":"session_meta","payload":{"id":"thread","base_instructions":"PRIVATE_SENTINEL"}}`
const turn = `{"type":"turn_context","payload":{"model":"test-model","effort":"high","cwd":"/private/PRIVATE_SENTINEL"}}`

func receipt(id string) string {
	return `{"type":"token_usage_record","timestamp":"2026-09-10T10:00:00Z","payload":{"response_id":"` + id + `","thread_id":"thread","usage":{"input_tokens":1000,"cached_input_tokens":800,"cache_write_input_tokens":100,"output_tokens":40,"reasoning_output_tokens":20,"total_tokens":1040},"thread_token_usage":{"input_tokens":999999,"output_tokens":999}}}`
}
func legacy(stamp string, input, output int64, lastInput, lastOutput int64) string {
	b, _ := json.Marshal(map[string]any{"type": "event_msg", "timestamp": stamp, "payload": map[string]any{"type": "token_count", "info": map[string]any{
		"total_token_usage": map[string]int64{"input_tokens": input, "output_tokens": output, "total_tokens": input + output},
		"last_token_usage":  map[string]int64{"input_tokens": lastInput, "output_tokens": lastOutput, "total_tokens": lastInput + lastOutput},
	}}})
	return string(b)
}
func write(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestReceiptsDedupeAndPrivacy(t *testing.T) {
	root := t.TempDir()
	body := strings.Join([]string{meta, turn, receipt("r"), receipt("r"), legacy("2026-09-10T10:00:00Z", 1000, 40, 1000, 40)}, "\n") + "\n"
	write(t, root, "sessions/one.jsonl", body)
	write(t, root, "archived_sessions/copy.jsonl", body)
	r := Reader{}
	rows, h := r.Read(root)
	if len(rows) != 1 || h.Duplicates != 3 || h.Invalid != 0 {
		t.Fatalf("%+v %+v", rows, h)
	}
	q := rows[0]
	if q.Tokens.Input != 100 || q.Tokens.CachedInput != 800 || q.Tokens.CacheWrite != 100 || q.Tokens.Output != 40 || q.Reasoning != 20 || q.Cost != nil || q.Model != "test-model" {
		t.Fatal(q)
	}
	b, _ := json.Marshal(rows)
	if strings.Contains(string(b), "PRIVATE_SENTINEL") || strings.Contains(string(b), "999999") {
		t.Fatal("transcript or cumulative counter leaked")
	}
}
func TestLegacySnapshotsResetsAndGaps(t *testing.T) {
	root := t.TempDir()
	body := strings.Join([]string{meta,
		legacy("2026-09-10T10:00:00Z", 100, 10, 100, 10),
		legacy("2026-09-10T10:00:01Z", 100, 10, 100, 10), // repeated
		legacy("2026-09-10T10:01:00Z", 220, 22, 120, 12),
		legacy("2026-09-10T10:02:00Z", 10, 1, 10, 1), // reset, not another full history
		legacy("2026-09-10T10:03:00Z", 30, 3, 20, 2),
		legacy("2026-09-10T10:04:00Z", 300, 30, 50, 5), // omitted calls
	}, "\n") + "\n"
	write(t, root, "sessions/legacy.jsonl", body)
	r := Reader{}
	rows, h := r.Read(root)
	if len(rows) != 3 || h.Legacy != 3 || h.Gaps != 2 {
		t.Fatalf("%d %+v", len(rows), h)
	}
	var n int64
	for _, q := range rows {
		n += q.Tokens.Input
	}
	if n != 240 {
		t.Fatal(n)
	}
}
func TestLiveAppendValidationAndRecovery(t *testing.T) {
	root := t.TempDir()
	write(t, root, "sessions/log.jsonl", meta+"\n"+receipt("one")+"\n"+`{"type":`)
	r := Reader{}
	rows, h := r.Read(root)
	if len(rows) != 1 || h.Partial != 1 {
		t.Fatal(h)
	}
	bad := strings.Replace(receipt("bad"), `"cached_input_tokens":800`, `"cached_input_tokens":1800`, 1)
	write(t, root, "sessions/log.jsonl", meta+"\n"+receipt("one")+"\n"+receipt("two")+"\n"+bad+"\n"+strings.Repeat("x", maxLine+2)+"\n"+receipt("three")+"\n")
	rows, h = r.Read(root)
	if len(rows) != 3 || h.Invalid != 2 || h.Partial != 0 {
		t.Fatalf("%d %+v", len(rows), h)
	}
	os.Remove(filepath.Join(root, "sessions/log.jsonl"))
	rows, _ = r.Read(root)
	if len(rows) != 0 {
		t.Fatal("deleted file retained")
	}
}
