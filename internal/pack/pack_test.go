package pack

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func engine(t *testing.T) *Engine {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e, err := Open(root, []string{"search"}, []string{"synthetic-upstream"}, time.Now().Add(-time.Minute), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func textResult(text string) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": text}}})
	return b
}
func TestExactUnicodeRecallAndAccounting(t *testing.T) {
	e := engine(t)
	text := strings.Repeat("가나다🙂\n", 5000) + "critical evidence at the end"
	raw := textResult(text)
	got := e.Transform("search", true, raw)
	if bytes.Equal(got, raw) || !bytes.Contains(got, []byte("incomplete")) || len(got) >= len(raw) {
		t.Fatal("not packed")
	}
	offset := 0
	var recalled strings.Builder
	for {
		b, err := e.Recall(digest(text), offset, 701)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		json.Unmarshal(b, &result)
		var page struct {
			Text string `json:"text"`
			Next *int   `json:"next_offset"`
		}
		json.Unmarshal([]byte(result.Content[0].Text), &page)
		recalled.WriteString(page.Text)
		if page.Next == nil {
			break
		}
		offset = *page.Next
	}
	if recalled.String() != text {
		t.Fatal("recall changed original bytes")
	}
	r, err := ReadReport(e.Root, time.Now())
	if err != nil || r.Packed != 1 || r.Recalls < 2 || r.RecallTraffic == 0 || r.Original != int64(len(raw)) || r.Delivered != int64(len(got)) {
		t.Fatal(r, err)
	}
	if _, err = e.Recall("../../secrets", 0, 100); err == nil {
		t.Fatal("traversal")
	}
	if _, err = e.Recall(digest(text), 1, 100); err == nil {
		t.Fatal("broken UTF-8 offset")
	}
	os.WriteFile(filepath.Join(e.Root, "archives", digest(text)+".txt"), []byte("tampered"), 0600)
	if _, err = e.Recall(digest(text), 0, 100); err == nil {
		t.Fatal("integrity not checked")
	}
}
func TestPreserveSemanticsAndFailOpen(t *testing.T) {
	e := engine(t)
	raw := textResult(strings.Repeat("data", 4000))
	for _, test := range []struct {
		tool string
		hint bool
		body json.RawMessage
	}{
		{"write", true, raw}, {"search", false, raw},
		{"search", true, textResult("small")},
		{"search", true, json.RawMessage(`{"isError":true,"content":[{"type":"text","text":"error"}]}`)},
		{"search", true, json.RawMessage(`{"structuredContent":{"secret":"keep"},"content":[{"type":"text","text":"text"}]}`)},
	} {
		if !bytes.Equal(e.Transform(test.tool, test.hint, test.body), test.body) {
			t.Fatal("changed ineligible result")
		}
	}
	// Journal failure must return the original even after the archive is saved.
	os.Mkdir(filepath.Join(e.Root, "events.jsonl"), 0700)
	if !bytes.Equal(e.Transform("search", true, raw), raw) {
		t.Fatal("unrecorded transformation")
	}
}
func TestExpiryStopRestartAndSatisfaction(t *testing.T) {
	e := engine(t)
	if err := Rate(e.Root, true, 4); err != nil {
		t.Fatal(err)
	}
	raw := textResult(strings.Repeat("z", 20000))
	if bytes.Equal(e.Transform("search", true, raw), raw) {
		t.Fatal("not active")
	}
	if err := Rate(e.Root, true, 5); err == nil {
		t.Fatal("retroactive baseline")
	}
	if err := Rate(e.Root, false, 2); err != nil {
		t.Fatal(err)
	}
	e.Now = func() time.Time { return e.Trial.Expires }
	if !bytes.Equal(e.Transform("search", true, raw), raw) {
		t.Fatal("expiry not enforced")
	}
	e.Now = time.Now
	if err := Stop(e.Root); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(e.Transform("search", true, raw), raw) {
		t.Fatal("stop ignored")
	}
	if _, err := e.Recall(digest(strings.Repeat("z", 20000)), 0, 100); err != nil {
		t.Fatal("undo broke recall", err)
	}
	restarted, err := Open(e.Root, []string{"search"}, []string{"synthetic-upstream"}, time.Now(), 24*time.Hour)
	if err != nil || !restarted.Trial.Expires.Equal(e.Trial.Expires) {
		t.Fatal("restart extended trial", err)
	}
	if _, err := Open(e.Root, []string{"write"}, []string{"synthetic-upstream"}, time.Now(), time.Hour); err == nil {
		t.Fatal("scope drift")
	}
	r, err := ReadReport(e.Root, time.Now())
	if err != nil || r.BeforeRating != 4 || r.AfterRating != 2 || !strings.HasPrefix(r.Status, "Stopped") || r.Packed != 1 || len(r.Hours) != 24 {
		t.Fatal(r, err)
	}
}

func TestUpstreamProcess(t *testing.T) {
	if os.Getenv("TRACEFRUGAL_TEST_UPSTREAM") != "1" {
		return
	}
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for {
		var m message
		if decoder.Decode(&m) != nil {
			os.Exit(0)
		}
		if len(m.ID) == 0 {
			continue
		}
		var result any
		switch m.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]string{"name": "synthetic", "version": "1"}}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{"name": "search", "annotations": map[string]bool{"readOnlyHint": true}, "inputSchema": map[string]string{"type": "object"}}}}
		case "tools/call":
			result = json.RawMessage(textResult(strings.Repeat("SYNTHETIC_", 2000)))
		default:
			result = map[string]any{}
		}
		b, _ := json.Marshal(result)
		encoder.Encode(message{JSONRPC: "2.0", ID: m.ID, Result: b})
	}
}
func TestRealStdioProxyHandshakePackRecall(t *testing.T) {
	t.Setenv("TRACEFRUGAL_TEST_UPSTREAM", "1")
	e := engine(t)
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer inW.Close()
	defer outR.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		err := Run(ctx, inR, outW, io.Discard, e, []string{os.Args[0], "-test.run=^TestUpstreamProcess$"})
		outW.CloseWithError(err)
		done <- err
	}()
	enc, dec := json.NewEncoder(inW), json.NewDecoder(outR)
	call := func(id int, method string, params any) message {
		t.Helper()
		if err := enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
			t.Fatal(err)
		}
		var reply message
		if err := dec.Decode(&reply); err != nil {
			t.Fatal(err)
		}
		return reply
	}
	if !bytes.Contains(call(1, "initialize", map[string]any{}).Result, []byte("synthetic")) {
		t.Fatal("handshake")
	}
	if !bytes.Contains(call(2, "tools/list", map[string]any{}).Result, []byte(RecallTool)) {
		t.Fatal("recall not advertised")
	}
	got := call(3, "tools/call", map[string]any{"name": "search", "arguments": map[string]any{}})
	if !bytes.Contains(got.Result, []byte("archived")) {
		t.Fatal("real tool result not transformed")
	}
	recall := call(4, "tools/call", map[string]any{"name": RecallTool, "arguments": map[string]any{"handle": digest(strings.Repeat("SYNTHETIC_", 2000)), "limit": 100}})
	if !bytes.Contains(recall.Result, []byte("next_offset")) {
		t.Fatal("recall protocol")
	}
	inW.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	r, err := ReadReport(e.Root, time.Now())
	if err != nil || r.Packed != 1 || r.Recalls != 1 {
		t.Fatal(r, err)
	}
}
