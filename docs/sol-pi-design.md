# Design notes: SoL-Pi and TraceFrugal

Reviewed upstream commit:
[`d7ecfc0`](https://github.com/NVlabs/SoL-Pi/tree/d7ecfc089944f0d04b80122a0a9a6ca0d786f3d0).
Features and compatibility may change after this commit.

| Concern | SoL-Pi at the reviewed commit | TraceFrugal v0.6 |
|---|---|---|
| Integration | TypeScript Pi extensions, with host context access | Go local log import + opt-in stdio MCP result proxy |
| Archive/recall | ObservationPack changes provider-facing context after an initial full-result period | Archives eligible text at tool-return time; exact paged recall |
| Host history | Provider projection can leave stored history intact | Cannot rewrite the host's existing history |
| Action fusion | Edit/write with an optional known follow-up command | Recommendations to batch independent work; no mutation fusion |
| Compaction | Host boundary selection plus cache economics | No active compaction controller |
| Evidence reducer | Optional model-assisted extraction with receipt verification | Deterministic excerpts; no reducer model calls |
| Visibility | Terminal ledgers; related inspector work | Local browser source coverage, token graphs, trial bytes, ratings and history |
| Quality | Evaluate feature combinations on benchmark tasks | Existing explicit task-result gate + human satisfaction; no automatic quality judge |

Neither project makes every hosted agent runtime interchangeable. The MCP proxy
works where a host supports stdio MCP, while native accounting still needs a
format-specific importer.

## Decisions implemented

1. **Measure coverage before claiming savings.** Multiple stores, copies,
   cumulative snapshots and unknown pricing are explicit.
2. **Archive before replacing.** A result is unchanged if storage or receipt
   writing fails. Exact recall checks content integrity and Unicode boundaries.
3. **Bound the experiment.** An explicit read-only allowlist, at most 24 hours,
   an enforceable stop marker, preserved recalls and an event journal.
4. **Keep quality separate.** A lower byte total is not a cheaper correct task.
   Falling satisfaction is visible; the API gate rejects newly failed tasks.

No upstream source code is copied into the implementation.

## Features deliberately not claimed

**Transparent historical context replacement** needs a host hook such as
Pi's `context` event. An MCP proxy cannot offer it in unrelated hosts.

**Automatic compaction** needs the actual removable span, retained tail,
summary overhead, cache-write/read prices and expected future requests.
SoL-Pi's [economics implementation](https://github.com/NVlabs/SoL-Pi/blob/d7ecfc089944f0d04b80122a0a9a6ca0d786f3d0/src/sol-pi/extensions/online-context-compact/economics.ts)
is a useful starting model, not a universal threshold to copy.

**Mutation fusion** must preserve ordering, approvals and partial failures.
An edit followed by a failing command is not atomic. See the upstream
[follow-up executor](https://github.com/NVlabs/SoL-Pi/blob/d7ecfc089944f0d04b80122a0a9a6ca0d786f3d0/src/sol-pi/extensions/action-fusion/then-run.ts).

**Automatic savings acceptance** requires comparable tasks and complete usage.
The next integration is explicit task IDs linking native request receipts to
a trial, with matched before/after workloads and correctness checks.
Timestamp proximity and satisfaction alone do not establish causation.
