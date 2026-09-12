# Claude Code + Codex, in one local view

[한국어 시작 안내](../README.ko.md) ·
[Session identity, input percentiles and cost formulas](session-inspector.md)

Run `tracefrugal` and select a source card. Use **All sources** for a combined
graph, **24 hours / 7 days / 30 days** for the period, and a session row for
request-by-request input, cache and output.

No API key or model calls are needed. A missing directory, unreadable log or
unsupported usage record is a coverage problem, not evidence of zero usage.

## Where the app looks

| Harness | Default stores | Environment override |
|---|---|---|
| Claude Code | `~/.claude`, existing `~/.claude-*` directories | `CLAUDE_CONFIG_DIR` |
| Codex | `~/.codex`, existing `~/.codex-*` directories | `CODEX_HOME` |

Canonical paths are deduplicated within a harness. The app reads Claude
`projects/**/*.jsonl` and Codex `sessions/**/*.jsonl` plus
`archived_sessions/**/*.jsonl`. It does not execute shell profiles, inspect
credentials, or infer which launcher, account or subscription owns a request.
Several launchers can share one store. Custom locations require explicit paths:

```sh
tracefrugal watch \
  --claude-dir /path/to/claude \
  --claude-dir /path/to/another-claude \
  --codex-dir /path/to/codex
```

Supplying any explicit directory **replaces** automatic discovery.
Add `--json` for a read-only metadata export. Raw store paths, prompts, tool
arguments and tool-result bodies are not exported. Project basenames, model
names and tool names can still reveal private information; review exports
before sharing. Keep real logs and reports outside this repository.

## Accounting rules

Claude keeps the greatest output snapshot for each response, without adding
repeated message blocks. Codex prefers `token_usage_record.payload.usage`.
It never sums `turn_token_usage` or `thread_token_usage` into those receipts.

For Codex:

```text
ordinary input = input_tokens - cached_input_tokens - cache_write_input_tokens
all processed tokens = input_tokens + output_tokens
reasoning_output_tokens is a subset of output_tokens
```

Negative counts, cache totals above input, reasoning above output, or
inconsistent totals are rejected. Native Codex dollars remain **unpriced**:
session counters do not establish provider billing terms, cache-write TTL or
subscription charges.

Older Codex files with only `token_count` events use `last_token_usage`:

* Repeated cumulative snapshots are ignored.
* The first usable event contributes only its last usage, never its entire
  historical total.
* Later events count only when the cumulative delta agrees with last usage.
* Resets or gaps establish a new baseline and are reported as unresolved.
* Mixed-format files use request receipts only. Earlier legacy events are
  reported as a coverage gap rather than combined across an uncertain boundary.

These legacy records are estimates of observed requests, not invoice records.
Coverage counters describe scanned retained logs, including outside the chart
period. The graph itself uses the selected period.

Copies across stores count once, keyed by harness and response identity. The
first listed store owns that receipt; excluded copies and conflicting values
are visible. Independent stores with colliding IDs cannot be disambiguated
without more provenance. Source totals follow the same attribution as the
combined graph. Copied legacy records without stable request IDs have weaker
deduplication guarantees.

## Evidence and useful actions

High cached-input share means reuse, not automatically waste. Keep stable
prefixes and useful cache hits while reducing unnecessary context. The app
recommends focused MCP discovery, smaller result pages, archive/recall, reuse
of still-current results, and handoffs for unrelated tasks.

Tool bytes, user turns and repeated calls currently come from **Claude
transcript evidence only**. Codex tool-payload attribution is unavailable.
Mixed totals are not divided by Claude-only user turns. Neither importer
attributes exact input tokens to schemas, memory or system instructions.
Use the host's context inspector where available.

## Trials and quality

Select **one Claude source** to preview an advisory rule trial. Rule loading
and compliance are unverified; see [Claude trial details](claude-code.md).
The trial's baseline and observations cover that selected store. Copied
receipts in another store may be excluded from the all-source graph while
still present in the source-local trial.

Codex native collection is read-only. For actual result transformation, use
the separate [MCP pack proxy](mcp-pack.md). With a connected packing trial:

```sh
tracefrugal watch --pack-state /path/to/private-pack-trial
```

Open **Changes & results** for hourly bytes, recall traffic, satisfaction and
Undo. The proxy trial is separate from the selected native source; the app
does not pretend that timestamps alone prove causal token savings.

Rate answer and reasoning satisfaction yourself. For a defensible token-cost
comparison, run the same tasks before and after and use the existing
[task-outcome comparison gate](methodology.md). No real 24-hour savings claim
is bundled with the app.
