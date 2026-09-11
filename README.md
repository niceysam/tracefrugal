<h1 align="center">TraceFrugal</h1>
<p align="center"><strong>Fewer tokens. Higher bill? Catch it before you ship.</strong></p>
<p align="center">
  <a href="https://github.com/niceysam/tracefrugal/actions/workflows/ci.yml"><img src="https://github.com/niceysam/tracefrugal/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/niceysam/tracefrugal/releases"><img src="https://img.shields.io/github/v/release/niceysam/tracefrugal" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="MIT License"></a>
  <img src="https://img.shields.io/badge/Go-1.23%2B-00ADD8.svg" alt="Go 1.23+">
</p>

TraceFrugal is a local CLI that checks whether a change to your AI agent actually reduces **estimated token cost per successful task**. It distinguishes cached input from new input and fails CI when costs rise or a previously passing task fails.

**One Go binary. No runtime dependencies. No API key. No network calls.**

## Why token counts can mislead

A smaller prompt can cost more if it loses cache reuse. A cheaper run can be worse if it stops solving the task.

These checked-in **synthetic fixtures** use hypothetical prices, not measured provider performance:

| Run | Input tokens, including cache reads | Successful tasks | Estimated token cost | Gate |
|---|---:|---:|---:|---|
| Baseline | 1,020,000 | 2/2 | $0.66 | Reference |
| Smaller input, no cache hits | 400,000 | 2/2 | $2.06 | **FAIL: +212.12% cost per success** |
| Smaller input, cache retained | 210,000 | 2/2 | $0.21 | **PASS: −68.18% cost per success** |
| Same savings, one wrong answer | 210,000 | 1/2 | $0.21 | **FAIL: task regression** |

The fixtures contain two requests each. The million-token figure is a **run total**, not a context-window size.

## Try it in a minute

Install with Go:

```sh
go install github.com/niceysam/tracefrugal/cmd/tracefrugal@latest
```

Or download a binary for macOS, Linux, or Windows from [Releases](https://github.com/niceysam/tracefrugal/releases). Verify its archive against `checksums.txt`.

Clone the examples:

```sh
git clone https://github.com/niceysam/tracefrugal.git
cd tracefrugal

tracefrugal report \
  --trace examples/baseline.jsonl \
  --prices examples/prices.json

tracefrugal compare \
  --baseline examples/baseline.jsonl \
  --candidate examples/candidate-cache-miss.jsonl \
  --prices examples/prices.json
```

The second command **intentionally exits 1**:

```text
FAIL — cost and task-outcome regression gate
Prices: SYNTHETIC demo rates — not a real provider price list (USD per million tokens)
Baseline:  $0.660000 | 2/2 successful | 2 requests
Candidate: $2.060000 | 2/2 successful | 2 requests
Cost per success: +212.12% (allowed increase: 5.00%)
- cost per success exceeded the allowed increase
Estimated token cost only; not an invoice or a statistical significance test.
```

Replace `candidate-cache-miss.jsonl` with `candidate-good.jsonl` to see a passing comparison. Use `candidate-quality-loss.jsonl` to see a cheaper run fail the quality gate.

You can run the same commands from source with `go run ./cmd/tracefrugal` instead of `tracefrugal`. The Go runner wraps nonzero child exit codes; **use the compiled binary in CI**.

## Bring your own usage, not your prompts

TraceFrugal consumes JSONL with two event types:

```jsonl
{"type":"request","task_id":"invoice-001","request_id":"req-001","model":"demo/model","tokens":{"input":10000,"cached_input":500000,"output":2000}}
{"type":"task_result","task_id":"invoice-001","success":true}
```

- Emit **one request event per model call**, using its final usage. Include retries, tool-loop calls, and subagent calls if they belong to the task.
- Emit **one task result per task**, produced by your tests, benchmark grader, or human review.
- Token categories are **disjoint**. `input` means uncached input, not total input.
- `task_id` must identify the same test case in both runs. Request IDs must be unique within each trace.
- No prompts, model responses, source code, or credentials are required.

Supply an explicit price book in **USD per million tokens**:

```json
{
  "schema_version": 1,
  "currency": "USD",
  "label": "Hypothetical example — replace with your applicable rates",
  "models": {
    "demo/model": {
      "input": 5,
      "cached_input": 0.5,
      "cache_write": 6.25,
      "cache_write_1h": 10,
      "output": 15
    }
  }
}
```

An unknown model or a missing rate for a **used** token category is an error. Free usage requires an explicit `0` rate. There are no silently guessed provider prices.

For long-context, regional, batch, or contracted rates, use separate model keys such as `provider/model@long-context` and map each request to the applicable rate class yourself. v0.1 does not infer pricing tiers.

### Normalize a provider response

Two small adapters can convert supported final response objects into request events:

```sh
tracefrugal normalize \
  --provider openai \
  --task invoice-001 \
  --response response.json > request.jsonl
```

`--provider anthropic` accepts Anthropic Messages usage. `--response -` reads stdin. Add the independently evaluated `task_result` event before comparison.

**Adapter boundaries:** OpenAI Responses cache reads and `cache_write_tokens` are supported. Anthropic nonzero cache writes require an explicit 5-minute/1-hour breakdown. Streaming events, Chat Completions, and native Claude Code/Codex session logs are not imported in v0.1. See [the format and adapter contract](docs/format.md).

## Put it in CI

Generate `runs/baseline.jsonl` and `runs/candidate.jsonl` using your own evaluation runner, then:

```sh
tracefrugal compare \
  --baseline runs/baseline.jsonl \
  --candidate runs/candidate.jsonl \
  --prices prices.json \
  --max-increase 5 \
  --format json > comparison.json
```

| Exit code | Meaning |
|---|---|
| `0` | Report completed, or comparison passed |
| `1` | Cost or task-outcome regression |
| `2` | Invalid input, incomplete comparison data, or CLI error |

The gate requires identical task sets and complete outcomes. It fails on **any newly failed task**, even when the aggregate success rate is unchanged. A run with zero successes cannot pass.

See [GitHub Actions integration](docs/ci.md).

## What it measures

```text
estimated token cost =
    uncached input × input rate
  + cached input × cache-read rate
  + cache writes × applicable cache-write rate
  + output × output rate

cost per success = all token spend / number of successful tasks
```

Failed-task spend stays in the numerator. Reasoning tokens already included in a provider's output count must not be added again.

`report --format json` also includes each task's cost, request count, outcome, and summed request duration when provided. Summed request durations are **not wall-clock latency** when calls overlap.

This is accounting and a deterministic policy gate, **not** an automatic correctness grader, compressor, invoice reconciler, statistical significance test, or agent runner. It cannot verify that your trace captured every billed request.

## How it fits

Use TraceFrugal **with** optimization tools:

- [RTK](https://github.com/rtk-ai/rtk) reduces command output.
- [Headroom](https://github.com/headroomlabs-ai/headroom) compresses agent context and offers related diagnostics.
- [LLMLingua](https://github.com/microsoft/LLMLingua) researches prompt compression.
- Your provider's native tool search and prompt caching can reduce overhead.

TraceFrugal's narrow job is to apply the **same explicit cost and task-outcome contract** to baseline and candidate runs. Other products offer overlapping features; this project does not claim to be the first.

## Limits and honest comparisons

- Costs are estimates from your rate file. Tool charges, storage, infrastructure, taxes, subscription allowances, and volume discounts are excluded unless already reflected in applicable token rates.
- Compare the same task inputs and grading rules. Record model settings, tool versions, and cache conditions with your experiment.
- Repeat stochastic tasks. A single pair of runs is not proof of a general improvement.
- Missing outcomes are displayed as unknown by `report`; `compare` rejects them.
- USD computations use IEEE 754 floating point. This tool is not a financial settlement system.
- v0.1 is a small foundation. Native session-log import, built-in graders, and automated experiment execution are not shipped features.

Read [measurement methodology](docs/methodology.md) before publishing savings claims.

## Development

Go 1.23 or later:

```sh
go test -race ./...
go vet ./...
go build -o bin/tracefrugal ./cmd/tracefrugal
```

Contributions are welcome, particularly anonymized **usage-only** fixtures, provider accounting edge cases, and reproducible failure reports. See [CONTRIBUTING.md](CONTRIBUTING.md).

MIT licensed. Built for developers working across models, tools, and languages.
