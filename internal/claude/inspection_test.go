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
	for _, summary := range []Summary{r.Summary, s.Summary} {
		if summary.Main != 164 || summary.Subagent != 41 || summary.Spend.Responses != 205 || math.Abs(summary.Spend.USD.Total()-sum) > 1e-10 {
			t.Fatalf("economics must include every main and subagent response: %+v", summary)
		}
	}
	requests[0].Harness, requests[0].Pricing, requests[0].Cost = "codex", nil, nil
	mixed := Build(requests[:2], Health{}, now, now.Add(time.Hour))
	if mixed.Summary.Unpriced != 1 || mixed.Summary.Spend.Responses != 1 || math.Abs(mixed.Summary.Spend.USD.Total()-*requests[1].Cost) > 1e-10 {
		t.Fatalf("partial pricing must retain explicit coverage: %+v", mixed.Summary)
	}
	r = Build(requests[:1], Health{}, now, now.Add(time.Hour))
	if g := r.Sessions[0].Prices[0]; g.Reason != "native_codex_unpriced" || g.Unpriced != 1 || g.Rates != nil {
		t.Fatal("Codex must remain explicitly unpriced", g)
	}
}

func TestSpendPreservesDisjointCategories(t *testing.T) {
	tokens := ledger.Tokens{Input: 1_000_000, CachedInput: 2_000_000, CacheWrite: 3_000_000, CacheWrite1h: 4_000_000, Output: 5_000_000}
	p := priceDetails("claude-sonnet-4-6", tokens, true, "standard", "")
	cost := p.USD.Total()
	s := Summary{}
	s.Add(Request{Tokens: tokens, Reasoning: 2_000_000, Pricing: &p, Cost: &cost})
	want := Amounts{3, .6, 11.25, 24, 75}
	if s.Spend.USD != want || s.Tokens.Output != 5_000_000 || s.Spend.Responses != 1 {
		t.Fatalf("cache writes and included reasoning must not be counted twice: %+v", s)
	}
	// An old archived cost can be known without a category breakdown.
	s.Add(Request{Tokens: tokens, Cost: &cost})
	if s.Spend.Responses != 1 || s.Spend.USD != want || s.KnownUSD != cost*2 {
		t.Fatalf("missing old category details must stay uncovered: %+v", s)
	}
}
