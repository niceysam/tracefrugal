package ledger

import (
	"strings"
	"testing"
)

func TestInsightsDoNotInventSavingsOrQuality(t *testing.T) {
	r := Report{Tokens: Tokens{Input: 100, CachedInput: 100}, Unknown: 1}
	text := strings.Join(Insights(r), "\n")
	for _, want := range []string{"50.0%", "not a request cache-hit rate", "not been evaluated", "cannot be separated"} {
		if !strings.Contains(text, want) {
			t.Fatal(want, text)
		}
	}
	if strings.Contains(text, "No cache reads") {
		t.Fatal(text)
	}
	if !strings.Contains(strings.Join(Insights(Report{Tokens: Tokens{Input: 10}}), "\n"), "No cache reads") {
		t.Fatal("missing evidence")
	}
}
