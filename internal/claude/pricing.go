package claude

import (
	"regexp"
	"strings"

	"github.com/niceysam/tracefrugal/internal/ledger"
)

const PriceLabel = "Anthropic standard list rates · checked 2026-09-11 · USD estimate"

// Exact public model names only. Routing aliases and provider-specific IDs are
// deliberately unpriced rather than guessed. An estimate is not a plan bill.
var standard = map[string][3]float64{
	"claude-opus-5": {5, .5, 25}, "claude-opus-4-8": {5, .5, 25},
	"claude-opus-4-7": {5, .5, 25}, "claude-opus-4-6": {5, .5, 25},
	"claude-opus-4-5": {5, .5, 25},
	"claude-sonnet-5": {2, .2, 10}, "claude-sonnet-4-6": {3, .3, 15},
	"claude-sonnet-4-5": {3, .3, 15}, "claude-haiku-4-5": {1, .1, 5},
	"claude-fable-5": {10, 1, 50}, "claude-fable-5-1": {10, .25, 50},
	"claude-mythos-5": {10, 1, 50}, "claude-mythos-5-1": {10, .25, 50},
}
var dated = regexp.MustCompile(`-\d{8}$`)

// Amounts uses the same disjoint categories for USD/1M rates and USD charges.
type Amounts struct {
	Input        float64 `json:"input"`
	CachedInput  float64 `json:"cached_input"`
	CacheWrite   float64 `json:"cache_write"`
	CacheWrite1h float64 `json:"cache_write_1h"`
	Output       float64 `json:"output"`
}

func (a Amounts) Total() float64 {
	return a.Input + a.CachedInput + a.CacheWrite + a.CacheWrite1h + a.Output
}

type Pricing struct {
	Reason string   `json:"reason,omitempty"`
	Rates  *Amounts `json:"rates,omitempty"`
	USD    *Amounts `json:"usd,omitempty"`
}

func Canonical(model string) string {
	return dated.ReplaceAllString(strings.TrimSuffix(model, "[1m]"), "")
}

func price(model string, t ledger.Tokens, ttlKnown bool, speed, geo string) *float64 {
	p := priceDetails(model, t, ttlKnown, speed, geo)
	if p.USD == nil {
		return nil
	}
	cost := p.USD.Total()
	return &cost
}

func priceDetails(model string, t ledger.Tokens, ttlKnown bool, speed, geo string) Pricing {
	name := Canonical(model)
	rate, ok := standard[name]
	if !ok {
		return Pricing{Reason: "model_not_in_price_book"}
	}
	if !ttlKnown {
		return Pricing{Reason: "cache_lifetime_unknown"}
	}
	if speed != "" && speed != "standard" {
		return Pricing{Reason: "unsupported_speed"}
	}
	if geo != "" && geo != "global" && geo != "us" {
		return Pricing{Reason: "unsupported_geography"}
	}
	// Older long-context tiers are outside this bundled price book.
	if total(t)-t.Output > 200_000 && (name == "claude-sonnet-4-5" || name == "claude-haiku-4-5" || name == "claude-opus-4-5") {
		return Pricing{Reason: "unsupported_long_context_tier"}
	}
	multiplier := 1.0
	if geo == "us" {
		multiplier = 1.1
	}
	rates := Amounts{rate[0] * multiplier, rate[1] * multiplier, rate[0] * 1.25 * multiplier, rate[0] * 2 * multiplier, rate[2] * multiplier}
	usd := Amounts{float64(t.Input) * rates.Input / 1e6, float64(t.CachedInput) * rates.CachedInput / 1e6, float64(t.CacheWrite) * rates.CacheWrite / 1e6, float64(t.CacheWrite1h) * rates.CacheWrite1h / 1e6, float64(t.Output) * rates.Output / 1e6}
	return Pricing{Rates: &rates, USD: &usd}
}
