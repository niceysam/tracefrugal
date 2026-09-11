package ledger

import (
	"fmt"
	"html/template"
	"io"
	"strings"
)

type htmlMetric struct {
	Label, Baseline, Candidate string
}

type htmlTask struct {
	ID, BaselineCost, CandidateCost, BaselineOutcome, CandidateOutcome string
	Regressed                                                          bool
}

type htmlView struct {
	Title, Verdict, Summary, PriceLabel, CostChange string
	Passed, Comparison                              bool
	BaselineCost, CandidateCost                     string
	BaselineWidth, CandidateWidth                   float64
	Metrics                                         []htmlMetric
	Tasks                                           []htmlTask
	Reasons                                         []string
	Insights                                        []string
}

func money(n float64) string { return fmt.Sprintf("$%.6f", n) }

func outcome(success *bool) string {
	if success == nil {
		return "Not evaluated"
	}
	if *success {
		return "Passed"
	}
	return "Failed"
}

func perSuccess(n *float64) string {
	if n == nil {
		return "Unavailable"
	}
	return money(*n)
}

func count(n int64) string {
	s := fmt.Sprint(n)
	var out []string
	for len(s) > 3 {
		out = append([]string{s[len(s)-3:]}, out...)
		s = s[:len(s)-3]
	}
	return strings.Join(append([]string{s}, out...), ",")
}

func metrics(a, b Report) []htmlMetric {
	return []htmlMetric{
		{"Estimated token cost", money(a.CostUSD), money(b.CostUSD)},
		{"Cost per successful task", perSuccess(a.CostPerSuccess), perSuccess(b.CostPerSuccess)},
		{"Successful tasks", fmt.Sprintf("%d / %d", a.Successes, len(a.Tasks)), fmt.Sprintf("%d / %d", b.Successes, len(b.Tasks))},
		{"Tasks without evaluation", fmt.Sprint(a.Unknown), fmt.Sprint(b.Unknown)},
		{"Model calls", fmt.Sprint(a.Requests), fmt.Sprint(b.Requests)},
		{"Uncached input tokens", count(a.Tokens.Input), count(b.Tokens.Input)},
		{"Cached input tokens", count(a.Tokens.CachedInput), count(b.Tokens.CachedInput)},
		{"Default cache-write tokens", count(a.Tokens.CacheWrite), count(b.Tokens.CacheWrite)},
		{"One-hour cache-write tokens", count(a.Tokens.CacheWrite1h), count(b.Tokens.CacheWrite1h)},
		{"Output tokens", count(a.Tokens.Output), count(b.Tokens.Output)},
	}
}

// WriteReportHTML creates an offline report with no external assets or scripts.
// html/template escapes task IDs and price labels supplied in input files.
func WriteReportHTML(w io.Writer, r Report) error {
	v := htmlView{Title: "Agent cost report", Verdict: "REPORT", PriceLabel: r.PriceLabel,
		Insights:      Insights(r),
		CandidateCost: money(r.CostUSD), CandidateWidth: 100, Metrics: metrics(r, r),
		Summary: fmt.Sprintf("%d model calls across %d tasks. %d passed, %d failed, %d have not been evaluated.",
			r.Requests, len(r.Tasks), r.Successes, r.Failures, r.Unknown)}
	if r.CostUSD == 0 {
		v.CandidateWidth = 0
	}
	for _, t := range r.Tasks {
		v.Tasks = append(v.Tasks, htmlTask{ID: t.ID, CandidateCost: money(t.CostUSD), CandidateOutcome: outcome(t.Success)})
	}
	return reportTemplate.Execute(w, v)
}

// WriteComparisonHTML presents the same gate result and accounting as JSON output.
func WriteComparisonHTML(w io.Writer, c Comparison) error {
	a, b := c.Baseline, c.Candidate
	v := htmlView{Title: "Agent cost comparison", Verdict: "FAIL", Passed: c.Passed, Comparison: true,
		Insights:   Insights(b),
		PriceLabel: a.PriceLabel, BaselineCost: money(a.CostUSD), CandidateCost: money(b.CostUSD),
		Metrics: metrics(a, b), Reasons: c.Reasons}
	if c.Passed {
		v.Verdict = "PASS"
		v.Summary = "No previously passing task failed, and cost per success stayed within your threshold."
	} else {
		v.Summary = "This change did not pass the cost and task-outcome gate. Review the reasons and task results below."
	}
	if c.CostChangePct != nil {
		v.CostChange = fmt.Sprintf("%+.2f%% cost per success · allowed increase %.2f%%", *c.CostChangePct, c.MaxIncreasePct)
	} else {
		v.CostChange = "Percentage change unavailable (zero baseline cost or no successful tasks)."
	}
	maxCost := a.CostUSD
	if b.CostUSD > maxCost {
		maxCost = b.CostUSD
	}
	if maxCost > 0 {
		v.BaselineWidth = a.CostUSD / maxCost * 100
		v.CandidateWidth = b.CostUSD / maxCost * 100
	}
	for i, t := range a.Tasks {
		bt := b.Tasks[i]
		v.Tasks = append(v.Tasks, htmlTask{ID: t.ID, BaselineCost: money(t.CostUSD), CandidateCost: money(bt.CostUSD),
			BaselineOutcome: outcome(t.Success), CandidateOutcome: outcome(bt.Success),
			Regressed: *t.Success && !*bt.Success})
	}
	return reportTemplate.Execute(w, v)
}

var reportTemplate = template.Must(template.New("report").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>TraceFrugal — {{.Title}}</title>
<style>
:root{color-scheme:light}*{box-sizing:border-box}body{margin:0;background:#f4f2ec;color:#20241f;font:16px/1.6 Arial,Helvetica,sans-serif}
main{max-width:1000px;margin:0 auto;padding:40px 24px}header{border-bottom:1px solid #d6d7cb;padding-bottom:24px}
.brand{font:700 18px monospace;letter-spacing:-.7px}.brand span{color:#d63e1c}h1{font-size:40px;line-height:1.1;letter-spacing:-1.5px;margin:20px 0}
.summary{color:#62665d;max-width:760px}.badge{display:inline-block;border:1px solid #d63e1c;color:#b22f14;padding:3px 12px;font:700 14px/1.6 monospace}
.badge.pass{border-color:#2e6943;color:#2e6943}.badge.report{border-color:#62665d;color:#62665d}.delta{font:13px monospace;margin-left:14px}
.cards{display:grid;grid-template-columns:repeat(auto-fit,minmax(230px,1fr));gap:18px;margin:30px 0}.card{background:#faf9f5;border:1px solid #d6d7cb;padding:22px}.card small{color:#62665d}.card strong{display:block;font-size:35px;font-weight:500;letter-spacing:-1px}
.track{height:12px;background:#e3e5da;margin-top:16px}.bar{height:12px;background:#8ea975}.bar.candidate{background:#d63e1c}
h2{font-size:23px;letter-spacing:-.5px;margin:32px 0 14px}.table-wrap{overflow:auto;border:1px solid #d6d7cb;background:#faf9f5}
table{border-collapse:collapse;width:100%;font-size:14px}th,td{text-align:left;padding:12px 16px;border-bottom:1px solid #e3e3d9}th{font-weight:600;background:#eaece1}tr:last-child td{border-bottom:0}.regressed{background:#fff0e9}.regressed td:last-child{color:#b22f14;font-weight:700}
.note{font-size:13px;color:#62665d}.reasons{padding:18px 24px;background:#fff0e9;border-left:3px solid #d63e1c}.reasons ul{margin:8px 0;padding-left:20px}
.label{font:12px/1.7 monospace;overflow-wrap:anywhere}footer{margin-top:34px;border-top:1px solid #d6d7cb;padding-top:20px;color:#62665d;font-size:12px}
@media(max-width:600px){h1{font-size:32px}.delta{display:block;margin:12px 0}.card strong{font-size:30px}th,td{padding:10px}}
@media print{body{background:white}main{padding:0}.card,.table-wrap{break-inside:avoid}h2{break-after:avoid}}
</style></head><body><main>
<header><div class="brand"><span>t</span> tracefrugal / offline report</div><h1>{{.Title}}</h1>
<span class="badge {{if .Passed}}pass{{else if not .Comparison}}report{{end}}">{{.Verdict}}</span>
{{if .Comparison}}<span class="delta">{{.CostChange}}</span>{{end}}
<p class="summary">{{.Summary}}</p><p class="label">PRICE BOOK: {{.PriceLabel}}</p></header>
<div class="cards">
{{if .Comparison}}<div class="card"><small>Baseline · estimated token cost</small><strong>{{.BaselineCost}}</strong><div class="track"><div class="bar" style="width:{{.BaselineWidth}}%"></div></div></div>{{end}}
<div class="card"><small>{{if .Comparison}}Candidate{{else}}Run{{end}} · estimated token cost</small><strong>{{.CandidateCost}}</strong><div class="track"><div class="bar candidate" style="width:{{.CandidateWidth}}%"></div></div></div></div>
{{if .Reasons}}<div class="reasons"><strong>Why this change failed</strong><ul>{{range .Reasons}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
<h2>What was counted</h2><div class="table-wrap"><table><thead><tr><th scope="col">Metric</th>{{if .Comparison}}<th scope="col">Baseline</th>{{end}}<th scope="col">{{if .Comparison}}Candidate{{else}}Run{{end}}</th></tr></thead><tbody>
{{range .Metrics}}<tr><th scope="row">{{.Label}}</th>{{if $.Comparison}}<td>{{.Baseline}}</td>{{end}}<td>{{.Candidate}}</td></tr>{{end}}
</tbody></table></div>
<p class="note">Cached input is priced separately from uncached input. Cache writes and output use their own rates. These are summed request counts, not context-window occupancy.</p>
<h2>Did the tasks still pass?</h2><div class="table-wrap"><table><thead><tr><th scope="col">Task</th>{{if .Comparison}}<th scope="col">Baseline cost</th><th scope="col">Baseline result</th>{{end}}<th scope="col">{{if .Comparison}}Candidate{{else}}Run{{end}} cost</th><th scope="col">Result</th></tr></thead><tbody>
{{range .Tasks}}<tr{{if .Regressed}} class="regressed"{{end}}><th scope="row">{{.ID}}</th>{{if $.Comparison}}<td>{{.BaselineCost}}</td><td>{{.BaselineOutcome}}</td>{{end}}<td>{{.CandidateCost}}</td><td>{{.CandidateOutcome}}{{if .Regressed}} — regression{{end}}</td></tr>{{end}}
</tbody></table></div>
<h2>What to try next</h2><ul>{{range .Insights}}<li>{{.}}</li>{{end}}</ul>
<h2>How to read this report</h2><p class="note">Cost per successful task = all estimated token spend ÷ successful tasks. Failed-task spend stays in the total. Task results come from the evaluator that produced the trace; TraceFrugal does not independently grade answers.</p>
<footer>USD estimates from your supplied prices, not an invoice. Excludes tool fees, infrastructure and taxes. A single comparison does not establish statistical significance. This self-contained report loads no external assets and runs no JavaScript.</footer>
</main></body></html>`))
