// Package claude reads local Claude Code transcripts without retaining messages.
package claude

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/niceysam/tracefrugal/internal/ledger"
)

const maxLine = 16 * 1024 * 1024

// Request contains usage metadata only. Prompts, tool results, and credentials
// are discarded during decoding and never enter a snapshot.
type Request struct {
	ID         string        `json:"-"`
	Session    string        `json:"session"`
	Project    string        `json:"project"`
	Model      string        `json:"model"`
	Time       time.Time     `json:"time"`
	Tokens     ledger.Tokens `json:"tokens"`
	Cost       *float64      `json:"cost"`
	Effort     string        `json:"effort,omitempty"`
	Version    string        `json:"-"`
	Subagent   bool          `json:"subagent"`
	Source     string        `json:"source,omitempty"`
	Harness    string        `json:"harness,omitempty"`
	Reasoning  int64         `json:"reasoning_output,omitempty"` // Subset of Output, never added to it.
	Accounting string        `json:"accounting,omitempty"`
}

type Health struct {
	Files      int  `json:"files"`
	Duplicates int  `json:"duplicates"`
	Invalid    int  `json:"invalid"`
	Partial    int  `json:"partial"`
	Unreadable int  `json:"unreadable"`
	Conflicts  int  `json:"conflicts"`
	Missing    bool `json:"missing"`
}

type cachedFile struct {
	size     int64
	mod      time.Time
	requests []Request
	evidence []Evidence
	health   Health
}

type Reader struct {
	mu    sync.Mutex
	files map[string]cachedFile
}

func hash(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])[:16]
}

func (r *Reader) Read(root string) ([]Request, Health, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.files == nil {
		r.files = make(map[string]cachedFile)
	}
	projects := filepath.Join(root, "projects")
	info, statErr := os.Lstat(projects)
	if errors.Is(statErr, os.ErrNotExist) {
		r.files = make(map[string]cachedFile)
		return []Request{}, Health{Missing: true}, nil
	}
	if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		r.files = make(map[string]cachedFile)
		return []Request{}, Health{Unreadable: 1}, nil
	}
	h := Health{}
	present := map[string]bool{}
	unique := map[string]Request{}
	err := filepath.WalkDir(projects, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			h.Unreadable++
			return nil
		}
		// Never follow transcript symlinks or scan spilled tool output.
		if d.IsDir() {
			if d.Name() == "tool-results" || d.Name() == "memory" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(path, ".jsonl") || strings.Contains(d.Name(), ".orphaned-") {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			h.Unreadable++
			return nil
		}
		h.Files++
		present[path] = true
		c, ok := r.files[path]
		if !ok || c.size != info.Size() || c.mod != info.ModTime() || c.health.Unreadable > 0 {
			c = readFile(path)
			c.size, c.mod = info.Size(), info.ModTime()
			r.files[path] = c
		}
		h.Invalid += c.health.Invalid
		h.Partial += c.health.Partial
		h.Unreadable += c.health.Unreadable
		for _, q := range c.requests {
			old, exists := unique[q.ID]
			if !exists {
				unique[q.ID] = q
				continue
			}
			h.Duplicates++
			if old.Model != q.Model || old.Tokens.Input != q.Tokens.Input || old.Tokens.CachedInput != q.Tokens.CachedInput || old.Tokens.CacheWrite != q.Tokens.CacheWrite || old.Tokens.CacheWrite1h != q.Tokens.CacheWrite1h {
				h.Conflicts++
			}
			// Transcript blocks repeat cumulative usage for a response. Retain
			// the greatest output snapshot, never add the blocks together.
			best := old
			if q.Tokens.Output > old.Tokens.Output {
				best = q
			}
			// Copied/forked transcripts belong to the earliest observed owner.
			if q.Time.Before(old.Time) {
				best.Time, best.Session, best.Project, best.Subagent = q.Time, q.Session, q.Project, q.Subagent
			} else {
				best.Time, best.Session, best.Project, best.Subagent = old.Time, old.Session, old.Project, old.Subagent
			}
			unique[q.ID] = best
		}
		return nil
	})
	for path := range r.files {
		if !present[path] {
			delete(r.files, path)
		}
	}
	requests := make([]Request, 0, len(unique))
	for _, q := range unique {
		requests = append(requests, q)
	}
	sort.Slice(requests, func(i, j int) bool {
		if requests[i].Time.Equal(requests[j].Time) {
			return requests[i].ID < requests[j].ID
		}
		return requests[i].Time.Before(requests[j].Time)
	})
	return requests, h, err
}

func readFile(path string) cachedFile {
	c := cachedFile{}
	f, err := os.Open(path)
	if err != nil {
		c.health.Unreadable++
		return c
	}
	defer f.Close()
	reader := bufio.NewReaderSize(f, 64*1024)
	for {
		var line []byte
		oversized := false
		for {
			part, e := reader.ReadSlice('\n')
			if len(line)+len(part) <= maxLine && !oversized {
				line = append(line, part...)
			} else {
				oversized = true
			}
			if e == bufio.ErrBufferFull {
				continue
			}
			err = e
			break
		}
		if oversized {
			c.health.Invalid++
		} else if len(bytes.TrimSpace(line)) != 0 {
			c.evidence = append(c.evidence, evidence(line, strings.Contains(filepath.ToSlash(path), "/subagents/"))...)
			q, ignored, parseErr := parse(line, path)
			if parseErr != nil {
				if err == io.EOF {
					c.health.Partial++
				} else {
					c.health.Invalid++
				}
			} else if !ignored {
				c.requests = append(c.requests, q)
			}
		}
		if err != nil {
			if err != io.EOF {
				c.health.Unreadable++
			}
			break
		}
	}
	return c
}

func parse(line []byte, path string) (Request, bool, error) {
	var row struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		Session   string `json:"sessionId"`
		CWD       string `json:"cwd"`
		Effort    string `json:"effort"`
		Version   string `json:"version"`
		Message   struct {
			ID    string `json:"id"`
			Model string `json:"model"`
			Usage *struct {
				Input    *int64 `json:"input_tokens"`
				Output   *int64 `json:"output_tokens"`
				Read     int64  `json:"cache_read_input_tokens"`
				Write    int64  `json:"cache_creation_input_tokens"`
				Creation *struct {
					Five *int64 `json:"ephemeral_5m_input_tokens"`
					Hour *int64 `json:"ephemeral_1h_input_tokens"`
				} `json:"cache_creation"`
				Speed string `json:"speed"`
				Geo   string `json:"inference_geo"`
			} `json:"usage"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &row); err != nil {
		return Request{}, false, err
	}
	u := row.Message.Usage
	if row.Type != "assistant" || u == nil || row.Message.Model == "<synthetic>" {
		return Request{}, true, nil
	}
	bad := errors.New("invalid usage metadata")
	when, err := time.Parse(time.RFC3339Nano, row.Timestamp)
	if err != nil || row.Session == "" || row.Message.ID == "" || row.Message.Model == "" || u.Input == nil || u.Output == nil {
		return Request{}, false, bad
	}
	tokens := ledger.Tokens{Input: *u.Input, Output: *u.Output, CachedInput: u.Read, CacheWrite: u.Write}
	ttlKnown := u.Write == 0
	if u.Creation != nil && u.Creation.Five != nil && u.Creation.Hour != nil {
		tokens.CacheWrite, tokens.CacheWrite1h = *u.Creation.Five, *u.Creation.Hour
		ttlKnown = true
		if u.Write != tokens.CacheWrite+tokens.CacheWrite1h {
			return Request{}, false, bad
		}
	}
	for _, n := range []int64{tokens.Input, tokens.Output, tokens.CachedInput, tokens.CacheWrite, tokens.CacheWrite1h} {
		// Keep sums exactly representable in JavaScript and reject corrupt data.
		if n < 0 || n > 1_000_000_000 {
			return Request{}, false, bad
		}
	}
	if total(tokens) == 0 {
		return Request{}, true, nil
	}
	project := filepath.Base(row.CWD)
	if project == "." || project == string(filepath.Separator) || len(project) > 100 {
		project = "Project " + hash(filepath.Dir(path))[:6]
	}
	return Request{
		ID: hash(row.Message.ID), Session: hash(row.Session), Project: project,
		Model: row.Message.Model, Time: when, Tokens: tokens,
		Cost:   price(row.Message.Model, tokens, ttlKnown, u.Speed, u.Geo),
		Effort: row.Effort, Version: row.Version,
		Subagent: strings.Contains(filepath.ToSlash(path), "/subagents/"),
	}, false, nil
}

func total(t ledger.Tokens) int64 {
	return t.Input + t.CachedInput + t.CacheWrite + t.CacheWrite1h + t.Output
}
