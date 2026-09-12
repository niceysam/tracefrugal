# The visual dashboard

[Open TraceFrugal](https://niceysam.github.io/tracefrugal/) to try the product without installing anything.

## Three things to do

1. **Read the graphs.** The line shows estimated evaluation costs. The ring shows new input, cached input, cache writes, and output tokens.
2. **Try optimization.** In the public demo, this tests a smaller response limit against two simulated tasks. A lower-cost candidate is applied only when both tasks pass.
3. **Undo last change.** Review the previous response limit, then restore it. Trials pause and the full change history remains.

The public workspace starts with one simulated improvement so there is a graph to explore immediately. The next trial succeeds; the following one fails its task checks. Reloading or choosing **Reset demo** resets the public simulation.

## Understand the numbers

| Card | What it means |
|---|---|
| Cost of active settings | Total token cost from the latest recorded evaluation matching the active profile. It is not ongoing production spend. |
| Applied improvement | Change in cost per successful task against the baseline of the applied experiment. It is not cumulative savings. |
| Tasks passing checks | Results from the evaluator. Missing results stay visible. TraceFrugal does not independently grade answers. |
| Total testing cost | Recorded baseline and candidate evaluation costs, including rejected trials. Missing usage from failed calls may be absent. |

The chart plots each experiment's baseline and candidate separately. A rejected point is excluded from the applied line. A restored point shows a previous evaluation of the restored settings, not a new model call. Trials can use different task mixes or prices, so the chart is descriptive; the quality gate compares the baseline and candidate within each trial.

For a usage report, the screen instead shows total recorded spend, cache share, request count, and costs **by task**. Usage records have no timestamps; TraceFrugal does not invent an hourly production chart.

## Use your own data

Select **Use my data → Choose a report** and open a TraceFrugal JSON report:

```sh
tracefrugal report --trace run.jsonl --prices prices.json --format json > report.json
```

The file stays in the browser; there is no upload service or telemetry. The viewer checks structure and consistency, but trusts the report's accounting and supplied prices. It does not re-price requests or verify a provider invoice. Raw OpenAI or Anthropic responses must be normalized and priced by the CLI first. Files larger than 4 MB are rejected.

For ongoing monitoring, [connect the usage recorder](live-dashboard.md) to an API application and run the local app. For automatic changes, also [connect a task evaluator and managed profile](experiments.md).

The same dashboard is embedded in the Go binary. Local history survives reloads in the state directory. Rollback and resume require a same-origin request, a session token, and a loopback Host. Real applications are changed only if they consume the managed active profile. Undo cannot reverse completed calls, costs, or application side effects.

## Supported sources

OpenAI Responses and Anthropic Messages final usage are supported by the recorder/normalizer. Other providers can emit the documented normalized trace format. Claude Code and Codex have a separate [native local dashboard](native-usage.md). ChatGPT and Claude Desktop are not automatically attached.

## Development

The source of the UI is `internal/webui/`. After editing it:

```sh
python3 scripts/build-site.py
node scripts/test-dashboard.cjs
```

CI verifies the checked-in public assets match the embedded UI. No JavaScript package installation is required.
