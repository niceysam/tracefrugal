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

func Canonical(model string) string {
	return dated.ReplaceAllString(strings.TrimSuffix(model, "[1m]"), "")
}

func price(model string, t ledger.Tokens, ttlKnown bool, speed, geo string) *float64 {
	name := Canonical(model)
	rate, ok := standard[name]
	if !ok || !ttlKnown || (speed != "" && speed != "standard") || (geo != "" && geo != "global" && geo != "us") {
		return nil
	}
	// Older long-context tiers are outside this bundled price book.
	if total(t)-t.Output > 200_000 && (name == "claude-sonnet-4-5" || name == "claude-haiku-4-5" || name == "claude-opus-4-5") {
		return nil
	}
	cost := (float64(t.Input)*rate[0] + float64(t.CachedInput)*rate[1] + float64(t.CacheWrite)*rate[0]*1.25 + float64(t.CacheWrite1h)*rate[0]*2 + float64(t.Output)*rate[2]) / 1e6
	if geo == "us" {
		cost *= 1.1
	}
	return &cost
}
