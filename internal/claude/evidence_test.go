package claude

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestToolEvidenceAttributionDedupeAndPrivacy(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	line := func(kind, id, content string) string {
		return fmt.Sprintf(`{"type":%q,"uuid":%q,"sessionId":"session","timestamp":%q,"message":{"content":%s}}`+"\n", kind, id, now.Format(time.RFC3339), content)
	}
	call := func(id string) string {
		return line("assistant", id, `[{"type":"tool_use","id":"`+id+`","name":"mcp__example__search","input":{"query":"PRIVATE_ARGUMENT"}}]`)
	}
	body, _ := json.Marshal(strings.Repeat("PRIVATE_RESULT", 2000))
	result := line("user", "result1", `[{"type":"tool_result","tool_use_id":"call1","content":`+string(body)+`}]`)
	content := line("user", "turn1", `"PRIVATE_USER"`) + call("call1") + call("call1") + call("call2") + result + result
	write(t, filepath.Join(root, "projects", "example", "session.jsonl"), content)
	write(t, filepath.Join(root, "projects", "example", "session", "subagents", "a.jsonl"), line("user", "subturn", `"SUBAGENT_PROMPT"`))
	var reader Reader
	_, _, err := reader.Read(root)
	if err != nil {
		t.Fatal(err)
	}
	events := reader.Evidence()
	d := Diagnose(events, Summary{}, now.Add(-time.Second), now.Add(time.Second))
	if d.HumanTurns != 1 || d.ToolCalls != 2 || d.RepeatedCalls != 1 || d.LargeResults != 1 || d.ResultBytes != int64(len(body)) || d.MCPBytes != d.ResultBytes {
		t.Fatal("bad tool evidence", d)
	}
	export, _ := json.Marshal(events)
	if strings.Contains(string(export), "PRIVATE_") || strings.Contains(string(export), "SUBAGENT_PROMPT") {
		t.Fatal("payload retained")
	}
	empty := Diagnose(events, Summary{}, now.Add(time.Second), now.Add(time.Hour))
	if empty.ToolCalls != 0 || empty.HumanTurns != 0 || empty.ResultBytes != 0 {
		t.Fatal("window boundary")
	}
}
