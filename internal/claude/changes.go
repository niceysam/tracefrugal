package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Recipe struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
	Rule   string `json:"rule"`
}

// These are behavioral instructions, not a context compressor or a guarantee.
// No model, effort, permission, or MCP server configuration is changed.
var Recipes = []Recipe{
	{"tool-results", "Keep large tool results out of the conversation", "Useful when tool results are large. Retrieve enough evidence without replaying entire files or logs.",
		"Use narrow file ranges, query filters, requested fields, and result limits before fetching data. For large outputs, keep the full artifact locally and return the relevant excerpt, counts, errors, and its path. Start with a small relevant excerpt and expand when needed. Do not hide failures or evidence needed to verify the task. Reuse information already read unless it changed."},
	{"mcp", "Use MCP discovery and smaller result sets", "Useful when MCP responses dominate observed tool-output bytes. This does not measure schema overhead.",
		"When MCP tool discovery is available, discover only the tools needed for the current step. Prefer specific queries, fields, pagination, and result limits over broad listings. Reuse relevant results already available. Expand when the task requires more evidence. Do not disable servers, change permissions, or skip required checks to save tokens."},
	{"turns", "Avoid redundant tool round trips", "Useful when identical tool calls recur. Some retries and polling are legitimate.",
		"Batch independent reads when supported. Reuse prior results unless files or external state changed. Poll at meaningful intervals; avoid repeating unchanged broad searches. Finish once the requested completion criteria and required checks pass. Preserve necessary verification and approval steps. For a new unrelated task, offer a concise handoff and a fresh session; do not clear context automatically."},
}

type Observation struct {
	Start time.Time `json:"start"`
	Until time.Time `json:"until"`
	Summary
	Diagnostics Diagnostics `json:"diagnostics"`
}

type Change struct {
	ID           string        `json:"id"`
	Started      time.Time     `json:"started"`
	Finished     time.Time     `json:"finished,omitempty"`
	Status       string        `json:"status"`
	Recipe       string        `json:"recipe"`
	BeforeRating int           `json:"before_rating"`
	AfterRating  int           `json:"after_rating"`
	Baseline     Observation   `json:"baseline"`
	After        Observation   `json:"after"`
	Hours        []Observation `json:"hours"`
	RuleBytes    int           `json:"rule_bytes"`
	RuleState    string        `json:"rule_state"`
}

type privateChange struct {
	Change
	Root      string `json:"root"`
	Before    []byte `json:"before"`
	AfterRule []byte `json:"after_rule"`
	Existed   bool   `json:"existed"`
}

func atomic(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tracefrugal-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func locked(state string, fn func() error) error {
	if err := os.MkdirAll(state, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(state, ".lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("another trial operation is running; inspect state if a prior process crashed")
	}
	f.Close()
	defer os.Remove(filepath.Join(state, ".lock"))
	return fn()
}

func loadChanges(state string) ([]privateChange, error) {
	b, err := os.ReadFile(filepath.Join(state, "changes.json"))
	if errors.Is(err, os.ErrNotExist) {
		return []privateChange{}, nil
	}
	if err != nil {
		return nil, err
	}
	var changes []privateChange
	err = json.Unmarshal(b, &changes)
	return changes, err
}

func saveChanges(state string, changes []privateChange) error {
	b, err := json.MarshalIndent(changes, "", "  ")
	if err != nil {
		return err
	}
	return atomic(filepath.Join(state, "changes.json"), b, 0600)
}

func ruleFile(root string) string { return filepath.Join(root, "rules", "tracefrugal-context.md") }

func readRule(root string) ([]byte, bool, error) {
	// Do not follow a rules-directory symlink to an unexpected write target.
	info, err := os.Lstat(filepath.Join(root, "rules"))
	if err == nil && !info.IsDir() {
		return nil, false, errors.New("rules directory is not a regular directory")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	path := ruleFile(root)
	info, err = os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64*1024 {
		return nil, false, errors.New("rule must be a regular small file")
	}
	b, err := os.ReadFile(path)
	return b, true, err
}

func Window(requests []Request, events []Evidence, from, until time.Time) Observation {
	o := Observation{Start: from, Until: until}
	for _, q := range requests {
		if !q.Time.Before(from) && q.Time.Before(until) {
			o.Add(q)
		}
	}
	o.Diagnostics = Diagnose(events, o.Summary, from, until)
	return o
}

// Sum saved hours plus the current partial hour, so log retention cannot erase
// completed observations while a trial is still running.
func combined(hours []Observation, partial Observation, from, until time.Time) Observation {
	o := Observation{Start: from, Until: until}
	tools := map[string]Tool{}
	for _, h := range append(append([]Observation{}, hours...), partial) {
		o.Requests += h.Requests
		o.Tokens.Input += h.Tokens.Input
		o.Tokens.CachedInput += h.Tokens.CachedInput
		o.Tokens.CacheWrite += h.Tokens.CacheWrite
		o.Tokens.CacheWrite1h += h.Tokens.CacheWrite1h
		o.Tokens.Output += h.Tokens.Output
		o.KnownUSD += h.KnownUSD
		o.Unpriced += h.Unpriced
		o.Diagnostics.HumanTurns += h.Diagnostics.HumanTurns
		o.Diagnostics.ToolCalls += h.Diagnostics.ToolCalls
		o.Diagnostics.ResultBytes += h.Diagnostics.ResultBytes
		o.Diagnostics.MCPBytes += h.Diagnostics.MCPBytes
		o.Diagnostics.LargeResults += h.Diagnostics.LargeResults
		o.Diagnostics.RepeatedCalls += h.Diagnostics.RepeatedCalls
		for _, tool := range h.Diagnostics.Tools {
			t := tools[tool.Name]
			t.Name = tool.Name
			t.Calls += tool.Calls
			t.Repeated += tool.Repeated
			t.Bytes += tool.Bytes
			if tool.Largest > t.Largest {
				t.Largest = tool.Largest
			}
			tools[t.Name] = t
		}
	}
	o.Diagnostics.Tools = []Tool{}
	for _, t := range tools {
		o.Diagnostics.Tools = append(o.Diagnostics.Tools, t)
	}
	sort.Slice(o.Diagnostics.Tools, func(i, j int) bool { return o.Diagnostics.Tools[i].Name < o.Diagnostics.Tools[j].Name })
	if o.Requests > 0 {
		o.Diagnostics.AvgInput = float64(total(o.Tokens)-o.Tokens.Output) / float64(o.Requests)
		o.Diagnostics.AvgOutput = float64(o.Tokens.Output) / float64(o.Requests)
	}
	return o
}

func Apply(root, state, recipe string, rating int, requests []Request, events []Evidence, now time.Time) error {
	return locked(state, func() error {
		if rating < 1 || rating > 5 {
			return errors.New("rate your current answer satisfaction from 1 to 5")
		}
		var selected *Recipe
		for i := range Recipes {
			if Recipes[i].ID == recipe {
				selected = &Recipes[i]
			}
		}
		if selected == nil {
			return errors.New("unknown context recipe")
		}
		baseline := Window(requests, events, now.Add(-24*time.Hour), now)
		if baseline.Requests == 0 {
			return errors.New("record some usage before starting a comparison")
		}
		changes, err := loadChanges(state)
		if err != nil {
			return err
		}
		for _, c := range changes {
			if c.Root != hash(root) || c.Status == "saved" || c.Status == "prepared" {
				return errors.New("finish or restore the current trial before another one")
			}
		}
		before, existed, err := readRule(root)
		if err != nil {
			return err
		}
		if existed {
			return errors.New("trial rule already exists; preserve it and resolve the previous trial first")
		}
		after := []byte("<!-- TraceFrugal context trial -->\n# Context discipline\n\n" + selected.Rule + "\n")
		c := privateChange{Change: Change{ID: now.UTC().Format("20060102T150405.000000000Z"), Started: now, Status: "prepared", Recipe: recipe, BeforeRating: rating, Baseline: baseline, Hours: []Observation{}, RuleBytes: len(after)}, Root: hash(root), Before: before, AfterRule: after, Existed: existed}
		changes = append(changes, c)
		if err := saveChanges(state, changes); err != nil {
			return err
		}
		current, exists, err := readRule(root)
		if err != nil || exists != existed || !bytes.Equal(current, before) {
			return errors.New("rule changed during preparation")
		}
		if err := os.MkdirAll(filepath.Dir(ruleFile(root)), 0700); err != nil {
			return err
		}
		// Exclusive creation cannot overwrite another writer's new file.
		f, err := os.OpenFile(ruleFile(root), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(after)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		changes[len(changes)-1].Status = "saved"
		return saveChanges(state, changes)
	})
}

func Restore(root, state, id string, now time.Time) error {
	return locked(state, func() error {
		changes, err := loadChanges(state)
		if err != nil || len(changes) == 0 {
			return errors.New("no trial to restore")
		}
		c := &changes[len(changes)-1]
		if c.ID != id || c.Root != hash(root) || (c.Status != "saved" && c.Status != "prepared") {
			return errors.New("trial changed")
		}
		current, exists, err := readRule(root)
		if err != nil {
			return err
		}
		alreadyBefore := exists == c.Existed && bytes.Equal(current, c.Before)
		if !alreadyBefore && (!exists || !bytes.Equal(current, c.AfterRule)) {
			return errors.New("rule was edited elsewhere; restore refused")
		}
		if !alreadyBefore {
			if c.Existed {
				err = atomic(ruleFile(root), c.Before, 0600)
			} else {
				err = os.Remove(ruleFile(root))
			}
			if err != nil {
				return err
			}
		}
		c.Status, c.Finished = "restored", now
		return saveChanges(state, changes)
	})
}

func Rate(root, state, id string, rating int) error {
	return locked(state, func() error {
		if rating < 1 || rating > 5 {
			return errors.New("rating must be 1 to 5")
		}
		changes, err := loadChanges(state)
		if err != nil {
			return err
		}
		for i := range changes {
			if changes[i].ID == id && changes[i].Root == hash(root) {
				if changes[i].After.Requests == 0 {
					return errors.New("record usage after the change before rating it")
				}
				changes[i].AfterRating = rating
				return saveChanges(state, changes)
			}
		}
		return errors.New("trial not found")
	})
}

func Observe(root, state string, requests []Request, events []Evidence, now time.Time) ([]Change, error) {
	public := []Change{}
	err := locked(state, func() error {
		changes, err := loadChanges(state)
		if err != nil {
			return err
		}
		dirty := false
		for i := range changes {
			c := &changes[i]
			if c.Root != hash(root) {
				return errors.New("state belongs to another Claude directory")
			}
			end := now
			if !c.Finished.IsZero() && c.Finished.Before(end) {
				end = c.Finished
			}
			if limit := c.Started.Add(24 * time.Hour); end.After(limit) {
				end = limit
			}
			if end.After(c.After.Until) {
				for h := len(c.Hours); h < 24; h++ {
					start := c.Started.Add(time.Duration(h) * time.Hour)
					if start.Add(time.Hour).After(end) {
						break
					}
					c.Hours = append(c.Hours, Window(requests, events, start, start.Add(time.Hour)))
				}
				partialStart := c.Started.Add(time.Duration(len(c.Hours)) * time.Hour)
				c.After = combined(c.Hours, Window(requests, events, partialStart, end), c.Started, end)
				dirty = true
			}
			copy := c.Change
			if c.Status == "saved" || c.Status == "prepared" {
				current, exists, err := readRule(root)
				switch {
				case err != nil:
					copy.RuleState = "unreadable"
				case !exists:
					copy.RuleState = "missing"
				case !bytes.Equal(current, c.AfterRule):
					copy.RuleState = "edited"
				default:
					copy.RuleState = "present"
				}
			}
			public = append(public, copy)
		}
		if dirty {
			return saveChanges(state, changes)
		}
		return nil
	})
	return public, err
}
