// Package pack implements opt-in MCP result archiving. It does not rewrite a
// host's history, remove schemas, estimate token savings, or invoke an LLM.
package pack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const RecallTool = "tracefrugal_recall"
const Threshold = 10 << 10
const PreviewBytes = 1024
const RecallBytes = 16 << 10
const MaxResultBytes = 16 << 20

type Trial struct {
	Version     int       `json:"version"`
	Started     time.Time `json:"started"`
	Expires     time.Time `json:"expires"`
	Allow       []string  `json:"allow"`
	CommandHash string    `json:"command_hash"`
}
type Event struct {
	Time      time.Time `json:"time"`
	Kind      string    `json:"kind"`
	Tool      string    `json:"tool,omitempty"`
	Handle    string    `json:"handle,omitempty"`
	Original  int       `json:"original_bytes"`
	Delivered int       `json:"delivered_bytes"`
}
type Hour struct {
	Start     time.Time `json:"start"`
	Packed    int       `json:"packed"`
	Recalls   int       `json:"recalls"`
	Original  int64     `json:"original_bytes"`
	Delivered int64     `json:"delivered_bytes"`
}
type Report struct {
	Trial
	Status        string  `json:"status"`
	Activation    string  `json:"activation"`
	Packed        int     `json:"packed"`
	Recalls       int     `json:"recalls"`
	Original      int64   `json:"original_bytes"`
	Delivered     int64   `json:"delivered_bytes"`
	RecallTraffic int64   `json:"recall_bytes"`
	BeforeRating  int     `json:"before_rating"`
	AfterRating   int     `json:"after_rating"`
	Hours         []Hour  `json:"hours"`
	Events        []Event `json:"events"`
}
type Engine struct {
	mu    sync.Mutex
	Root  string
	Trial Trial
	Now   func() time.Time
}

func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

// safeDir refuses symlink components and uses private newly-created directories.
// Other processes running as the same user are outside this security boundary.
func safeDir(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	p := absolute
	for {
		info, e := os.Lstat(p)
		if e == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return errors.New("state path must contain only directories, not links")
		}
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return os.MkdirAll(absolute, 0700)
}
func regular(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("expected regular private state file")
	}
	return nil
}
func create(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return err
}
func Open(root string, allow []string, command []string, now time.Time, duration time.Duration) (*Engine, error) {
	if root == "" || len(allow) == 0 || len(command) == 0 || duration <= 0 || duration > 24*time.Hour {
		return nil, errors.New("state, allowlisted tools, command and a duration up to 24h are required")
	}
	for _, name := range allow {
		if name == "" || name == RecallTool {
			return nil, errors.New("invalid allowlisted tool")
		}
	}
	sort.Strings(allow)
	if err := safeDir(root); err != nil {
		return nil, err
	}
	if err := safeDir(filepath.Join(root, "archives")); err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(command)
	trial := Trial{Version: 1, Started: now, Expires: now.Add(duration), Allow: allow, CommandHash: digest(string(encoded))}
	path := filepath.Join(root, "trial.json")
	if err := regular(path); err == nil {
		b, err := os.ReadFile(path)
		var old Trial
		if err != nil || json.Unmarshal(b, &old) != nil || old.Version != 1 || old.Started.IsZero() || !old.Expires.After(old.Started) || old.Expires.Sub(old.Started) > 24*time.Hour {
			return nil, errors.New("invalid existing trial")
		}
		if old.CommandHash != trial.CommandHash || strings.Join(old.Allow, "\x00") != strings.Join(allow, "\x00") {
			return nil, errors.New("trial scope differs; use a new state directory")
		}
		trial = old // Restarting never extends the trial.
	} else if os.IsNotExist(err) {
		b, _ := json.Marshal(trial)
		if err = create(path, b); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	return &Engine{Root: root, Trial: trial, Now: time.Now}, nil
}
func (e *Engine) active() bool {
	now := e.Now()
	if now.Before(e.Trial.Started) || !now.Before(e.Trial.Expires) {
		return false
	}
	_, err := os.Lstat(filepath.Join(e.Root, "stopped"))
	return os.IsNotExist(err) // Unknown stop-marker state means do not transform.
}
func (e *Engine) append(event Event) error {
	path := filepath.Join(e.Root, "events.jsonl")
	if err := regular(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	b, _ := json.Marshal(event)
	b = append(b, '\n')
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

// Transform changes only pure-text, non-error results, after both an explicit
// allowlist and an upstream readOnlyHint. Structured/multimodal results pass
// through unchanged. The original remains available for paged exact recall.
func (e *Engine) Transform(tool string, readOnly bool, raw json.RawMessage) json.RawMessage {
	e.mu.Lock()
	defer e.mu.Unlock()
	allowed := false
	for _, name := range e.Trial.Allow {
		if tool == name {
			allowed = true
		}
	}
	if !e.active() || !allowed || !readOnly {
		return raw
	}
	var result map[string]json.RawMessage
	if json.Unmarshal(raw, &result) != nil {
		return raw
	}
	var isError bool
	if b, ok := result["isError"]; ok && (json.Unmarshal(b, &isError) != nil || isError) {
		return raw
	}
	// Preserve all non-content semantics; only the standard isError may coexist.
	for k := range result {
		if k != "content" && k != "isError" {
			return raw
		}
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(result["content"], &blocks) != nil || len(blocks) != 1 || blocks[0].Type != "text" {
		return raw
	}
	var content []map[string]json.RawMessage
	json.Unmarshal(result["content"], &content)
	for k := range content[0] {
		if k != "type" && k != "text" {
			return raw
		}
	}
	text := blocks[0].Text
	if len(text) <= Threshold || len(text) > MaxResultBytes || !utf8.ValidString(text) {
		return raw
	}
	handle := digest(text)
	path := filepath.Join(e.Root, "archives", handle+".txt")
	if err := create(path, []byte(text)); err != nil {
		if regular(path) != nil {
			return raw
		}
		old, err := os.ReadFile(path)
		if err != nil || digest(string(old)) != handle {
			return raw
		}
	}
	headEnd := PreviewBytes / 2
	for headEnd > 0 && !utf8.RuneStart(text[headEnd]) {
		headEnd--
	}
	tailStart := len(text) - PreviewBytes/2
	for tailStart < len(text) && !utf8.RuneStart(text[tailStart]) {
		tailStart++
	}
	preview := fmt.Sprintf("TraceFrugal archived this tool result (%d UTF-8 bytes; SHA-256 %s).\nThis excerpt is incomplete. Recall evidence before drawing conclusions that depend on omitted content.\nUse %s with handle=%s, offset=0; follow next_offset for the full original.\n--- BEGIN EXCERPT ---\n%s\n[... omitted ...]\n%s\n--- END EXCERPT ---", len(text), handle, RecallTool, handle, text[:headEnd], text[tailStart:])
	replacement, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": preview}}, "isError": false})
	event := Event{Time: e.Now(), Kind: "packed", Tool: tool, Handle: handle, Original: len(raw), Delivered: len(replacement)}
	if e.append(event) != nil {
		return raw
	} // No receipt, no claim of activation.
	return replacement
}
func (e *Engine) Recall(handle string, offset, limit int) (json.RawMessage, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(handle) != 64 {
		return nil, errors.New("invalid handle")
	}
	if _, err := hex.DecodeString(handle); err != nil || strings.ToLower(handle) != handle {
		return nil, errors.New("invalid handle")
	}
	if limit <= 0 {
		limit = RecallBytes
	}
	if limit > RecallBytes || offset < 0 {
		return nil, errors.New("invalid recall range")
	}
	path := filepath.Join(e.Root, "archives", handle+".txt")
	if regular(path) != nil {
		return nil, errors.New("archive not found")
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() > MaxResultBytes {
		return nil, errors.New("invalid archive")
	}
	b, err := os.ReadFile(path)
	if err != nil || digest(string(b)) != handle || !utf8.Valid(b) {
		return nil, errors.New("archive integrity check failed")
	}
	if offset > len(b) || offset < len(b) && !utf8.RuneStart(b[offset]) {
		return nil, errors.New("offset must be a UTF-8 boundary")
	}
	end := offset + limit
	if end > len(b) {
		end = len(b)
	}
	for end > offset && end < len(b) && !utf8.RuneStart(b[end]) {
		end--
	}
	if end == offset && offset < len(b) {
		return nil, errors.New("limit is too small for the next character")
	}
	var next *int
	if end < len(b) {
		next = &end
	}
	payload, _ := json.Marshal(map[string]any{"handle": handle, "offset": offset, "next_offset": next, "total_bytes": len(b), "text": string(b[offset:end]), "sha256": handle})
	out, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": string(payload)}}, "isError": false})
	if err := e.append(Event{Time: e.Now(), Kind: "recalled", Tool: RecallTool, Handle: handle, Delivered: len(out)}); err != nil {
		return nil, errors.New("cannot record recall receipt")
	}
	return out, nil
}

func Stop(root string) error {
	if err := safeDir(root); err != nil {
		return err
	}
	if err := regular(filepath.Join(root, "trial.json")); err != nil {
		return errors.New("trial not found")
	}
	path := filepath.Join(root, "stopped")
	if err := create(path, []byte(time.Now().UTC().Format(time.RFC3339Nano))); err != nil {
		if regular(path) == nil {
			return nil
		}
		return err
	}
	return nil
}
func Rate(root string, before bool, rating int) error {
	if rating < 1 || rating > 5 {
		return errors.New("choose satisfaction from 1 to 5")
	}
	r, err := ReadReport(root, time.Now())
	if err != nil {
		return err
	}
	if err := safeDir(root); err != nil {
		return err
	}
	if before && r.Packed > 0 {
		return errors.New("baseline rating must be recorded before the first packed result")
	}
	if !before && r.Packed == 0 {
		return errors.New("no observed transformations to rate")
	}
	name := "after-rating.json"
	if before {
		name = "before-rating.json"
	}
	// Ratings are append-only events so corrections retain their history.
	path := filepath.Join(root, name)
	if err := regular(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, _ := json.Marshal(map[string]any{"time": time.Now().UTC(), "rating": rating})
	_, err = f.Write(append(b, '\n'))
	return err
}
