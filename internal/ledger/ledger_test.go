package ledger

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func demoPrices() PriceBook {
	return PriceBook{SchemaVersion: 1, Currency: "USD", Label: "test rates",
		Models: map[string]Rates{"demo/model": {
			Input: ptr(5.0), CachedInput: ptr(0.5), CacheWrite: ptr(6.25),
			CacheWrite1h: ptr(10.0), Output: ptr(15.0),
		}}}
}

func trace(id string, input, cached int, success bool) string {
	return fmt.Sprintf(`{"type":"request","task_id":%q,"request_id":%q,"model":"demo/model","tokens":{"input":%d,"cached_input":%d,"output":2000}}
{"type":"task_result","task_id":%q,"success":%t}
`, id, "req-"+id, input, cached, id, success)
}

func mustAnalyze(t *testing.T, data string) Report {
	t.Helper()
	r, err := Analyze(strings.NewReader(data), demoPrices())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCostIncludesFailedTasks(t *testing.T) {
	r := mustAnalyze(t, trace("a", 10000, 500000, true)+trace("b", 10000, 500000, false))
	if math.Abs(r.CostUSD-0.66) > 1e-10 || r.CostPerSuccess == nil || math.Abs(*r.CostPerSuccess-0.66) > 1e-10 {
		t.Fatalf("failed task cost must stay in numerator: %+v", r)
	}
}

func TestSmallerInputCanCostMore(t *testing.T) {
	a := mustAnalyze(t, trace("a", 10000, 500000, true))
	b := mustAnalyze(t, trace("a", 200000, 0, true))
	c, err := Compare(a, b, 5)
	if err != nil || c.Passed || c.CostChangePct == nil || *c.CostChangePct < 200 {
		t.Fatalf("cache loss should fail: %+v, %v", c, err)
	}
}

func TestSwappedSuccessesCannotHideRegression(t *testing.T) {
	a := mustAnalyze(t, trace("a", 10000, 0, true)+trace("b", 10000, 0, false))
	b := mustAnalyze(t, trace("a", 1000, 0, false)+trace("b", 1000, 0, true))
	c, err := Compare(a, b, 5)
	if err != nil || c.Passed || len(c.NewlyFailedTasks) != 1 || c.NewlyFailedTasks[0] != "a" {
		t.Fatalf("same aggregate success is not sufficient: %+v, %v", c, err)
	}
}

func TestMalformedTraces(t *testing.T) {
	cases := map[string]string{
		"empty":             "",
		"duplicate request": trace("a", 1, 0, true) + trace("a", 1, 0, true),
		"duplicate result":  trace("a", 1, 0, true) + `{"type":"task_result","task_id":"a","success":false}`,
		"negative":          trace("a", -1, 0, true),
		"unknown model":     `{"type":"request","task_id":"a","request_id":"r","model":"missing","tokens":{"input":1}}`,
		"unknown field":     `{"type":"request","task_id":"a","request_id":"r","model":"demo/model","tokens":{"total_tokens":100}}`,
		"missing tokens":    `{"type":"request","task_id":"a","request_id":"r","model":"demo/model"}`,
		"missing success":   trace("a", 1, 0, true) + `{"type":"task_result","task_id":"b"}`,
		"orphan result":     trace("a", 1, 0, true) + `{"type":"task_result","task_id":"b","success":true}`,
		"two values":        `{} {}`,
		"negative duration": `{"type":"request","task_id":"a","request_id":"r","model":"demo/model","tokens":{},"duration_ms":-1}`,
		"overflow": `{"type":"request","task_id":"a","request_id":"r1","model":"demo/model","tokens":{"input":9223372036854775807}}
{"type":"request","task_id":"a","request_id":"r2","model":"demo/model","tokens":{"input":1}}`,
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Analyze(strings.NewReader(data), demoPrices()); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestMissingRateNotFree(t *testing.T) {
	p := demoPrices()
	p.Models["demo/model"] = Rates{Output: ptr(15.0)}
	if _, err := Analyze(strings.NewReader(trace("a", 1, 0, true)), p); err == nil {
		t.Fatal("missing used rate accepted")
	}
	p.Models["demo/model"] = Rates{Input: ptr(0.0), Output: ptr(0.0)}
	r, err := Analyze(strings.NewReader(trace("a", 1, 0, true)), p)
	if err != nil || r.CostUSD != 0 {
		t.Fatalf("explicit zero rates should work: %v", err)
	}
}

func TestComparisonRequirements(t *testing.T) {
	a := mustAnalyze(t, trace("a", 10000, 0, true))
	t.Run("unknown outcome", func(t *testing.T) {
		b := mustAnalyze(t, strings.Split(trace("a", 1, 0, true), "\n")[0])
		if b.CostPerSuccess != nil {
			t.Fatal("unknown outcome should not have cost per success")
		}
		if _, err := Compare(a, b, 5); err == nil {
			t.Fatal("missing outcome accepted")
		}
	})
	t.Run("different tasks", func(t *testing.T) {
		b := mustAnalyze(t, trace("b", 1, 0, true))
		if _, err := Compare(a, b, 5); err == nil {
			t.Fatal("different tasks accepted")
		}
	})
	t.Run("zero successes", func(t *testing.T) {
		b := mustAnalyze(t, trace("a", 1, 0, false))
		c, err := Compare(a, b, 5)
		if err != nil || c.Passed {
			t.Fatalf("zero success must fail: %v", err)
		}
	})
	t.Run("invalid threshold", func(t *testing.T) {
		for _, n := range []float64{-1, math.NaN(), math.Inf(1)} {
			if _, err := Compare(a, a, n); err == nil {
				t.Fatal("invalid threshold accepted")
			}
		}
	})
	t.Run("zero baseline", func(t *testing.T) {
		z := a
		z.CostUSD = 0
		z.CostPerSuccess = ptr(0.0)
		c, err := Compare(z, a, 5)
		if err != nil || c.Passed || c.CostChangePct != nil {
			t.Fatal("zero baseline mishandled")
		}
		c, err = Compare(z, z, 0)
		if err != nil || !c.Passed {
			t.Fatal("two zero baselines should pass")
		}
	})
	t.Run("threshold boundary", func(t *testing.T) {
		b := a
		b.CostPerSuccess = ptr(*a.CostPerSuccess * 1.05)
		c, err := Compare(a, b, 5)
		if err != nil || !c.Passed {
			t.Fatal("exact threshold should pass")
		}
	})
}

func TestReadPrices(t *testing.T) {
	for _, data := range []string{
		`{}`, `{"schema_version":2,"currency":"USD","label":"x","models":{"m":{}}}`,
		`{"schema_version":1,"currency":"EUR","label":"x","models":{"m":{}}}`,
		`{"schema_version":1,"currency":"USD","label":"x","models":{"m":{"input":-1}}}`,
		`{"schema_version":1,"currency":"USD","label":"x","models":{"m":{"imput":1}}}`,
	} {
		if _, err := ReadPrices(strings.NewReader(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
	if _, err := ReadPrices(strings.NewReader(`{"schema_version":1,"currency":"USD","label":"test","models":{"m":{"input":0}}}`)); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeOpenAI(t *testing.T) {
	data := `{"id":"resp-1","model":"example","output":[],"usage":{"input_tokens":510000,"output_tokens":2000,"input_tokens_details":{"cached_tokens":500000},"output_tokens_details":{"reasoning_tokens":500}}}`
	e, err := Normalize(strings.NewReader(data), "openai", "a", "")
	if err != nil {
		t.Fatal(err)
	}
	if e.Tokens.Input != 10000 || e.Tokens.CachedInput != 500000 || e.Tokens.Output != 2000 || e.Success != nil {
		t.Fatalf("cached or reasoning tokens double-counted: %+v", e)
	}
}

func TestNormalizeOpenAICacheWrites(t *testing.T) {
	data := `{"id":"r","model":"m","usage":{"input_tokens":510000,"output_tokens":2000,"input_tokens_details":{"cached_tokens":500000,"cache_write_tokens":6000}}}`
	e, err := Normalize(strings.NewReader(data), "openai", "a", "")
	if err != nil {
		t.Fatal(err)
	}
	if e.Tokens.Input != 4000 || e.Tokens.CacheWrite != 6000 || e.Tokens.CachedInput != 500000 {
		t.Fatalf("cache writes double-counted: %+v", e.Tokens)
	}
}

func TestNormalizeAnthropic(t *testing.T) {
	data := `{"id":"msg-1","model":"example","usage":{"input_tokens":10000,"output_tokens":2000,"cache_read_input_tokens":500000,"cache_creation_input_tokens":3000,"cache_creation":{"ephemeral_5m_input_tokens":1000,"ephemeral_1h_input_tokens":2000}}}`
	e, err := Normalize(strings.NewReader(data), "anthropic", "a", "")
	if err != nil {
		t.Fatal(err)
	}
	if e.Tokens.Input != 10000 || e.Tokens.CachedInput != 500000 || e.Tokens.CacheWrite != 1000 || e.Tokens.CacheWrite1h != 2000 {
		t.Fatalf("incorrect disjoint categories: %+v", e)
	}
}

func TestNormalizeRejectsAmbiguousUsage(t *testing.T) {
	cases := []struct{ provider, usage string }{
		{"openai", `{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cached_tokens":11}}`},
		{"openai", `{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cache_creation_tokens":4}}`},
		{"openai", `{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cached_tokens":8,"cache_write_tokens":3}}`},
		{"openai", `{"input_tokens":10,"output_tokens":1,"input_tokens_details":{"cache_write_tokens":-1}}`},
		{"openai", `{"input_tokens":-1,"output_tokens":1}`},
		{"openai", `{"output_tokens":1}`},
		{"anthropic", `{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":4}`},
		{"anthropic", `{"input_tokens":10,"output_tokens":1,"cache_creation_input_tokens":4,"cache_creation":{"ephemeral_5m_input_tokens":2,"ephemeral_1h_input_tokens":1}}`},
		{"anthropic", `{"input_tokens":10,"output_tokens":1,"cache_creation":{"ephemeral_5m_input_tokens":2}}`},
		{"unknown", `{"input_tokens":10,"output_tokens":1}`},
	}
	for _, tc := range cases {
		data := `{"id":"r","model":"m","usage":` + tc.usage + `}`
		if _, err := Normalize(strings.NewReader(data), tc.provider, "a", ""); err == nil {
			t.Fatalf("accepted %s: %s", tc.provider, data)
		}
	}
}
