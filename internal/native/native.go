// Package native aggregates explicitly scoped local harness stores.
package native

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/niceysam/tracefrugal/internal/claude"
	"github.com/niceysam/tracefrugal/internal/codex"
	"github.com/niceysam/tracefrugal/internal/pack"
)

type Source struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Harness string `json:"harness"`
	Root    string `json:"-"` // Paths never leave the local collector.
}
type Coverage struct {
	Source
	Summary         claude.Summary `json:"summary"`
	Health          claude.Health  `json:"health"`
	Latest          *time.Time     `json:"latest"`
	Status          string         `json:"status"`
	Legacy          int            `json:"legacy_requests"`
	Gaps            int            `json:"unresolved_usage_events"`
	CrossDuplicates int            `json:"cross_source_duplicates"`
	ToolEvidence    bool           `json:"tool_evidence"`
	Activation      string         `json:"activation"`
}
type Recommendation struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Evidence   string `json:"evidence"`
	Action     string `json:"action"`
	Verify     string `json:"verify"`
	Confidence string `json:"confidence"`
}
type Report struct {
	claude.Report
	Sources         []Coverage       `json:"sources"`
	Selected        string           `json:"selected_source"`
	Recommendations []Recommendation `json:"recommendations"`
	EvidenceScope   string           `json:"evidence_scope"`
	Quality         string           `json:"quality_status"`
	Packing         *pack.Report     `json:"packing,omitempty"`
}
type Reader struct {
	claude map[string]*claude.Reader
	codex  map[string]*codex.Reader
}

func key(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}

// Discover reads only directory names and the two caller-provided environment
// overrides. Shell aliases, launchers, credentials and settings are not parsed.
// Explicit roots replace auto-discovery so an audit can be narrowly scoped.
func Discover(home, claudeEnv, codexEnv string, explicit []Source) ([]Source, error) {
	candidates := append([]Source{}, explicit...)
	if len(explicit) == 0 {
		for _, h := range []string{"claude", "codex"} {
			base := filepath.Join(home, "."+h)
			candidates = append(candidates, Source{Harness: h, Root: base})
			matches, _ := filepath.Glob(base + "-*")
			for _, p := range matches {
				if info, err := os.Stat(p); err == nil && info.IsDir() {
					candidates = append(candidates, Source{Harness: h, Root: p})
				}
			}
		}
		if claudeEnv != "" {
			candidates = append(candidates, Source{Harness: "claude", Root: claudeEnv})
		}
		if codexEnv != "" {
			candidates = append(candidates, Source{Harness: "codex", Root: codexEnv})
		}
	}
	seen := map[string]bool{}
	out := []Source{}
	for _, s := range candidates {
		if s.Harness != "claude" && s.Harness != "codex" || strings.TrimSpace(s.Root) == "" {
			return nil, errors.New("sources require a harness (claude or codex) and a directory")
		}
		root, err := filepath.Abs(s.Root)
		if err != nil {
			return nil, err
		}
		if resolved, e := filepath.EvalSymlinks(root); e == nil {
			root = resolved
		}
		id := key(s.Harness + ":" + root)
		if seen[id] {
			continue
		}
		seen[id] = true
		s.ID, s.Root = id, root
		if s.Name == "" {
			s.Name = filepath.Base(root)
		}
		out = append(out, s)
	}
	return out, nil
}

// Read returns global, de-duplicated request totals. The first listed source
// owns copied receipts. A filtered report follows that same attribution.
// Evidence currently comes from Claude logs only and is labeled accordingly.
func (r *Reader) Read(sources []Source, selected string, from, until time.Time) (Report, error) {
	if r.claude == nil {
		r.claude = map[string]*claude.Reader{}
		r.codex = map[string]*codex.Reader{}
	}
	result := Report{Sources: []Coverage{}, Selected: selected, Quality: "Not measured. Reasoning tokens are output usage, not an answer-quality score.", EvidenceScope: "Claude Code transcript tool events only. Codex tool payloads and actual MCP schema tokens are not attributed."}
	requests := []claude.Request{}
	events := []claude.Evidence{}
	unique := map[string]claude.Request{}
	evidenceIDs := map[string]bool{}
	found := selected == ""
	health := claude.Health{}
	for _, source := range sources {
		c := Coverage{Source: source, Activation: "Usage only", ToolEvidence: source.Harness == "claude"}
		var batch []claude.Request
		var evidence []claude.Evidence
		if source.Harness == "claude" {
			reader := r.claude[source.ID]
			if reader == nil {
				reader = &claude.Reader{}
				r.claude[source.ID] = reader
			}
			var err error
			batch, c.Health, err = reader.Read(source.Root)
			if err != nil {
				c.Health.Unreadable++
			}
			evidence = reader.Evidence()
			c.Activation = "Advisory rules; loading and compliance unverified"
		} else {
			reader := r.codex[source.ID]
			if reader == nil {
				reader = &codex.Reader{}
				r.codex[source.ID] = reader
			}
			var h codex.Health
			batch, h = reader.Read(source.Root)
			c.Health, c.Gaps = h.Health, h.Gaps
		}
		if selected == source.ID {
			found = true
		}
		for _, q := range batch {
			if c.Latest == nil || q.Time.After(*c.Latest) {
				t := q.Time
				c.Latest = &t
			}
			id := source.Harness + ":" + q.ID
			if old, ok := unique[id]; ok {
				c.CrossDuplicates++
				if old.Tokens != q.Tokens || old.Reasoning != q.Reasoning {
					c.Health.Conflicts++
				}
				continue
			}
			unique[id] = q
			q.ID = id
			q.Source = source.ID
			q.Harness = source.Harness
			q.Session = source.Harness + ":" + q.Session
			if !q.Time.Before(from) && q.Time.Before(until) {
				c.Summary.Add(q)
				if q.Accounting == "legacy_last_usage" {
					c.Legacy++
				}
			}
			if selected == "" || selected == source.ID {
				requests = append(requests, q)
			}
		}
		for _, e := range evidence {
			if evidenceIDs[e.ID] {
				continue
			}
			evidenceIDs[e.ID] = true
			if selected == "" || selected == source.ID {
				events = append(events, e)
			}
		}
		c.Status = "Observed"
		if c.Summary.Requests == 0 {
			c.Status = "No usage in this period"
		}
		if c.Health.Missing {
			c.Status = "Store not found"
		}
		if c.Gaps > 0 || c.Health.Invalid > 0 || c.Health.Partial > 0 || c.Health.Conflicts > 0 {
			c.Status = "Partial coverage"
		}
		if c.Health.Unreadable > 0 {
			c.Status = "Read errors"
		}
		result.Sources = append(result.Sources, c)
		if selected == "" || selected == source.ID {
			health.Files += c.Health.Files
			health.Duplicates += c.Health.Duplicates + c.CrossDuplicates
			health.Invalid += c.Health.Invalid
			health.Partial += c.Health.Partial
			health.Unreadable += c.Health.Unreadable
			health.Conflicts += c.Health.Conflicts
		}
	}
	if !found {
		return result, errors.New("unknown source")
	}
	sort.Slice(requests, func(i, j int) bool {
		if requests[i].Time.Equal(requests[j].Time) {
			return requests[i].ID < requests[j].ID
		}
		return requests[i].Time.Before(requests[j].Time)
	})
	result.Report = claude.Build(requests, health, from, until)
	result.Mode = "native"
	result.Diagnostics = claude.Diagnose(events, result.Summary, from, until)
	result.Recipes = []claude.Recipe{}
	for i := range result.Sessions {
		if n := len(result.Sessions[i].Timeline); n > 200 {
			result.Sessions[i].Timeline = result.Sessions[i].Timeline[n-200:]
		}
	}
	result.Recommendations = recommend(result)
	// Replace provider-specific /clear or /context instructions on mixed reports.
	result.Findings = []claude.Finding{{Title: "Your sources are visible, not your launcher identities", Detail: "Stores shared by multiple launchers are counted once. A directory does not identify an account, subscription, or which shell shortcut launched a session.", Action: "Select a source above for a scoped graph. Use --claude-dir or --codex-dir for custom stores."}}
	return result, nil
}

func recommend(r Report) []Recommendation {
	out := []Recommendation{}
	s, d := r.Summary, r.Diagnostics
	input := s.Tokens.Input + s.Tokens.CachedInput + s.Tokens.CacheWrite + s.Tokens.CacheWrite1h
	if s.Requests > 0 && input/int64(s.Requests) >= 50_000 {
		out = append(out, Recommendation{ID: "handoff", Title: "Start unrelated work with a short handoff", Evidence: "Large average input is observed. Logs do not separate system instructions, schemas, files and conversation tokens.", Action: "Save the goal, decisions, relevant file paths and unfinished checks. Start a fresh session for an unrelated task; load details only when needed.", Verify: "Compare input per response on similar tasks and rate answer usefulness. A smaller input alone is not a successful outcome.", Confidence: "Observed usage; cause unverified"})
	}
	if input > 0 && float64(s.Tokens.CachedInput)/float64(input) > .8 {
		out = append(out, Recommendation{ID: "cache", Title: "Keep useful cache reuse; reduce what gets repeated", Evidence: "Over 80% of input came from cache. Reuse lowers input pricing; it does not remove context occupancy.", Action: "Keep stable instructions and tool ordering. Reduce oversized tool results and unused context first. Avoid repeatedly rewriting the prefix or clearing a useful ongoing session.", Verify: "Track new input, cache writes and cache reads separately. Compression can cause a cache rebuild before later requests benefit.", Confidence: "Observed cache ratio"})
	}
	if d.LargeResults > 0 {
		out = append(out, Recommendation{ID: "pack", Title: "Archive large read-only tool results", Evidence: "Large tool-result payloads are present in Claude logs. Payload bytes are measured; their token contribution is unknown.", Action: "Request fields, ranges and pages at the source. For an allowlisted read-only MCP tool, use the opt-in pack proxy to archive its full text and return an excerpt with an exact recall handle.", Verify: "Check archive/recall receipts and errors, then compare real usage and satisfaction over 24 hours. Byte reduction is not billed-token savings.", Confidence: "Observed payload sizes"})
	}
	if d.RepeatedCalls > 0 {
		out = append(out, Recommendation{ID: "roundtrips", Title: "Reuse results and batch independent reads", Evidence: "Identical tool arguments recur in a session. Some repeats may be necessary after state changes.", Action: "Reuse still-current results and batch independent reads. Combine steps only when later actions are already known; preserve checks, approvals and error handling.", Verify: "Compare tool calls per user turn for Claude sources, alongside task correctness and satisfaction.", Confidence: "Observed repeats; redundancy unverified"})
	}
	out = append(out, Recommendation{ID: "mcp", Title: "Load only the MCP tools needed for this task", Evidence: "Actual loaded tool schemas are not present in these usage counters. Connected tools are not proof of schema-token cost.", Action: "Use tool search or deferred discovery if your host supports it. Otherwise create a task-specific MCP profile after reviewing dependencies. The pack proxy changes tool results; it cannot unload the host's schemas or rewrite old context.", Verify: "Inspect the host's context breakdown before and after. Record schema tokens there; do not estimate them from the number of connectors.", Confidence: "Capability-dependent; schema attribution unavailable"})
	return out
}
