// Package codex imports local Codex usage. It never exports transcript content.
// Codex logs are versioned implementation details, not a stable billing API.
package codex

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/niceysam/tracefrugal/internal/claude"
	"github.com/niceysam/tracefrugal/internal/ledger"
)

const maxLine = 16 << 20

type Health struct {
	claude.Health
	Legacy int `json:"legacy_requests"`
	Gaps   int `json:"unresolved_usage_events"`
}

type usage struct {
	Input     *int64 `json:"input_tokens"`
	Cached    int64  `json:"cached_input_tokens"`
	Write     int64  `json:"cache_write_input_tokens"`
	Output    *int64 `json:"output_tokens"`
	Reasoning int64  `json:"reasoning_output_tokens"`
	Total     *int64 `json:"total_tokens"`
}

func (u usage) valid() bool {
	if u.Input == nil || u.Output == nil || u.Total == nil {
		return false
	}
	for _, n := range []int64{*u.Input, *u.Output, *u.Total, u.Cached, u.Write, u.Reasoning} {
		if n < 0 || n > 1_000_000_000_000 {
			return false
		}
	}
	return *u.Total == *u.Input+*u.Output && u.Cached+u.Write <= *u.Input && u.Reasoning <= *u.Output
}
func (u usage) values() [6]int64 {
	return [6]int64{*u.Input, u.Cached, u.Write, *u.Output, u.Reasoning, *u.Total}
}
func (u usage) tokens() ledger.Tokens {
	return ledger.Tokens{Input: *u.Input - u.Cached - u.Write, CachedInput: u.Cached, CacheWrite: u.Write, Output: *u.Output}
}
func hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:24]
}

type cached struct {
	size     int64
	mod      time.Time
	requests []claude.Request
	health   Health
}

// Reader is owned by a caller that serializes Read calls.
type Reader struct{ files map[string]cached }

func (r *Reader) Read(root string) ([]claude.Request, Health) {
	if r.files == nil {
		r.files = map[string]cached{}
	}
	h := Health{}
	seenFiles := map[string]bool{}
	unique := map[string]claude.Request{}
	found := false
	for _, name := range []string{"sessions", "archived_sessions"} {
		dir := filepath.Join(root, name)
		info, err := os.Lstat(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			h.Unreadable++
			continue
		}
		found = true
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				h.Unreadable++
				return nil
			}
			if d.IsDir() || d.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(d.Name(), ".jsonl") {
				return nil
			}
			info, err := d.Info()
			if err != nil || !info.Mode().IsRegular() {
				h.Unreadable++
				return nil
			}
			h.Files++
			seenFiles[path] = true
			c, ok := r.files[path]
			if !ok || c.size != info.Size() || !c.mod.Equal(info.ModTime()) || c.health.Unreadable > 0 {
				c = readFile(path)
				c.size, c.mod = info.Size(), info.ModTime()
				r.files[path] = c
			}
			h.Invalid += c.health.Invalid
			h.Partial += c.health.Partial
			h.Unreadable += c.health.Unreadable
			h.Gaps += c.health.Gaps
			for _, q := range c.requests {
				old, exists := unique[q.ID]
				if exists {
					h.Duplicates++
					if old.Tokens != q.Tokens || old.Reasoning != q.Reasoning {
						h.Conflicts++
					}
					// First chronological receipt wins; conflicting totals are flagged.
					if !q.Time.Before(old.Time) {
						continue
					}
				}
				unique[q.ID] = q
			}
			return nil
		})
	}
	h.Missing = !found
	for p := range r.files {
		if !seenFiles[p] {
			delete(r.files, p)
		}
	}
	out := make([]claude.Request, 0, len(unique))
	for _, q := range unique {
		out = append(out, q)
		if q.Accounting == "legacy_last_usage" {
			h.Legacy++
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Time.Equal(out[j].Time) {
			return out[i].ID < out[j].ID
		}
		return out[i].Time.Before(out[j].Time)
	})
	return out, h
}

func readFile(path string) cached {
	c := cached{}
	f, err := os.Open(path)
	if err != nil {
		c.health.Unreadable++
		return c
	}
	defer f.Close()
	reader := bufio.NewReaderSize(f, 64<<10)
	session, model, effort := "", "unknown", ""
	var legacy []claude.Request
	var previous *usage
	modern := false
	for {
		var line []byte
		oversized := false
		for {
			part, e := reader.ReadSlice('\n')
			if !oversized && len(line)+len(part) <= maxLine {
				line = append(line, part...)
			} else {
				oversized = true
			}
			err = e
			if e != bufio.ErrBufferFull {
				break
			}
		}
		if oversized {
			c.health.Invalid++
		} else if len(bytes.TrimSpace(line)) > 0 {
			var row struct {
				Type    string          `json:"type"`
				Time    time.Time       `json:"timestamp"`
				Payload json.RawMessage `json:"payload"`
			}
			if json.Unmarshal(line, &row) != nil {
				if err == io.EOF {
					c.health.Partial++
				} else {
					c.health.Invalid++
				}
			} else {
				switch row.Type {
				case "session_meta":
					var m struct {
						ID      string `json:"id"`
						Session string `json:"session_id"`
					}
					if json.Unmarshal(row.Payload, &m) == nil {
						session = m.ID
						if session == "" {
							session = m.Session
						}
					}
				case "turn_context":
					var m struct {
						Model  string `json:"model"`
						Effort string `json:"effort"`
					}
					if json.Unmarshal(row.Payload, &m) == nil {
						if m.Model != "" {
							model = m.Model
						}
						effort = m.Effort
					}
				case "token_usage_record":
					// A file with request receipts never also contributes token_count
					// snapshots. Mixing them would count the same call twice.
					modern = true
					var p struct {
						Response string `json:"response_id"`
						Thread   string `json:"thread_id"`
						Session  string `json:"session_id"`
						Usage    usage  `json:"usage"`
					}
					if json.Unmarshal(row.Payload, &p) != nil || p.Response == "" || row.Time.IsZero() || !p.Usage.valid() {
						c.health.Invalid++
						break
					}
					id := p.Thread
					if id == "" {
						id = session
					}
					if id == "" {
						id = p.Session
					}
					if id == "" {
						c.health.Gaps++
						break
					}
					c.requests = append(c.requests, request(hash(p.Response), id, model, effort, row.Time, p.Usage, "request_receipt"))
				case "event_msg":
					var p struct {
						Type string `json:"type"`
						Info *struct {
							Total usage `json:"total_token_usage"`
							Last  usage `json:"last_token_usage"`
						} `json:"info"`
					}
					if json.Unmarshal(row.Payload, &p) != nil || p.Type != "token_count" || p.Info == nil {
						break
					}
					u, last := p.Info.Total, p.Info.Last
					if !u.valid() || !last.valid() || row.Time.IsZero() || session == "" {
						c.health.Gaps++
						break
					}
					current := u.values()
					if previous != nil && previous.values() == current {
						break
					}
					ok := true
					if previous != nil {
						before, want := previous.values(), last.values()
						for i, n := range current {
							if n-before[i] != want[i] {
								ok = false
							}
						}
					}
					previous = &u
					if !ok {
						// Reset, carry-over, or missing events: establish a new
						// baseline, never guess how many requests were omitted.
						c.health.Gaps++
						break
					}
					if *last.Total > 0 {
						fingerprint, _ := json.Marshal(current)
						id := hash(session + ":" + row.Time.Format(time.RFC3339Nano) + ":" + string(fingerprint))
						legacy = append(legacy, request(id, session, model, effort, row.Time, last, "legacy_last_usage"))
					}
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				c.health.Unreadable++
			}
			break
		}
	}
	if modern {
		// Older events in a mixed-format file may predate request receipts.
		// Report that coverage gap instead of inventing an overlap boundary.
		if len(c.requests) > 0 {
			first := c.requests[0].Time
			for _, q := range legacy {
				if q.Time.Before(first) {
					c.health.Gaps++
				}
			}
		} else if len(legacy) > 0 {
			c.health.Gaps += len(legacy)
		}
	} else {
		c.requests = legacy
	}
	return c
}

func request(id, session, model, effort string, t time.Time, u usage, basis string) claude.Request {
	return claude.Request{ID: id, Session: hash(session), Project: "Codex session", Model: model, Effort: effort, Time: t, Tokens: u.tokens(), Reasoning: u.Reasoning, Accounting: basis, Harness: "codex"}
}
