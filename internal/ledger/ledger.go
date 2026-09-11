// Package ledger validates request-level usage and computes reproducible cost reports.
package ledger

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
)

const MaxLineBytes = 4 * 1024 * 1024

// Tokens are disjoint billing categories, never session-to-date counters.
type Tokens struct {
	Input        int64 `json:"input"`
	CachedInput  int64 `json:"cached_input"`
	CacheWrite   int64 `json:"cache_write"`
	CacheWrite1h int64 `json:"cache_write_1h"`
	Output       int64 `json:"output"`
}

type Event struct {
	Type       string  `json:"type"`
	TaskID     string  `json:"task_id"`
	RequestID  string  `json:"request_id,omitempty"`
	Model      string  `json:"model,omitempty"`
	Tokens     *Tokens `json:"tokens,omitempty"`
	DurationMS *int64  `json:"duration_ms,omitempty"`
	Success    *bool   `json:"success,omitempty"`
}

type Rates struct {
	Input        *float64 `json:"input"`
	CachedInput  *float64 `json:"cached_input"`
	CacheWrite   *float64 `json:"cache_write"`
	CacheWrite1h *float64 `json:"cache_write_1h"`
	Output       *float64 `json:"output"`
}

type PriceBook struct {
	SchemaVersion int              `json:"schema_version"`
	Currency      string           `json:"currency"`
	Label         string           `json:"label"`
	Models        map[string]Rates `json:"models"`
}

type Task struct {
	ID            string  `json:"task_id"`
	Requests      int     `json:"requests"`
	CostUSD       float64 `json:"estimated_cost_usd"`
	Success       *bool   `json:"success"`
	DurationMS    int64   `json:"summed_request_duration_ms"`
	MeasuredCalls int     `json:"calls_with_duration"`
}

type Report struct {
	SchemaVersion  int      `json:"schema_version"`
	PriceLabel     string   `json:"price_label"`
	Requests       int      `json:"requests"`
	Tokens         Tokens   `json:"tokens"`
	CostUSD        float64  `json:"estimated_cost_usd"`
	Successes      int      `json:"successful_tasks"`
	Failures       int      `json:"failed_tasks"`
	Unknown        int      `json:"tasks_without_result"`
	CostPerSuccess *float64 `json:"estimated_cost_per_success_usd"`
	Tasks          []Task   `json:"tasks"`
}

func decodeStrict(data []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON value")
	}
	return nil
}

func ReadPrices(r io.Reader) (PriceBook, error) {
	var p PriceBook
	data, err := io.ReadAll(io.LimitReader(r, MaxLineBytes+1))
	if err != nil {
		return p, err
	}
	if len(data) > MaxLineBytes {
		return p, fmt.Errorf("price book exceeds %d bytes", MaxLineBytes)
	}
	if err := decodeStrict(data, &p); err != nil {
		return p, fmt.Errorf("price book: %w", err)
	}
	if p.SchemaVersion != 1 || p.Currency != "USD" || strings.TrimSpace(p.Label) == "" || len(p.Models) == 0 {
		return p, fmt.Errorf("price book requires schema_version=1, currency=USD, a label and models")
	}
	for model, rates := range p.Models {
		if strings.TrimSpace(model) == "" {
			return p, fmt.Errorf("empty model key")
		}
		for _, v := range rates.values() {
			if v != nil && (math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0) {
				return p, fmt.Errorf("model %q: rates must be finite and nonnegative", model)
			}
		}
	}
	return p, nil
}

func (t Tokens) values() []int64 {
	return []int64{t.Input, t.CachedInput, t.CacheWrite, t.CacheWrite1h, t.Output}
}

func (r Rates) values() []*float64 {
	return []*float64{r.Input, r.CachedInput, r.CacheWrite, r.CacheWrite1h, r.Output}
}

func (t *Tokens) add(other Tokens) error {
	dst := []*int64{&t.Input, &t.CachedInput, &t.CacheWrite, &t.CacheWrite1h, &t.Output}
	for i, n := range other.values() {
		if n < 0 || *dst[i] > math.MaxInt64-n {
			return fmt.Errorf("negative token count or token total overflow")
		}
		*dst[i] += n
	}
	return nil
}

func price(t Tokens, r Rates) (float64, error) {
	var total float64
	names := []string{"input", "cached_input", "cache_write", "cache_write_1h", "output"}
	for i, n := range t.values() {
		if n < 0 {
			return 0, fmt.Errorf("%s must be nonnegative", names[i])
		}
		rate := r.values()[i]
		if n > 0 && rate == nil {
			return 0, fmt.Errorf("missing price for used category %s", names[i])
		}
		if rate != nil {
			total += float64(n) / 1_000_000 * *rate
		}
	}
	if math.IsInf(total, 0) || math.IsNaN(total) {
		return 0, fmt.Errorf("cost overflow")
	}
	return total, nil
}

func Analyze(r io.Reader, p PriceBook) (Report, error) {
	out := Report{SchemaVersion: 1, PriceLabel: p.Label}
	tasks := map[string]*Task{}
	seen := map[string]bool{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), MaxLineBytes)
	line := 0
	for scanner.Scan() {
		line++
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		var e Event
		if err := decodeStrict(scanner.Bytes(), &e); err != nil {
			return out, fmt.Errorf("line %d: %w", line, err)
		}
		if strings.TrimSpace(e.TaskID) == "" {
			return out, fmt.Errorf("line %d: task_id is required", line)
		}
		task := tasks[e.TaskID]
		if task == nil {
			task = &Task{ID: e.TaskID}
			tasks[e.TaskID] = task
		}
		switch e.Type {
		case "request":
			if strings.TrimSpace(e.RequestID) == "" || strings.TrimSpace(e.Model) == "" || e.Tokens == nil || e.Success != nil {
				return out, fmt.Errorf("line %d: request requires request_id, model, tokens and no success field", line)
			}
			if seen[e.RequestID] {
				return out, fmt.Errorf("line %d: duplicate request_id %q (use final usage once per call)", line, e.RequestID)
			}
			seen[e.RequestID] = true
			rates, ok := p.Models[e.Model]
			if !ok {
				return out, fmt.Errorf("line %d: no exact price entry for model %q", line, e.Model)
			}
			cost, err := price(*e.Tokens, rates)
			if err != nil {
				return out, fmt.Errorf("line %d: %w", line, err)
			}
			if err := out.Tokens.add(*e.Tokens); err != nil {
				return out, fmt.Errorf("line %d: %w", line, err)
			}
			if e.DurationMS != nil {
				if *e.DurationMS < 0 || task.DurationMS > math.MaxInt64-*e.DurationMS {
					return out, fmt.Errorf("line %d: invalid duration or duration total overflow", line)
				}
				task.DurationMS += *e.DurationMS
				task.MeasuredCalls++
			}
			task.Requests++
			task.CostUSD += cost
			out.Requests++
			out.CostUSD += cost
			if math.IsInf(out.CostUSD, 0) {
				return out, fmt.Errorf("line %d: total cost overflow", line)
			}
		case "task_result":
			if e.Success == nil || e.RequestID != "" || e.Model != "" || e.Tokens != nil || e.DurationMS != nil {
				return out, fmt.Errorf("line %d: task_result requires only type, task_id and explicit success", line)
			}
			if task.Success != nil {
				return out, fmt.Errorf("line %d: duplicate task_result for %q", line, e.TaskID)
			}
			task.Success = e.Success
		default:
			return out, fmt.Errorf("line %d: unknown event type %q", line, e.Type)
		}
	}
	if err := scanner.Err(); err != nil {
		return out, fmt.Errorf("read JSONL: %w", err)
	}
	if out.Requests == 0 {
		return out, fmt.Errorf("trace has no requests")
	}
	for _, t := range tasks {
		if t.Requests == 0 {
			return out, fmt.Errorf("task %q has a result but no request", t.ID)
		}
		switch {
		case t.Success == nil:
			out.Unknown++
		case *t.Success:
			out.Successes++
		default:
			out.Failures++
		}
		out.Tasks = append(out.Tasks, *t)
	}
	sort.Slice(out.Tasks, func(i, j int) bool { return out.Tasks[i].ID < out.Tasks[j].ID })
	if out.Successes > 0 && out.Unknown == 0 {
		value := out.CostUSD / float64(out.Successes)
		out.CostPerSuccess = &value
	}
	return out, nil
}

type Comparison struct {
	SchemaVersion    int      `json:"schema_version"`
	Passed           bool     `json:"passed"`
	MaxIncreasePct   float64  `json:"max_increase_percent"`
	CostChangePct    *float64 `json:"cost_per_success_change_percent"`
	NewlyFailedTasks []string `json:"newly_failed_tasks"`
	Reasons          []string `json:"reasons"`
	Baseline         Report   `json:"baseline"`
	Candidate        Report   `json:"candidate"`
}

func Compare(a, b Report, maxIncrease float64) (Comparison, error) {
	c := Comparison{SchemaVersion: 1, Passed: true, MaxIncreasePct: maxIncrease,
		NewlyFailedTasks: []string{}, Reasons: []string{}, Baseline: a, Candidate: b}
	if math.IsNaN(maxIncrease) || math.IsInf(maxIncrease, 0) || maxIncrease < 0 {
		return c, fmt.Errorf("max-increase must be finite and nonnegative")
	}
	if a.Unknown > 0 || b.Unknown > 0 {
		return c, fmt.Errorf("comparison requires explicit task_result for every task in both runs")
	}
	if len(a.Tasks) != len(b.Tasks) {
		return c, fmt.Errorf("runs must contain exactly the same task IDs")
	}
	for i, task := range a.Tasks {
		if task.ID != b.Tasks[i].ID {
			return c, fmt.Errorf("runs must contain exactly the same task IDs")
		}
		if *task.Success && !*b.Tasks[i].Success {
			c.NewlyFailedTasks = append(c.NewlyFailedTasks, task.ID)
		}
	}
	if len(c.NewlyFailedTasks) > 0 {
		c.Reasons = append(c.Reasons, "previously successful tasks failed")
	}
	if a.CostPerSuccess == nil || b.CostPerSuccess == nil {
		c.Reasons = append(c.Reasons, "cost per success is undefined: a run has zero successful tasks")
	} else if *a.CostPerSuccess == 0 {
		if *b.CostPerSuccess > 0 {
			c.Reasons = append(c.Reasons, "cost per success increased from a zero-cost baseline")
		} else {
			zero := 0.0
			c.CostChangePct = &zero
		}
	} else {
		change := (*b.CostPerSuccess / *a.CostPerSuccess - 1) * 100
		if math.IsInf(change, 0) || math.IsNaN(change) {
			return c, fmt.Errorf("cost change overflow")
		}
		c.CostChangePct = &change
		if change > maxIncrease+1e-9 {
			c.Reasons = append(c.Reasons, "cost per success exceeded the allowed increase")
		}
	}
	c.Passed = len(c.Reasons) == 0
	return c, nil
}
