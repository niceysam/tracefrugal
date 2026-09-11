// Package experiment runs user-defined evaluations against isolated profiles.
// Only a managed active.json is promoted; application integration is explicit.
package experiment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/niceysam/tracefrugal/internal/ledger"
)

type Config struct {
	BaselineProfile   string   `json:"baseline_profile"`
	CandidateProfile  string   `json:"candidate_profile"`
	Prices            string   `json:"prices"`
	Command           []string `json:"command"`
	TimeoutSeconds    int      `json:"timeout_seconds"`
	AutoApply         bool     `json:"auto_apply"`
	OutputTokenFactor *float64 `json:"output_token_factor,omitempty"`
	MinOutputTokens   int64    `json:"min_output_tokens,omitempty"`
}

type Entry struct {
	ID              string   `json:"id"`
	Started         string   `json:"started_at"`
	Finished        string   `json:"finished_at"`
	Action          string   `json:"action"`
	Status          string   `json:"status"`
	Message         string   `json:"message"`
	BeforeHash      string   `json:"before_sha256"`
	AfterHash       string   `json:"after_sha256"`
	BaselineCost    *float64 `json:"baseline_cost_usd,omitempty"`
	CandidateCost   *float64 `json:"candidate_cost_usd,omitempty"`
	Change          *float64 `json:"cost_per_success_change_percent,omitempty"`
	Applied         bool     `json:"applied"`
	BeforeOutputCap *int64   `json:"before_output_cap,omitempty"`
	AfterOutputCap  *int64   `json:"after_output_cap,omitempty"`
	HasReport       bool     `json:"has_report"`
}

func outputCap(data []byte) *int64 {
	var p struct {
		Cap *int64 `json:"max_output_tokens"`
	}
	if json.Unmarshal(data, &p) != nil {
		return nil
	}
	return p.Cap
}

func digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func ReadConfig(path string) (Config, error) {
	var c Config
	data, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, err
	}
	if err = d.Decode(new(any)); err != io.EOF {
		return c, fmt.Errorf("expected one config")
	}
	if c.BaselineProfile == "" || c.Prices == "" || len(c.Command) == 0 || c.Command[0] == "" || c.TimeoutSeconds < 1 || c.TimeoutSeconds > 86400 {
		return c, fmt.Errorf("requires baseline_profile, prices, command and timeout_seconds between 1 and 86400")
	}
	if (c.CandidateProfile == "") == (c.OutputTokenFactor == nil) {
		return c, fmt.Errorf("choose exactly one: candidate_profile or output_token_factor")
	}
	if c.OutputTokenFactor != nil && (*c.OutputTokenFactor <= 0 || *c.OutputTokenFactor >= 1 || c.MinOutputTokens < 1) {
		return c, fmt.Errorf("output_token_factor must be between 0 and 1; min_output_tokens must be positive")
	}
	// Data paths are relative to the config. Commands run from its directory.
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return c, err
	}
	for _, p := range []*string{&c.BaselineProfile, &c.CandidateProfile, &c.Prices} {
		if *p != "" && !filepath.IsAbs(*p) {
			*p = filepath.Join(base, *p)
		}
	}
	return c, nil
}

func candidate(c Config, before []byte) ([]byte, error) {
	if c.CandidateProfile != "" {
		return profile(c.CandidateProfile)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(before, &obj); err != nil {
		return nil, err
	}
	var cap int64
	if err := json.Unmarshal(obj["max_output_tokens"], &cap); err != nil || cap < 1 {
		return nil, fmt.Errorf("automatic output-cap experiment requires positive integer max_output_tokens in the profile")
	}
	next := int64(float64(cap) * *c.OutputTokenFactor)
	if next < c.MinOutputTokens {
		next = c.MinOutputTokens
	}
	if next >= cap {
		return before, nil
	}
	obj["max_output_tokens"] = json.RawMessage(fmt.Sprint(next))
	return json.MarshalIndent(obj, "", "  ")
}

func profile(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil || obj == nil {
		return nil, fmt.Errorf("profile must be a JSON object")
	}
	return b, nil
}

// atomicWrite keeps readers from seeing a partially written profile or event.
func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".pending-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func lock(state string) (func(), error) {
	if err := os.MkdirAll(filepath.Join(state, "runs"), 0700); err != nil {
		return nil, err
	}
	p := filepath.Join(state, ".lock")
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("state is locked; do not remove .lock until its runner has stopped: %w", err)
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	f.Close()
	return func() { os.Remove(p) }, nil
}

func start(state, action string) (Entry, string, error) {
	now := time.Now().UTC()
	e := Entry{ID: now.Format("20060102T150405.000000000Z"), Started: now.Format(time.RFC3339Nano), Action: action, Status: "running"}
	dir := filepath.Join(state, "runs", e.ID)
	if err := os.Mkdir(dir, 0700); err != nil {
		return e, dir, err
	}
	return e, dir, save(dir, e)
}

func save(dir string, e Entry) error {
	b, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, "event.json"), b)
}

func evaluate(ctx context.Context, c Config, cwd, profilePath, tracePath string, prices ledger.PriceBook) (ledger.Report, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Command[0], c.Command[1:]...)
	cmd.Dir = cwd
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "TRACEFRUGAL_PROFILE=") && !strings.HasPrefix(v, "TRACEFRUGAL_TRACE=") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "TRACEFRUGAL_PROFILE="+profilePath, "TRACEFRUGAL_TRACE="+tracePath)
	// Do not persist subprocess output: it may contain prompts or credentials.
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ledger.Report{}, fmt.Errorf("evaluation timed out or was cancelled")
		}
		return ledger.Report{}, fmt.Errorf("evaluation command failed; inspect your evaluator locally")
	}
	f, err := os.Open(tracePath)
	if err != nil {
		return ledger.Report{}, fmt.Errorf("evaluator did not produce a readable trace")
	}
	defer f.Close()
	return ledger.Analyze(f, prices)
}

// Run tests baseline and candidate before applying. Profiles are opaque:
// the user's evaluator/application must actually consume the supplied file.
func Run(ctx context.Context, configPath, state string) (entry Entry, err error) {
	state, err = filepath.Abs(state)
	if err != nil {
		return entry, err
	}
	unlock, err := lock(state)
	if err != nil {
		return entry, err
	}
	defer unlock()
	if _, statErr := os.Stat(filepath.Join(state, "paused")); statErr == nil {
		return entry, fmt.Errorf("automation paused after rollback; run tracefrugal resume --state <directory> to continue")
	} else if !os.IsNotExist(statErr) {
		return entry, statErr
	}
	entry, dir, err := start(state, "experiment")
	if err != nil {
		return entry, err
	}
	defer func() {
		entry.Finished = time.Now().UTC().Format(time.RFC3339Nano)
		if err != nil {
			entry.Status = "error"
			entry.Message = "Experiment incomplete; inspect local CLI error. Check active.json before retrying."
		}
		if saveErr := save(dir, entry); saveErr != nil {
			err = fmt.Errorf("could not finalize experiment journal: %w", saveErr)
		}
	}()
	c, err := ReadConfig(configPath)
	if err != nil {
		return entry, err
	}
	before, err := profile(filepath.Join(state, "active.json"))
	if os.IsNotExist(err) {
		before, err = profile(c.BaselineProfile)
		if err == nil {
			err = atomicWrite(filepath.Join(state, "active.json"), before)
		}
	}
	if err != nil {
		return entry, err
	}
	after, err := candidate(c, before)
	if err != nil {
		return entry, err
	}
	entry.BeforeHash, entry.AfterHash = digest(before), digest(after)
	entry.BeforeOutputCap, entry.AfterOutputCap = outputCap(before), outputCap(after)
	if entry.BeforeHash == entry.AfterHash {
		entry.Status = "unchanged"
		entry.Message = "Candidate equals active profile or the minimum output cap was reached. No model calls were made."
		return entry, nil
	}
	if err = atomicWrite(filepath.Join(dir, "before.json"), before); err != nil {
		return entry, err
	}
	if err = atomicWrite(filepath.Join(dir, "candidate.json"), after); err != nil {
		return entry, err
	}
	priceData, err := os.ReadFile(c.Prices)
	if err != nil {
		return entry, err
	}
	prices, err := ledger.ReadPrices(bytes.NewReader(priceData))
	if err != nil {
		return entry, err
	}
	if err = atomicWrite(filepath.Join(dir, "prices.json"), priceData); err != nil {
		return entry, err
	}
	if err = save(dir, entry); err != nil {
		return entry, err
	}
	cwd, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		return entry, err
	}
	a, err := evaluate(ctx, c, cwd, filepath.Join(dir, "before.json"), filepath.Join(dir, "baseline.jsonl"), prices)
	if err != nil {
		return entry, fmt.Errorf("baseline: %w", err)
	}
	entry.BaselineCost = &a.CostUSD
	if err = save(dir, entry); err != nil {
		return entry, err
	}
	b, err := evaluate(ctx, c, cwd, filepath.Join(dir, "candidate.json"), filepath.Join(dir, "candidate.jsonl"), prices)
	if err != nil {
		return entry, fmt.Errorf("candidate: %w", err)
	}
	entry.CandidateCost = &b.CostUSD
	for name, expected := range map[string]string{"before.json": entry.BeforeHash, "candidate.json": entry.AfterHash} {
		snapshot, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil || digest(snapshot) != expected {
			return entry, fmt.Errorf("evaluator modified a profile snapshot; results cannot be attributed safely")
		}
	}
	comparison, err := ledger.Compare(a, b, 0)
	if err != nil {
		return entry, err
	}
	entry.BaselineCost, entry.CandidateCost, entry.Change = &a.CostUSD, &b.CostUSD, comparison.CostChangePct
	var html bytes.Buffer
	if err = ledger.WriteComparisonHTML(&html, comparison); err != nil {
		return entry, err
	}
	if err = atomicWrite(filepath.Join(dir, "report.html"), html.Bytes()); err != nil {
		return entry, err
	}
	entry.HasReport = true
	// Auto-apply is stricter than the generic compare command: a measurable
	// reduction in cost/success is required, not just "no regression".
	improved := comparison.Passed && comparison.CostChangePct != nil && *comparison.CostChangePct < -1e-9
	entry.Status = "rejected"
	entry.Message = "No verified cost-per-success reduction without new task failures. Active profile retained."
	if improved {
		entry.Status = "recommended"
		entry.Message = "Lower cost per success with no newly failing tasks in this evaluation. Auto-apply is disabled."
		if c.AutoApply {
			current, readErr := os.ReadFile(filepath.Join(state, "active.json"))
			if readErr != nil || digest(current) != entry.BeforeHash {
				return entry, fmt.Errorf("active profile changed during evaluation; refusing promotion")
			}
			// Save intent before mutation so an interrupted run is auditable.
			entry.Status = "applying"
			if err = save(dir, entry); err != nil {
				return entry, err
			}
			if err = atomicWrite(filepath.Join(state, "active.json"), after); err != nil {
				return entry, err
			}
			entry.Applied, entry.Status = true, "applied"
			entry.Message = "Candidate activated in managed active.json. Your application must read that file; no external settings were changed."
		}
	}
	return entry, nil
}

func ValidID(id string) bool {
	if len(id) != len("20060102T150405.000000000Z") {
		return false
	}
	_, err := time.Parse("20060102T150405.000000000Z", id)
	return err == nil
}

// Rollback restores the state immediately before one applied experiment.
// A hash guard refuses to overwrite intervening changes.
func Rollback(state, id string) (entry Entry, err error) {
	if !ValidID(id) {
		return entry, fmt.Errorf("invalid experiment ID")
	}
	unlock, err := lock(state)
	if err != nil {
		return entry, err
	}
	defer unlock()
	data, err := os.ReadFile(filepath.Join(state, "runs", id, "event.json"))
	if err != nil {
		return entry, err
	}
	var source Entry
	if err = json.Unmarshal(data, &source); err != nil {
		return entry, err
	}
	if !source.Applied || source.Action != "experiment" {
		return entry, fmt.Errorf("only applied experiments can be rolled back")
	}
	current, err := profile(filepath.Join(state, "active.json"))
	if err != nil {
		return entry, err
	}
	if digest(current) != source.AfterHash {
		return entry, fmt.Errorf("active profile differs from this experiment; roll back newer changes first")
	}
	previous, err := profile(filepath.Join(state, "runs", id, "before.json"))
	if err != nil {
		return entry, err
	}
	if digest(previous) != source.BeforeHash {
		return entry, fmt.Errorf("rollback snapshot integrity mismatch")
	}
	entry, dir, err := start(state, "rollback")
	if err != nil {
		return entry, err
	}
	entry.BeforeHash, entry.AfterHash = digest(current), digest(previous)
	entry.BeforeOutputCap, entry.AfterOutputCap = outputCap(current), outputCap(previous)
	entry.Message = "Restore profile before experiment " + id
	if err = save(dir, entry); err != nil {
		return entry, err
	}
	if err = atomicWrite(filepath.Join(state, "paused"), []byte("Paused after rollback. Resume explicitly.\n")); err != nil {
		return entry, err
	}
	if err = atomicWrite(filepath.Join(state, "active.json"), previous); err != nil {
		return entry, err
	}
	entry.Applied, entry.Status = true, "rolled_back"
	entry.Finished = time.Now().UTC().Format(time.RFC3339Nano)
	err = save(dir, entry)
	return entry, err
}

func Resume(state string) (Entry, error) {
	unlock, err := lock(state)
	if err != nil {
		return Entry{}, err
	}
	defer unlock()
	if _, err := os.Stat(filepath.Join(state, "paused")); err != nil {
		return Entry{}, fmt.Errorf("state is not paused: %w", err)
	}
	e, dir, err := start(state, "resume")
	if err != nil {
		return e, err
	}
	if err = os.Remove(filepath.Join(state, "paused")); err != nil {
		return e, err
	}
	e.Status = "resumed"
	e.Message = "Automation resumed explicitly. Start experiment --every 1h to schedule evaluations."
	e.Finished = time.Now().UTC().Format(time.RFC3339Nano)
	return e, save(dir, e)
}
