# Changelog

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
