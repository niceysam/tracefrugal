package optimize

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/niceysam/tracefrugal/internal/claude"
)

type Sample struct {
	claude.Summary
	From   time.Time `json:"from"`
	Until  time.Time `json:"until"`
	Models []string  `json:"models"`
}
type TrialView struct {
	Trial
	Baseline   Sample    `json:"baseline"`
	After      Sample    `json:"after"`
	Hours      []Sample  `json:"hours"`
	Receipts   []Receipt `json:"receipts"`
	Drift      bool      `json:"drift"`
	CanRestore bool      `json:"can_restore"`
	Assessment string    `json:"assessment"`
}
type Report struct {
	Diagnosis Diagnosis     `json:"diagnosis"`
	Recent    Sample        `json:"recent"`
	Trials    []TrialView   `json:"trials"`
	Health    claude.Health `json:"health"`
	LogError  bool          `json:"log_error"`
}

func sample(rows []claude.Request, from, until time.Time) Sample {
	s := Sample{From: from, Until: until, Models: []string{}}
	models := map[string]bool{}
	for _, q := range rows {
		if !q.Time.Before(from) && q.Time.Before(until) {
			s.Add(q)
			models[q.Model] = true
		}
	}
	for model := range models {
		s.Models = append(s.Models, model)
	}
	sort.Strings(s.Models)
	return s
}

func ReadProject(c Config, reader *claude.Reader, now time.Time) ([]claude.Request, claude.Health, error) {
	all, health, err := reader.Read(c.Store)
	rows := []claude.Request{}
	for _, q := range all {
		if q.ProjectID == Hash(c.Project)[:16] && !q.Time.After(now) {
			rows = append(rows, q)
		}
	}
	return rows, health, err
}

func SaveBaseline(c Config, id string, rows []claude.Request, now time.Time) error {
	if !idRE.MatchString(id) {
		return errors.New("invalid trial")
	}
	b, _ := json.Marshal(sample(rows, now.Add(-24*time.Hour), now))
	return atomic(filepath.Join(c.State, id, "baseline.json"), b)
}

type savedRequest struct {
	Key string `json:"key"`
	claude.Request
}

func Snapshot(c Config, d Diagnosis, reader *claude.Reader, now time.Time) (Report, error) {
	rows, health, err := ReadProject(c, reader, now)
	r := Report{Diagnosis: d, Recent: sample(rows, now.Add(-24*time.Hour), now), Trials: []TrialView{}, Health: health}
	if err != nil {
		r.LogError = true
	}
	err = locked(c, func() error {
		j, err := load(c)
		if err != nil {
			return err
		}
		current, exists, settingsErr := readSettings(c)
		for _, t := range j.Trials {
			v := TrialView{Trial: t, Hours: []Sample{}, Receipts: []Receipt{}}
			if b, e := os.ReadFile(filepath.Join(c.State, t.ID, "baseline.json")); e == nil {
				if json.Unmarshal(b, &v.Baseline) != nil {
					return errors.New("baseline snapshot is invalid")
				}
			}
			v.Drift = settingsErr != nil || (t.Status != "restored" && (!exists || Hash(string(current)) != t.AfterHash))
			v.CanRestore = t.Status != "restored" && settingsErr == nil && ((!v.Drift) || (exists == t.BeforeExists && Hash(string(current)) == t.BeforeHash))
			paths, _ := filepath.Glob(filepath.Join(c.State, t.ID, "receipts", "*.json"))
			starts := map[string]time.Time{}
			for _, path := range paths {
				b, e := os.ReadFile(path)
				var receipt Receipt
				if e != nil || json.Unmarshal(b, &receipt) != nil {
					return errors.New("activation receipt is unreadable")
				}
				v.Receipts = append(v.Receipts, receipt)
				if receipt.Limit == Limit {
					start, ok := starts[receipt.Session]
					if !ok || receipt.Time.Before(start) {
						starts[receipt.Session] = receipt.Time
					}
				}
			}
			until := t.Started.Add(24 * time.Hour)
			if now.Before(until) {
				until = now
			}
			if t.Ended != nil && t.Ended.Before(until) {
				until = *t.Ended
			}
			saved := map[string]savedRequest{}
			usagePath := filepath.Join(c.State, t.ID, "usage.json")
			if b, e := os.ReadFile(usagePath); e == nil {
				if len(b) > 32<<20 || json.Unmarshal(b, &saved) != nil {
					return errors.New("trial usage archive is invalid")
				}
			} else if !os.IsNotExist(e) {
				return e
			}
			for _, q := range rows {
				start, ok := starts[q.Session]
				if ok && q.Version == t.Version && !q.Time.Before(start) && !q.Time.Before(t.Started) && q.Time.Before(until) {
					saved[q.ID] = savedRequest{Key: q.ID, Request: q}
				}
			}
			after := []claude.Request{}
			for _, q := range saved {
				after = append(after, q.Request)
			}
			if len(saved) > 0 {
				b, _ := json.Marshal(saved)
				if err := atomic(usagePath, b); err != nil {
					return err
				}
			}
			v.After = sample(after, t.Started, until)
			for i := 0; i < 24; i++ {
				from := t.Started.Add(time.Duration(i) * time.Hour)
				if !from.Before(until) {
					break
				}
				end := from.Add(time.Hour)
				if until.Before(end) {
					end = until
				}
				v.Hours = append(v.Hours, sample(after, from, end))
			}
			v.Assessment = "Collect comparable work and rate its quality. This is an observational comparison, not causal savings."
			switch {
			case t.Status == "prepared" || t.Status == "restoring":
				v.Assessment = "Interrupted change. Use restore to recover the original settings."
			case v.Drift && t.Status == "active":
				v.Assessment = "Settings changed outside TraceFrugal. Automatic restore is blocked; review the local backup."
			case t.Status == "restored":
				v.Assessment = "Original settings restored. This history is retained; start a new trial to compare again."
			case len(starts) == 0:
				v.Assessment = "Waiting for a Claude SessionStart receipt with the proposed setting."
			case t.Outcome == "regression" || t.AfterRating > 0 && t.AfterRating < t.BeforeRating:
				v.Assessment = "Quality regression reported. Restore before continuing."
			case v.After.Requests == 0:
				v.Assessment = "Session setting observed. Waiting for usage from that session."
			case v.Baseline.Requests == 0:
				v.Assessment = "Usage observed, but no prior project baseline. No saving can be calculated."
			case t.AfterRating == 0 || t.Outcome == "unknown":
				v.Assessment = "Usage is available. Rate quality before deciding whether to keep the change."
			}
			r.Trials = append(r.Trials, v)
		}
		return nil
	})
	return r, err
}
