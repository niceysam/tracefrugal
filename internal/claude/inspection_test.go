package claude

import (
	"math"
	"testing"
	"time"

	"github.com/niceysam/tracefrugal/internal/ledger"
)

func TestCategoryPricingAndUnpricedReasons(t *testing.T) {
	tokens := ledger.Tokens{Input: 1_000_000, CachedInput: 1_000_000, CacheWrite: 1_000_000, CacheWrite1h: 1_000_000, Output: 1_000_000}
	p := priceDetails("claude-sonnet-4-6", tokens, true, "standard", "us")
	want := Amounts{3.3, .33, 4.125, 6.6, 16.5}
	if p.Rates == nil || math.Abs(p.USD.Total()-want.Total()) > 1e-10 {
		t.Fatalf("all five categories and geography must be applied: %+v", p)
	}
	cases := []struct {
		model, speed, geo, reason string
		ttl                       bool
	}{
		{"unknown", "", "", "model_not_in_price_book", true},
		{"claude-sonnet-4-6", "", "", "cache_lifetime_unknown", false},
		{"claude-sonnet-4-6", "fast", "", "unsupported_speed", true},
		{"claude-sonnet-4-6", "", "unknown", "unsupported_geography", true},
		{"claude-sonnet-4-5", "", "", "unsupported_long_context_tier", true},
	}
	for _, c := range cases {
		p := priceDetails(c.model, tokens, c.ttl, c.speed, c.geo)
		if p.Reason != c.reason || p.USD != nil || p.Rates != nil {
			t.Fatalf("unpriced category must have a reason and no invented dollars: %+v", p)
		}
	}
}

func TestInspectionCoversAllResponsesAndSeparatesRates(t *testing.T) {
	now := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	var requests []Request
	for i := 1; i <= 205; i++ {
		q := Request{Session: "s", Model: "claude-sonnet-4-6", Source: "source-a", Harness: "claude", Time: now.Add(time.Duration(i) * time.Second), Tokens: ledger.Tokens{Input: int64(i), Output: 1}, Subagent: i%5 == 0}
		geo := ""
		if i%2 == 0 {
			geo = "us"
		}
		p := priceDetails(q.Model, q.Tokens, true, "", geo)
		cost := p.USD.Total()
		q.Pricing, q.Cost = &p, &cost
		requests = append(requests, q)
	}
	// The importer may cap the returned timeline later. Aggregates must retain
	// every response, including the five that would disappear from a 200 cap.
	r := Build(requests, Health{}, now, now.Add(time.Hour))
	s := r.Sessions[0]
	if s.Stats.InputP50 != 103 || s.Stats.InputP95 != 195 || s.Stats.InputMax != 205 || s.Stats.Main != 164 || s.Stats.Subagent != 41 {
		t.Fatalf("incorrect full-session distribution: %+v", s.Stats)
	}
	if len(s.Prices) != 2 || len(s.Sources) != 1 {
		t.Fatalf("rates were merged or source missing: %+v", s)
	}
	sum, count := 0.0, 0
	for _, g := range s.Prices {
		sum += g.USD.Total()
		count += g.Requests
	}
	if count != 205 || math.Abs(sum-r.Summary.KnownUSD) > 1e-10 {
		t.Fatal("price breakdown differs from total")
	}
	requests[0].Harness, requests[0].Pricing, requests[0].Cost = "codex", nil, nil
	r = Build(requests[:1], Health{}, now, now.Add(time.Hour))
	if g := r.Sessions[0].Prices[0]; g.Reason != "native_codex_unpriced" || g.Unpriced != 1 || g.Rates != nil {
		t.Fatal("Codex must remain explicitly unpriced", g)
	}
}
