# Claude Code: from install to a useful first graph

TraceFrugal reads the usage Claude Code already records on **your computer**.
It does not need an API key, an SDK recorder, a model call, or a cloud account.

## Start

1. [Download the latest release](https://github.com/niceysam/tracefrugal/releases/latest).
   Choose your OS and architecture: `darwin_arm64` for Apple Silicon Macs,
   `darwin_amd64` for Intel Macs, `windows_amd64` for most Windows PCs, or the
   matching Linux/Windows ARM archive. Extract the archive.
2. Open a terminal in the extracted folder and run:

   ```sh
   ./tracefrugal
   ```

   On Windows PowerShell:

   ```powershell
   .\tracefrugal.exe
   ```

Your browser opens the local dashboard. Keep the terminal running; Ctrl-C stops
the app. If the browser does not open, use the printed `http://127.0.0.1:...` URL.
If the default port is busy, the app chooses a free port.

The macOS binary is not notarized. If macOS blocks it, inspect the release and
checksum, then use **System Settings → Privacy & Security → Open Anyway** if you
trust the download. Do not disable Gatekeeper globally.

With Go 1.23 or newer, installation is also available through:

```sh
go install github.com/niceysam/tracefrugal/cmd/tracefrugal@latest
tracefrugal
```

## Read the dashboard

- **Estimated token cost** uses dated Anthropic standard list rates for exact
  recognized model IDs. It is **not** your Pro/Max subscription bill, remaining
  plan allowance, or an authoritative API invoice.
- **Input / response** includes new input, cache writes, and cache reads.
  **Output / response** shows the resulting output in comparison. Totals sum
  repeated context across requests, not a single request's context occupancy.
- **Input from cache** is cached input divided by all recorded input.
- Choose **24 hours**, **7 days**, or **30 days**. Time windows are rolling;
  chart buckets align to UTC, so the first and last bucket can be partial.
- Click a **session** to see individual requests. Main and recorded subagent
  responses are included. The detailed graph shows the latest 60 responses;
  aggregate totals include the entire selected period.
- Sort sessions by tokens or known cost. **Hide project names** masks labels
  for a screenshot; it does not anonymize the underlying local API.
- **Est. cost** leaves a gap when any request in a bucket is unpriced. Such a
  bucket is not free. The table still shows its known priced subtotal.

Repeated response blocks use the snapshot with the greatest cumulative output
count, counted once per message ID. Copies across sessions are attributed to the
earliest observed timestamp. Conflicting input snapshots are flagged in coverage.
This avoids counting streaming blocks and copied history as separate requests.

## Understand the input

Input contains instructions, history, tool schemas, and tool results. Much of it
may repeat on the next request. Cached reads are discounted reuse, not automatically
waste; shrinking a stable prefix can also lose cache reuse.

The diagnostic panel measures serialized tool-result **bytes**, names their tools,
and counts repeated calls with identical serialized arguments within a session.
Payloads are measured and arguments hashed locally, then discarded. These are
clues, not token attribution or proof a call was unnecessary. Unmatched results
have their own label. Main user entries exclude tool results, metadata, and
subagent prompts; unusual transcript formats may still include synthetic turns.

Use **`/context`** for Claude Code's current schema/memory breakdown. Modern Claude
Code defers MCP definitions when tool search is available. A connected server does
not prove its complete schema is resent. Use **`/mcp`** to review unused servers
and check discovery support in your provider configuration. Keep always-loaded
memory concise; move specialist instructions into scoped rules or skills.

## Try one context improvement for a day

| Recipe | Instruction |
|---|---|
| Smaller tool results | Read narrow ranges and fields; keep full artifacts locally; expand as needed |
| MCP discovery | Discover relevant tools when available; use specific queries and bounded pages |
| Fewer round trips | Batch independent reads; reuse unchanged information; avoid redundant polling |

The app does not change your model, reasoning effort, permissions, MCP server
configuration, or existing memory. Each recipe preserves required verification.

1. Select **Preview 24-hour trial**. Read the instruction and rate current answer
   and reasoning satisfaction from **1 (very dissatisfied) to 5 (very satisfied)**.
   Usage in the preceding 24 hours is required for a baseline.
2. Confirm **Add rule & start trial**. The app saves its journal, then creates
   `rules/tracefrugal-context.md` in the chosen Claude config directory. An existing
   file is never overwritten. This user rule applies across projects using that
   directory and adds a small amount of instruction context.
3. Start a new Claude Code session. Verify loading with **`/context`**. The app can
   check that the file exists; it cannot prove Claude loaded or followed it.
   These rules guide behavior, not enforce compression.
4. Work normally for a day. **Changes & results** compares the preceding 24 hours
   with up to 24 hours afterward, plus hourly snapshots. Both periods include all
   recorded models and subagents in this directory.
5. Review input/cache reads per response, input/responses per user turn, tool-result
   bytes, output, and estimated cost per response. Rate the answers and reasoning
   after trying it. Falling satisfaction is highlighted even if input is lower.
6. Keep the rule if it helps, or **Undo · remove trial rule** and start a fresh
   Claude session. The rule stays active after 24 hours until removed; observation
   stops at 24 hours.

Per-response and per-turn comparisons adjust for activity volume, not task
difficulty, model mix, or other confounders. An observational decrease is **not a
controlled savings claim**. Satisfaction is a user rating, not an automatic
correctness score. Controlled application experiments use the separate
[evaluator workflow](experiments.md).

Restore removes only the trial's file and refuses if it was edited elsewhere.
Rule symlinks and symlinked rules directories are refused. History and ratings
remain. Restore cannot refund usage or undo actions already performed.

## Hourly observations and restarts

Keep TraceFrugal running for automatic collection. It checks once a minute even
when the browser is closed; the browser refreshes every 30 seconds.
No email, notification service, background login item, or system scheduler is
installed. Results are in **Changes & results**.

After a restart, completed missing windows are reconstructed from the available
local transcripts. Saved observations persist if transcripts later expire.
**Zero recorded requests is not proof you were inactive**: logs may be absent,
deleted, unreadable, or located on another device. A trial restored mid-hour
keeps prior complete hours and an aggregate ending at restoration. The incomplete
hour is not added as a complete hourly window. Finished comparisons survive
source-log expiration; reconstructing missing windows needs retained logs.
After-period tool-repeat counts sum the individual hourly windows, so identical
calls spanning an hour boundary are not classified as repeated within that sum.

## Paths and privacy

Discovery order: explicit `--dir`, then `CLAUDE_CONFIG_DIR`, then `~/.claude`
(`%USERPROFILE%\.claude` on Windows). Only `projects/**/*.jsonl` is scanned.
Subagent logs are included. Transcript symlinks, spilled tool results, memory
directories, and orphaned transcripts are skipped.

```sh
tracefrugal claude --dir /path/to/claude-config --no-open --port 0
tracefrugal claude --days 30 --json > usage.json
```

The JSON export contains **usage metadata, project basenames, model/tool names,
times, and aggregate tool-result sizes**, not prompts, arguments, or tool-result
contents. Treat it as private activity data.
Export mode writes no settings or state files and does not open a browser.

State defaults to the OS user config directory under `tracefrugal/<root-hash>/`.
The terminal prints its exact location; `--state /private/path` overrides it.
`changes.json` contains the trial instruction, observations, and satisfaction
ratings. Keep it private and do not commit or upload it.
State files use mode 0600 and directories 0700 where the OS supports POSIX modes;
Windows access follows your user-directory ACLs. The local HTTP API serves the
public change fields, not backups, raw transcripts, or arbitrary files.

The dashboard binds to loopback, rejects foreign Host headers, has no CORS
permission, and requires same-origin requests plus a random token for writes.
It is for one trusted local user, not a shared or remotely exposed server.

## Coverage and pricing limits

This is a local-log reader, not a provider billing integration. Deleted or missing
logs, background usage not recorded as assistant messages, remote sessions, other
devices, and non-token charges are outside the total. The coverage footer reports
malformed records, incomplete last lines, unreadable files, duplicates, and
conflicting snapshots.

Bundled rates were checked against the public pricing page on **2026-09-11**.
Supported exact IDs include Opus 4.5–4.8/5, Sonnet 4.5/4.6/5, Haiku 4.5,
Fable 5/5.1 and Mythos 5/5.1, plus date suffixes and `[1m]` markers.
Custom/cloud routing IDs are not guessed. Unknown cache-write TTL, unsupported
long-context tiers, and nonstandard speed/geo fields remain unpriced. Standard
5-minute and 1-hour writes are priced separately; recorded `inference_geo: "us"`
uses the documented 1.1× multiplier.

Contract discounts, subscription fees, tool charges, cloud-provider prices, and
other account-specific terms are not inferred. Check the provider console for
authoritative billing. Other models can still appear in token graphs.

## Sources

- [Claude Code directory reference](https://code.claude.com/docs/en/claude-directory):
  transcript locations, subagent files, config directory, and retention.
- [Claude Code costs](https://code.claude.com/docs/en/costs):
  token estimates versus subscription billing; native `/usage` and `/insights`.
- [Claude Code memory and rules](https://code.claude.com/docs/en/memory):
  user rules, scoped instructions, and instruction-following limitations.
- [Claude Code MCP](https://code.claude.com/docs/en/mcp):
  tool discovery, deferred definitions, and provider-specific support.
- [Anthropic pricing](https://platform.claude.com/docs/en/about-claude/pricing):
  input/output/cache-write/cache-read rates and special pricing.

The JSONL format is an observed implementation format, not a stable public usage
API. Fixture tests cover known shapes; coverage flags make unsupported records
visible. Claude Code itself already provides `/usage` and `/insights`.
TraceFrugal adds tool-output diagnostics and reversible context trials with user
satisfaction. Sustained usefulness still needs real user validation.
