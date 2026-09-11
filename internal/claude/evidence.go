package claude

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// Evidence describes observed tool traffic, not input-token attribution.
// Payloads and arguments are measured/hashed locally, then discarded.
type Evidence struct {
	ID, Session, Kind, Tool, Signature string
	Time                               time.Time
	Bytes                              int64
}

type Tool struct {
	Name     string `json:"name"`
	Calls    int    `json:"calls"`
	Repeated int    `json:"repeated"`
	Bytes    int64  `json:"bytes"`
	Largest  int64  `json:"largest"`
}

type Diagnostics struct {
	HumanTurns    int     `json:"human_turns"`
	ToolCalls     int     `json:"tool_calls"`
	ResultBytes   int64   `json:"result_bytes"`
	MCPBytes      int64   `json:"mcp_bytes"`
	LargeResults  int     `json:"large_results"`
	RepeatedCalls int     `json:"repeated_calls"`
	Tools         []Tool  `json:"tools"`
	AvgInput      float64 `json:"avg_input"`
	AvgOutput     float64 `json:"avg_output"`
}

func evidence(line []byte, subagent bool) []Evidence {
	var row struct {
		Type      string    `json:"type"`
		UUID      string    `json:"uuid"`
		Session   string    `json:"sessionId"`
		Timestamp time.Time `json:"timestamp"`
		IsMeta    bool      `json:"isMeta"`
		Message   struct {
			ID      string          `json:"id"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &row) != nil || row.Session == "" || row.Timestamp.IsZero() {
		return nil
	}
	var blocks []struct {
		Type    string          `json:"type"`
		ID      string          `json:"id"`
		ToolID  string          `json:"tool_use_id"`
		Name    string          `json:"name"`
		Input   json.RawMessage `json:"input"`
		Content json.RawMessage `json:"content"`
	}
	json.Unmarshal(row.Message.Content, &blocks)
	var out []Evidence
	hasResult := false
	for _, b := range blocks {
		if row.Type == "assistant" && b.Type == "tool_use" && b.ID != "" && b.Name != "" {
			out = append(out, Evidence{ID: hash("call:" + b.ID), Session: hash(row.Session), Kind: "call", Tool: b.Name, Signature: hash(b.Name + ":" + string(b.Input)), Time: row.Timestamp})
		}
		if row.Type == "user" && b.Type == "tool_result" && b.ToolID != "" {
			hasResult = true
			out = append(out, Evidence{ID: hash("result:" + b.ToolID), Session: hash(row.Session), Kind: "result", Signature: hash("call:" + b.ToolID), Bytes: int64(len(b.Content)), Time: row.Timestamp})
		}
	}
	if row.Type == "user" && row.UUID != "" && !hasResult && !row.IsMeta && !subagent {
		out = append(out, Evidence{ID: hash("turn:" + row.UUID), Session: hash(row.Session), Kind: "turn", Time: row.Timestamp})
	}
	return out
}

func (r *Reader) Evidence() []Evidence {
	r.mu.Lock()
	defer r.mu.Unlock()
	unique := map[string]Evidence{}
	for _, file := range r.files {
		for _, e := range file.evidence {
			old, ok := unique[e.ID]
			if !ok || e.Time.Before(old.Time) || (e.Time.Equal(old.Time) && e.Bytes > old.Bytes) {
				unique[e.ID] = e
			}
		}
	}
	result := make([]Evidence, 0, len(unique))
	for _, e := range unique {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Time.Equal(result[j].Time) {
			return result[i].ID < result[j].ID
		}
		return result[i].Time.Before(result[j].Time)
	})
	return result
}

func Diagnose(events []Evidence, summary Summary, from, until time.Time) Diagnostics {
	d := Diagnostics{Tools: []Tool{}}
	if summary.Requests > 0 {
		d.AvgInput = float64(total(summary.Tokens)-summary.Tokens.Output) / float64(summary.Requests)
		d.AvgOutput = float64(summary.Tokens.Output) / float64(summary.Requests)
	}
	names := map[string]string{}
	for _, e := range events {
		if e.Kind == "call" {
			names[e.ID] = e.Tool
		}
	}
	tools := map[string]*Tool{}
	signatures := map[string]bool{}
	for _, e := range events {
		if e.Time.Before(from) || !e.Time.Before(until) {
			continue
		}
		if e.Kind == "turn" {
			d.HumanTurns++
			continue
		}
		name := e.Tool
		if e.Kind == "result" {
			name = names[e.Signature]
			if name == "" {
				name = "Unmatched tool result"
			}
		}
		t := tools[name]
		if t == nil {
			t = &Tool{Name: name}
			tools[name] = t
		}
		if e.Kind == "call" {
			t.Calls++
			d.ToolCalls++
			key := e.Session + ":" + e.Signature
			if signatures[key] {
				t.Repeated++
				d.RepeatedCalls++
			}
			signatures[key] = true
		} else if e.Kind == "result" {
			t.Bytes += e.Bytes
			if e.Bytes > t.Largest {
				t.Largest = e.Bytes
			}
			d.ResultBytes += e.Bytes
			if strings.HasPrefix(name, "mcp__") {
				d.MCPBytes += e.Bytes
			}
			if e.Bytes >= 20_000 {
				d.LargeResults++
			}
		}
	}
	for _, t := range tools {
		d.Tools = append(d.Tools, *t)
	}
	sort.Slice(d.Tools, func(i, j int) bool {
		if d.Tools[i].Bytes == d.Tools[j].Bytes {
			return d.Tools[i].Name < d.Tools[j].Name
		}
		return d.Tools[i].Bytes > d.Tools[j].Bytes
	})
	return d
}
