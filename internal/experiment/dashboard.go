package experiment

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/niceysam/tracefrugal/internal/ledger"
)

// DashboardSnapshot contains usage metadata only. Profiles and raw traces are
// never served by the dashboard. Old journals remain compatible.
type DashboardSnapshot struct {
	Entries       []Entry        `json:"entries"`
	Paused        bool           `json:"paused"`
	ActiveCap     *int64         `json:"active_cap"`
	ActiveHash    string         `json:"active_hash"`
	Spend         float64        `json:"spend"`
	Synthetic     bool           `json:"synthetic"`
	CurrentReport *ledger.Report `json:"current_report,omitempty"`
	RollbackID    string         `json:"rollback_id,omitempty"`
}

func Snapshot(state string) (DashboardSnapshot, error) {
	var s DashboardSnapshot
	entries, err := History(state)
	if err != nil {
		return s, err
	}
	s.Entries = entries
	active, err := os.ReadFile(filepath.Join(state, "active.json"))
	if err != nil && !os.IsNotExist(err) {
		return s, err
	}
	s.ActiveHash, s.ActiveCap = digest(active), outputCap(active)
	_, err = os.Stat(filepath.Join(state, "paused"))
	s.Paused = err == nil
	_, err = os.Stat(filepath.Join(state, "synthetic-demo"))
	s.Synthetic = err == nil
	for _, e := range entries {
		if e.BaselineCost != nil {
			s.Spend += *e.BaselineCost
		}
		if e.CandidateCost != nil {
			s.Spend += *e.CandidateCost
		}
		if e.Action != "experiment" || !ValidID(e.ID) {
			continue
		}
		if s.RollbackID == "" && e.Applied && e.AfterHash == s.ActiveHash {
			s.RollbackID = e.ID
		}
		if s.CurrentReport != nil {
			continue
		}
		trace := ""
		if e.BeforeHash == s.ActiveHash && e.BaselineCost != nil {
			trace = "baseline.jsonl"
		} else if e.AfterHash == s.ActiveHash && e.CandidateCost != nil {
			trace = "candidate.jsonl"
		}
		if trace == "" {
			continue
		}
		dir := filepath.Join(state, "runs", e.ID)
		priceData, priceErr := os.ReadFile(filepath.Join(dir, "prices.json"))
		traceData, traceErr := os.ReadFile(filepath.Join(dir, trace))
		if priceErr != nil || traceErr != nil {
			continue // Old or incomplete records have no current usage card.
		}
		prices, readErr := ledger.ReadPrices(bytes.NewReader(priceData))
		if readErr != nil {
			continue
		}
		report, analyzeErr := ledger.Analyze(bytes.NewReader(traceData), prices)
		if analyzeErr == nil {
			s.CurrentReport = &report
		}
	}
	return s, nil
}
