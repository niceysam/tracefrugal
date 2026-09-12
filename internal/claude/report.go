package claude

import (
	"sort"
	"time"

	"github.com/niceysam/tracefrugal/internal/ledger"
)

type Summary struct {
	Requests  int           `json:"requests"`
	Tokens    ledger.Tokens `json:"tokens"`
	KnownUSD  float64       `json:"known_usd"`
	Unpriced  int           `json:"unpriced"`
	Reasoning int64         `json:"reasoning_output"`
}

func (s *Summary) Add(q Request) {
	s.Requests++
	s.Tokens.Input += q.Tokens.Input
	s.Tokens.CachedInput += q.Tokens.CachedInput
	s.Tokens.CacheWrite += q.Tokens.CacheWrite
	s.Tokens.CacheWrite1h += q.Tokens.CacheWrite1h
	s.Tokens.Output += q.Tokens.Output
	s.Reasoning += q.Reasoning
	if q.Cost == nil {
		s.Unpriced++
	} else {
		s.KnownUSD += *q.Cost
	}
}

type Bucket struct {
	Start time.Time `json:"start"`
	Summary
}

type Session struct {
	ID       string       `json:"id"`
	Project  string       `json:"project"`
	Start    time.Time    `json:"start"`
	End      time.Time    `json:"end"`
	Models   []string     `json:"models"`
	Timeline []Request    `json:"timeline"`
	Sources  []string     `json:"sources"`
	Stats    SessionStats `json:"stats"`
	Prices   []PriceGroup `json:"prices"`
	Summary
}

type SessionStats struct {
	InputP50 int64 `json:"input_p50"`
	InputP95 int64 `json:"input_p95"`
	InputMax int64 `json:"input_max"`
	Main     int   `json:"main_responses"`
	Subagent int   `json:"subagent_responses"`
}

// PriceGroup is computed from every selected response, before the UI timeline
// is capped. Never infer rates from token totals or combine differing rates.
type PriceGroup struct {
	Model  string   `json:"model"`
	Reason string   `json:"reason,omitempty"`
	Rates  *Amounts `json:"rates,omitempty"`
	USD    Amounts  `json:"usd"`
	Summary
}

func inspectSession(s *Session) {
	inputs := make([]int64, 0, len(s.Timeline))
	s.Sources, s.Prices = []string{}, []PriceGroup{}
	for _, q := range s.Timeline {
		inputs = append(inputs, total(q.Tokens)-q.Tokens.Output)
		if q.Subagent {
			s.Stats.Subagent++
		} else {
			s.Stats.Main++
		}
		if q.Source != "" {
			found := false
			for _, source := range s.Sources {
				found = found || source == q.Source
			}
			if !found {
				s.Sources = append(s.Sources, q.Source)
			}
		}
		p := q.Pricing
		if p == nil {
			reason := "price_breakdown_unavailable"
			if q.Harness == "codex" {
				reason = "native_codex_unpriced"
			}
			p = &Pricing{Reason: reason}
		}
		index := -1
		for i, g := range s.Prices {
			sameRates := g.Rates == nil && p.Rates == nil || g.Rates != nil && p.Rates != nil && *g.Rates == *p.Rates
			if g.Model == q.Model && g.Reason == p.Reason && sameRates {
				index = i
				break
			}
		}
		if index < 0 {
			s.Prices = append(s.Prices, PriceGroup{Model: q.Model, Reason: p.Reason, Rates: p.Rates})
			index = len(s.Prices) - 1
		}
		g := &s.Prices[index]
		g.Add(q)
		if p.USD != nil {
			g.USD.Input += p.USD.Input
			g.USD.CachedInput += p.USD.CachedInput
			g.USD.CacheWrite += p.USD.CacheWrite
			g.USD.CacheWrite1h += p.USD.CacheWrite1h
			g.USD.Output += p.USD.Output
		}
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i] < inputs[j] })
	if n := len(inputs); n > 0 {
		// Nearest-rank percentiles: rank = ceil(p*n).
		s.Stats.InputP50 = inputs[(n+1)/2-1]
		s.Stats.InputP95 = inputs[(95*n+99)/100-1]
		s.Stats.InputMax = inputs[n-1]
	}
}

type Finding struct {
	Title   string `json:"title"`
	Detail  string `json:"detail"`
	Action  string `json:"action"`
	Session string `json:"session,omitempty"`
}

type Report struct {
	Mode        string      `json:"mode"`
	From        time.Time   `json:"from"`
	Until       time.Time   `json:"until"`
	Price       string      `json:"price_label"`
	Health      Health      `json:"health"`
	Summary     Summary     `json:"summary"`
	Buckets     []Bucket    `json:"buckets"`
	Sessions    []Session   `json:"sessions"`
	Findings    []Finding   `json:"findings"`
	Changes     []Change    `json:"changes"`
	Diagnostics Diagnostics `json:"diagnostics"`
	Recipes     []Recipe    `json:"recipes"`
}

func Build(requests []Request, health Health, from, until time.Time) Report {
	r := Report{Mode: "claude", From: from, Until: until, Price: PriceLabel, Health: health, Sessions: []Session{}, Findings: []Finding{}, Changes: []Change{}, Buckets: []Bucket{}, Recipes: Recipes}
	step := time.Hour
	if until.Sub(from) > 48*time.Hour {
		step = 24 * time.Hour
	}
	// UTC hour/day boundaries are stable through daylight-saving transitions.
	start := from.UTC().Truncate(step)
	buckets := map[int64]int{}
	for t := start; t.Before(until); t = t.Add(step) {
		buckets[t.Unix()] = len(r.Buckets)
		r.Buckets = append(r.Buckets, Bucket{Start: t})
	}
	sessions := map[string]*Session{}
	for _, q := range requests {
		if q.Time.Before(from) || !q.Time.Before(until) {
			continue
		}
		r.Summary.Add(q)
		if i, ok := buckets[q.Time.UTC().Truncate(step).Unix()]; ok {
			r.Buckets[i].Add(q)
		}
		s := sessions[q.Session]
		if s == nil {
			s = &Session{ID: q.Session, Project: q.Project, Start: q.Time, End: q.Time, Models: []string{}, Timeline: []Request{}}
			sessions[q.Session] = s
		}
		s.Add(q)
		s.Timeline = append(s.Timeline, q)
		if q.Time.Before(s.Start) {
			s.Start = q.Time
		}
		if q.Time.After(s.End) {
			s.End = q.Time
		}
		found := false
		for _, model := range s.Models {
			found = found || model == q.Model
		}
		if !found {
			s.Models = append(s.Models, q.Model)
		}
	}
	for _, s := range sessions {
		inspectSession(s)
		r.Sessions = append(r.Sessions, *s)
	}
	sort.Slice(r.Sessions, func(i, j int) bool {
		if r.Sessions[i].End.Equal(r.Sessions[j].End) {
			return r.Sessions[i].ID < r.Sessions[j].ID
		}
		return r.Sessions[i].End.After(r.Sessions[j].End)
	})
	// A signal is scoped to consecutive main-agent responses from the same
	// session and model. It is not an assertion about the reason for a miss.
	drops, largest, largeID := 0, int64(0), ""
	for _, s := range r.Sessions {
		var prev *Request
		for i := range s.Timeline {
			q := s.Timeline[i]
			if q.Subagent {
				continue
			}
			input := total(q.Tokens) - q.Tokens.Output
			if input > largest {
				largest, largeID = input, s.ID
			}
			if prev != nil && prev.Model == q.Model && q.Time.Sub(prev.Time) < 5*time.Minute {
				pinput := total(prev.Tokens) - prev.Tokens.Output
				if pinput >= 10_000 && input >= 10_000 && float64(prev.Tokens.CachedInput)/float64(pinput) >= .8 && float64(q.Tokens.CachedInput)/float64(input) < .5 {
					drops++
				}
			}
			copy := q
			prev = &copy
		}
	}
	if drops > 0 {
		r.Findings = append(r.Findings, Finding{Title: "Cache reuse dropped between nearby requests", Detail: "A main-session request used under 50% cached input after one above 80%, within five minutes on the same model.", Action: "Inspect the session graph. Check /context and /usage in Claude Code. Compaction, tool changes, or cache expiry may explain it; this log does not prove a cause."})
	}
	if largest >= 100_000 {
		r.Findings = append(r.Findings, Finding{Title: "A request carried over 100k input tokens", Detail: "The session graph shows repeated input, including cache reads. This is request input, not the sum of a conversation's unique tokens.", Action: "For an unrelated task, start a fresh session with /clear. Save a handoff first if you still need the previous context.", Session: largeID})
	}
	if r.Summary.Unpriced > 0 {
		r.Findings = append(r.Findings, Finding{Title: "Some requests have no reliable price", Detail: "Unknown model IDs, cache-write lifetimes, special speed modes, or unsupported pricing tiers remain unpriced. Their tokens are still counted.", Action: "Use the token graph for complete observed usage. Check your provider's bill for authoritative dollars."})
	}
	if len(r.Findings) == 0 && r.Summary.Requests > 0 {
		r.Findings = append(r.Findings, Finding{Title: "No strong signal from these checks", Detail: "This is not a quality score. Usage alone cannot tell whether the work was useful.", Action: "Open a session below to inspect input growth, cache reuse, and output. Compare similar tasks before deciding to change settings."})
	}
	return r
}
