package ledger

import (
	"bytes"
	"strings"
	"testing"
)

func TestHTMLComparisonEscapesInputAndPreservesOutcome(t *testing.T) {
	a := mustAnalyze(t, trace("invoice", 10000, 500000, true))
	b := mustAnalyze(t, trace("invoice", 5000, 100000, false))
	a.PriceLabel = `<script>alert("price")</script>`
	c, err := Compare(a, b, 5)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := WriteComparisonHTML(&out, c); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{"$0.330000", "$0.105000", "Failed — regression", "&lt;script&gt;", "previously successful tasks failed"} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(html, "<script>") || strings.Contains(html, "ZgotmplZ") {
		t.Fatal("unsafe or rejected HTML content")
	}
}

func TestHTMLReportShowsUnknownOutcomes(t *testing.T) {
	r := mustAnalyze(t, strings.Split(trace("a", 1, 0, true), "\n")[0])
	var out bytes.Buffer
	if err := WriteReportHTML(&out, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Unavailable", "Not evaluated", "1 have not been evaluated"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q", want)
		}
	}
}
