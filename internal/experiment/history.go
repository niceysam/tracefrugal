package experiment

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func History(state string) ([]Entry, error) {
	dirs, err := os.ReadDir(filepath.Join(state, "runs"))
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	entries := []Entry{}
	for _, d := range dirs {
		if !d.IsDir() || !ValidID(d.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(state, "runs", d.Name(), "event.json"))
		if err != nil {
			return nil, err
		}
		var e Entry
		if err = json.Unmarshal(b, &e); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Started > entries[j].Started })
	return entries, nil
}

func WriteHistory(w io.Writer, state string) error {
	return WriteHistoryWithControls(w, state, "")
}

// An empty token renders a read-only export. Controls are local-server only.
func WriteHistoryWithControls(w io.Writer, state, token string) error {
	entries, err := History(state)
	if err != nil {
		return err
	}
	_, err = os.Stat(filepath.Join(state, "paused"))
	data, _ := os.ReadFile(filepath.Join(state, "active.json"))
	_, demoErr := os.Stat(filepath.Join(state, "synthetic-demo"))
	total := 0.0
	for _, e := range entries {
		if e.BaselineCost != nil {
			total += *e.BaselineCost
		}
		if e.CandidateCost != nil {
			total += *e.CandidateCost
		}
	}
	return historyTemplate.Execute(w, struct {
		Entries    []Entry
		Paused     bool
		ActiveCap  *int64
		Spend      string
		Token      string
		ActiveHash string
		Demo       bool
	}{entries, err == nil, outputCap(data), fmt.Sprintf("$%.6f", total), token, digest(data), demoErr == nil})
}

var historyTemplate = template.Must(template.New("history").Funcs(template.FuncMap{
	"cap": func(v *int64) string {
		if v == nil {
			return "—"
		}
		return fmt.Sprint(*v)
	},
	"status": func(v string) string { return strings.ReplaceAll(v, "_", " ") },
	"cost": func(v *float64) string {
		if v == nil {
			return "—"
		}
		return fmt.Sprintf("$%.6f", *v)
	},
	"change": func(v *float64) string {
		if v == nil {
			return "—"
		}
		return fmt.Sprintf("%+.2f%%", *v)
	},
}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>TraceFrugal — Optimization history</title><style>
*{box-sizing:border-box}body{margin:0;background:#f4f2ec;color:#20241f;font:16px/1.6 system-ui,sans-serif}main{max-width:1180px;margin:auto;padding:40px 24px}h1{font-size:40px;letter-spacing:-1.5px;line-height:1.15}h2{font-size:23px}.label{font:13px monospace;color:#b63819}.intro{max-width:800px;color:#62665d}.scroll{overflow:auto}table{border-collapse:collapse;width:100%;background:#faf9f5;font-size:14px}td,th{text-align:left;padding:14px;border:1px solid #dadccf}th{background:#e8ebdd}code{overflow-wrap:anywhere;font-size:12px}.status{font-weight:700}.applied{color:#24623e}.rejected,.error,.rolled_back{color:#b63819}article{padding:18px 24px;background:#faf9f5;border:1px solid #dadccf;margin:16px 0}footer{margin-top:30px;color:#62665d;font-size:13px}a{color:#b63819}button{background:#253f2b;color:white;border:0;padding:12px 20px;font:600 15px system-ui;cursor:pointer}button:focus-visible{outline:3px solid #d63e1c;outline-offset:3px}
</style></head><body><main><div class="label">TRACEFRUGAL / OPTIMIZATION HISTORY</div>
<h1>See every experiment.<br>Keep control of every change.</h1>
{{if .Demo}}<article><strong>Synthetic demo — no model calls, no API key, no real savings claim.</strong><p>Two experiments improved the simulated cost and were applied. The third failed its quality checks and was rejected. Try restoring the previous settings below; only this demo's files change.</p></article>{{end}}
<p class="intro">Each run evaluates the active profile and a candidate on the same task IDs. Auto-apply requires lower estimated cost per success and no newly failing tasks. Results describe your evaluator, not a guarantee of answer quality.</p>
<article><strong>Active output cap: {{cap .ActiveCap}} tokens</strong><p>Recorded evaluation spend: <strong>{{.Spend}}</strong>. Includes baseline and candidate tests, not live application spend. Failed calls without usage may be missing. A dash means the profile has no output cap.</p></article>
<article><strong>{{if .Paused}}Automation paused after rollback{{else}}Automation is not paused{{end}}</strong><p>Scheduling runs only while <code>tracefrugal experiment --config experiment.json --state runs/state --every 1h</code> is running. An unpaused state does not mean a scheduler is currently running.</p></article>
{{if and .Token .Paused}}<form action="/resume" method="post"><input type="hidden" name="token" value="{{.Token}}"><button type="submit">Allow experiments again</button><p>Removes the pause. It does not start a stopped scheduler.</p></form>{{end}}
<h2>Timeline (UTC)</h2><div class="scroll"><table><thead><tr><th>Started / event</th><th>Status</th><th>Output cap<br>before → after</th><th>Baseline cost</th><th>Candidate cost</th><th>Cost per success</th></tr></thead><tbody>
{{range .Entries}}<tr><td>{{.Started}}<br>{{.Action}}</td><td class="status {{.Status}}">{{status .Status}}</td><td>{{cap .BeforeOutputCap}} → {{cap .AfterOutputCap}}</td><td>{{cost .BaselineCost}}</td><td>{{cost .CandidateCost}}</td><td>{{change .Change}}</td></tr>{{else}}<tr><td colspan="6">No experiments yet. Run the included synthetic demo or connect your evaluator.</td></tr>{{end}}
</tbody></table></div>
{{range .Entries}}<article><strong>{{status .Status}} · {{.Started}}</strong><p>{{.Message}}</p>
{{if .HasReport}}<p><a href="/runs/{{.ID}}">View cost and task report →</a></p>{{end}}
{{if and .Applied (eq .Action "experiment") (not $.Token)}}<p>To restore the profile before this experiment:</p><code>tracefrugal rollback --state YOUR_STATE_DIRECTORY --id {{.ID}}</code><p>Rollback pauses automation and refuses to overwrite a newer or manually changed profile. Use the state directory passed to serve.</p>{{end}}
{{if and $.Token .Applied (eq .Action "experiment") (eq .AfterHash $.ActiveHash)}}<form action="/rollback" method="post"><input type="hidden" name="token" value="{{$.Token}}"><input type="hidden" name="id" value="{{.ID}}"><button type="submit">Restore previous settings &amp; pause</button><p>Restore this experiment's previous profile{{if .BeforeOutputCap}} (output cap {{cap .BeforeOutputCap}}){{end}}. Completed actions and spend remain.</p></form>{{end}}
<details><summary>Record and profile integrity</summary><small>Record ID: <code>{{.ID}}</code><br>Before SHA-256: <code>{{.BeforeHash}}</code><br>After SHA-256: <code>{{.AfterHash}}</code></small></details></article>{{end}}
<footer>Stored locally: profile snapshots, price books, usage traces and decisions. Only managed active.json changes; your application must consume it. Rolling back a profile cannot undo model calls, spend, emails, file edits or other actions already performed by an application. No automatic optimization of native Claude Code or Codex settings.</footer>
</main></body></html>`))
