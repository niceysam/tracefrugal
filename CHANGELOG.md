# Changelog

## 0.8.0

- Local Claude optimizer: CLI/version and fixed official-document checks, project setting preview and one-click apply.
- Native MCP result limit trial with a real SessionStart receipt; exact project/session/version attribution.
- Private before/after snapshots, recoverable changes, external-edit protection and byte-exact restore.
- Hourly input/cache/output/cost comparison, optional satisfaction, explicit keep decision, restore and independent reapply history.
- English/Korean control panel, visible missing-data states, public usage guide and reproducible fixture.
- Real CLI evidence: seven sessions / 19 responses reconciled in every token category; failures and cache/timing tradeoffs published.
- Recipe remains experimental and restricted to reviewed Claude Code 2.1.268. No general savings or day-long productivity claim.

## 0.7.0

- English/Korean dashboards, remembered language choice, direct language URLs and Korean README/guide.
- Distinct session IDs, local browser names, search, observed dates and source/model provenance.
- Full-period input P50/P95/max and main/subagent counts before timeline truncation.
- Per-model and effective-rate cost tables with disjoint input/cache/output components and unpriced reasons.
- Session comparison and aggregate-only JSON export excluding project names, aliases and raw timelines.
- Interactive explanation of short questions, repeated context, tool-returned input and model output.
- Existing prices remain a dated standard-rate estimate; native Codex remains unpriced.

## 0.6.0

* Native Claude Code + Codex source discovery, coverage, filtering and deduplication.
* Request-level Codex accounting with labeled conservative legacy fallback.
* Context recommendations and separate reasoning/quality visibility.
* Opt-in read-only stdio MCP result archive/recall with bounded 24-hour trials.
* Hourly payload receipts including recall traffic, satisfaction and stop controls.
* Original Go archive/recall design informed by NVlabs SoL-Pi.

## 0.5.0

- Open a native Claude Code dashboard by running `tracefrugal` without arguments.
- Discover local usage, deduplicate cumulative response blocks, and show real
  time/session token graphs without a recorder or API key.
- Keep unknown pricing visibly unpriced; use dated standard list estimates for
  recognized public model IDs.
- Diagnose input/output imbalance, cache reuse, tool-result byte sources, and
  repeated calls without retaining their contents.
- Add previewable MCP, tool-output, and round-trip context rules, prior/next
  24-hour comparisons, hourly graphs, answer satisfaction, and guarded undo.
- Publish a Claude-focused interactive demo and a two-step English setup guide.
- Preserve the API/evaluator workflow separately at `experiments.html`.

## 0.4.0

- Visual dashboard shared by the public website and the Go binary: evaluation cost trend, token ring chart, task-cost bars, and outcome cards.
- Installation-free browser demo with interactive apply, reject, undo, resume, and history.
- "Use my data" screen with local JSON report viewing and source-specific connection guidance.
- Graphs and active-profile usage update after rollback; recorded testing spend is retained.
- Background polling replaces full-page reloads; unchanged data preserves focus and expanded details.
- Public assets checked against embedded sources in CI; demo transitions and import validation tested without JavaScript dependencies.
- The former landing-page cost calculator remains at `methodology.html`.

## 0.3.0

- One-command `demo` with a built-in synthetic evaluator: no configuration, Python or API key.
- Local dashboard buttons for guarded rollback and explicit resume.
- Same-origin, per-session token and loopback-host checks for browser controls.
- Clear synthetic-demo status and a first-run walkthrough.

## 0.2.0

- Evaluated output-cap experiments with optional automatic profile activation.
- Recurring evaluation intervals, finite run limits, timestamped HTML history.
- Guarded profile rollback that pauses automation, and explicit resume.

- Local, automatically refreshing usage dashboard (`serve`).
- Usage-only Python SDK recorder for OpenAI Responses and Anthropic Messages.
- Evidence-based cache and cost suggestions without invented savings.

- Self-contained HTML reports for individual runs and comparisons.
- Side-by-side costs, plain-language gate reasons, and per-task outcomes.
- Interactive public demo with synthetic cache-loss and quality-loss scenarios.
- Explicit provider compatibility guide.

## 0.1.1

- Report the installed module version for `go install ...@version` builds, as well as prebuilt release binaries.

## 0.1.0

Initial release:

- Local JSONL usage reports with explicit token price books.
- Baseline/candidate gate on cost per success and newly failed tasks.
- Separate uncached input, cache reads, cache writes, one-hour writes, and output.
- Small adapters for supported OpenAI Responses and Anthropic Messages usage.
- Synthetic examples of cache loss and quality regression.
- JSON output, documented exit codes, and CI integration.

No live provider benchmarking or automatic compression is included.
