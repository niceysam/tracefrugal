package claude

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/niceysam/tracefrugal/internal/ledger"
)

func TestContextTrialDayRatingAndRestore(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	now := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	price := .01
	requests := []Request{{Time: now.Add(-time.Hour), Tokens: ledger.Tokens{Input: 100, CachedInput: 900, Output: 20}, Cost: &price}}
	events := []Evidence{{ID: "before", Kind: "turn", Time: now.Add(-time.Hour)}}
	settings := filepath.Join(root, "settings.json")
	write(t, settings, `{"effortLevel":"max","permissions":{"deny":["private"]}}`)
	for _, rating := range []int{0, 6} {
		if Apply(root, state, "mcp", rating, requests, events, now) == nil {
			t.Fatal("invalid rating accepted")
		}
	}
	if err := Apply(root, state, "mcp", 5, requests, events, now); err != nil {
		t.Fatal(err)
	}
	rule, err := os.ReadFile(ruleFile(root))
	if err != nil || !bytes.Contains(rule, []byte("MCP tool discovery")) {
		t.Fatal("rule not created", err)
	}
	if Apply(root, state, "turns", 4, requests, events, now) == nil {
		t.Fatal("overlapping trial accepted")
	}
	for h := 0; h < 24; h++ {
		at := now.Add(time.Duration(h)*time.Hour + time.Minute)
		requests = append(requests, Request{Time: at, Tokens: ledger.Tokens{Input: 50, CachedInput: 400, Output: 20}, Cost: &price})
		events = append(events, Evidence{Kind: "turn", Time: at})
	}
	changes, err := Observe(root, state, requests, events, now.Add(90*time.Minute))
	if err != nil || len(changes) != 1 || len(changes[0].Hours) != 1 || changes[0].After.Requests != 2 || changes[0].RuleState != "present" {
		t.Fatal("partial observation", changes, err)
	}
	changes, err = Observe(root, state, requests, events, now.Add(25*time.Hour))
	if err != nil || len(changes[0].Hours) != 24 || changes[0].After.Requests != 24 || changes[0].Baseline.Requests != 1 || changes[0].After.Diagnostics.HumanTurns != 24 {
		t.Fatal("day comparison", changes, err)
	}
	id := changes[0].ID
	if err := Rate(root, state, id, 2); err != nil {
		t.Fatal(err)
	}
	// A finalized day survives log cleanup and later reads.
	changes, err = Observe(root, state, nil, nil, now.Add(26*time.Hour))
	if err != nil || changes[0].After.Requests != 24 || changes[0].AfterRating != 2 {
		t.Fatal("history lost", changes, err)
	}
	if err := Restore(root, state, id, now.Add(26*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ruleFile(root)); !os.IsNotExist(err) {
		t.Fatal("rule not removed")
	}
	got, _ := os.ReadFile(settings)
	if string(got) != `{"effortLevel":"max","permissions":{"deny":["private"]}}` {
		t.Fatal("unrelated settings changed")
	}
	changes, err = Observe(root, state, nil, nil, now.Add(27*time.Hour))
	if err != nil || changes[0].Status != "restored" || changes[0].BeforeRating != 5 || changes[0].AfterRating != 2 {
		t.Fatal("restore history", changes, err)
	}
}

func TestContextTrialPreservesExternalRules(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	now := time.Now()
	requests := []Request{{Time: now.Add(-time.Minute)}}
	write(t, ruleFile(root), "owner's original instructions")
	if Apply(root, state, "mcp", 4, requests, nil, now) == nil {
		t.Fatal("overwrote existing rule")
	}
	os.Remove(ruleFile(root))
	if err := Apply(root, state, "turns", 4, requests, nil, now); err != nil {
		t.Fatal(err)
	}
	changes, _ := Observe(root, state, requests, nil, now)
	write(t, ruleFile(root), "owner edited this")
	if Restore(root, state, changes[0].ID, now) == nil {
		t.Fatal("overwrote external edit")
	}
	changes, _ = Observe(root, state, requests, nil, now.Add(time.Minute))
	if changes[0].RuleState != "edited" {
		t.Fatal("missing drift signal")
	}
	// Do not follow a rules-directory link.
	other := t.TempDir()
	linkRoot := t.TempDir()
	if err := os.Symlink(other, filepath.Join(linkRoot, "rules")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if Apply(linkRoot, t.TempDir(), "mcp", 4, requests, nil, now) == nil {
		t.Fatal("followed rules symlink")
	}
	if entries, _ := os.ReadDir(other); len(entries) != 0 {
		t.Fatal("wrote outside intended directory")
	}
}

func TestRestoredPartialObservationStaysFrozen(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	now := time.Now()
	requests := []Request{{Time: now.Add(-time.Minute)}, {Time: now.Add(10 * time.Minute)}}
	if err := Apply(root, state, "tool-results", 3, requests, nil, now); err != nil {
		t.Fatal(err)
	}
	end := now.Add(20 * time.Minute)
	changes, _ := Observe(root, state, requests, nil, end)
	if err := Restore(root, state, changes[0].ID, end); err != nil {
		t.Fatal(err)
	}
	changes, _ = Observe(root, state, nil, nil, now.Add(3*time.Hour))
	if changes[0].After.Requests != 1 || len(changes[0].Hours) != 0 {
		t.Fatal("partial history changed")
	}
}

func TestSavedHourSurvivesLogRetentionDuringTrial(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	now := time.Now()
	requests := []Request{{Time: now.Add(-time.Minute)}, {Time: now.Add(time.Minute), Tokens: ledger.Tokens{CachedInput: 900}}}
	if err := Apply(root, state, "mcp", 4, requests, nil, now); err != nil {
		t.Fatal(err)
	}
	if _, err := Observe(root, state, requests, nil, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	changes, err := Observe(root, state, []Request{{Time: now.Add(70 * time.Minute), Tokens: ledger.Tokens{Input: 100}}}, nil, now.Add(90*time.Minute))
	if err != nil || changes[0].After.Requests != 2 || changes[0].After.Tokens.CachedInput != 900 || changes[0].After.Tokens.Input != 100 {
		t.Fatal("saved hour was erased", changes, err)
	}
}
