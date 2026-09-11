# Trace format, version 1

Each nonblank line is one JSON object. Unknown fields in normalized events and price books are rejected to catch misspellings. Trace files are read incrementally, with a maximum line size of approximately 4 MiB. Do not include prompt or response bodies.

## Request event

| Field | Required | Meaning |
|---|---|---|
| `type` | Yes | `request` |
| `task_id` | Yes | Stable task case ID, shared between runs |
| `request_id` | Yes | Unique per call within this trace |
| `model` | Yes | Exact price-book key, including any applicable rate class |
| `tokens` | Yes | Disjoint nonnegative integer counts |
| `duration_ms` | No | Nonnegative request duration in milliseconds |

Token fields:

| Field | Meaning |
|---|---|
| `input` | Ordinary, uncached input tokens |
| `cached_input` | Input billed at the cache-read rate |
| `cache_write` | Input billed at the default cache-write rate; Anthropic adapter uses 5-minute writes |
| `cache_write_1h` | Input billed at the one-hour cache-write rate |
| `output` | Billed output tokens, including reasoning when the provider includes it |

Omitted token fields are zero. All-zero usage is allowed for explicitly recorded unbilled calls. Negative and fractional counts are rejected.

Use request counts from the provider's final usage, not session-to-date totals. An input count of 500,000 on one request and 510,000 on another represents 1,010,000 input tokens of usage across two requests, not 1,510,000 and not a 1,010,000-token context window.

Do not append intermediate streaming usage updates as separate calls. Duplicate request IDs fail validation. If a retry is independently billed, give it its own ID. Include its usage even if the task eventually fails.

## Task result event

```json
{"type":"task_result","task_id":"invoice-001","success":true}
```

The only fields are `type`, `task_id`, and the explicit boolean `success`. A task may contain multiple model requests but exactly one final outcome. Events may arrive in any order.

The outcome must come from a task-level evaluator. HTTP 200, a completed model response, or valid JSON alone does not establish task success.

`report` accepts tasks without outcomes and marks them unknown. Cost per success is unavailable if any outcome is missing. `compare` requires outcomes for all tasks and exactly matching task IDs.

## Provider adapters

`normalize` accepts **one final response JSON object**. It emits one request event, without a task outcome. It reads the response's `id` and `model`; `--request-id` overrides the ID. Model keys receive a provider prefix.

### OpenAI Responses: cache reads and writes

```json
{
  "id": "resp_example",
  "model": "example",
  "usage": {
    "input_tokens": 510000,
    "input_tokens_details": {"cached_tokens": 500000, "cache_write_tokens": 6000},
    "output_tokens": 2000,
    "output_tokens_details": {"reasoning_tokens": 500}
  }
}
```

Normalized model: `openai/example`. Counts: `input=4000`, `cached_input=500000`, `cache_write=6000`, `output=2000`. Reasoning tokens are not added to output again.

This adapter subtracts both `cached_tokens` and `cache_write_tokens` from the input total, following the [documented input-cost formula](https://developers.openai.com/api/docs/guides/prompt-caching#monitor-cache-performance). An absent write count is treated as zero for older response schemas. Cache reads plus writes cannot exceed total input. Supply applicable cache-write rates in your price book. Other provider schema changes may need a new adapter; a response shape alone cannot establish your billing contract.

Chat Completions (`prompt_tokens` / `completion_tokens`), SSE events, and Codex session cumulative counters are not accepted by this adapter.

### Anthropic Messages: disjoint usage

```json
{
  "id": "msg_example",
  "model": "example",
  "usage": {
    "input_tokens": 10000,
    "cache_read_input_tokens": 500000,
    "cache_creation_input_tokens": 3000,
    "cache_creation": {
      "ephemeral_5m_input_tokens": 1000,
      "ephemeral_1h_input_tokens": 2000
    },
    "output_tokens": 2000
  }
}
```

Normalized model: `anthropic/example`. Counts: `input=10000`, `cached_input=500000`, `cache_write=1000`, `cache_write_1h=2000`, `output=2000`.

Cache reads are **not subtracted** from `input_tokens` for this schema. If a creation total and a TTL breakdown are both present, they must match. Nonzero writes without a TTL breakdown are rejected rather than assigned a guessed price.

For other schemas, generate normalized JSONL directly using your SDK or evaluation runner.

## Price books

`schema_version` must be `1`, `currency` must be `USD`, and `label` must identify the rates. Every model key maps token categories to USD per million tokens. A missing price is permitted only for unused categories. Explicit `0` represents free usage.

v0.1 supports one rate per category per model key. Map pricing tiers to separate model keys before analysis. Do not silently apply standard-context pricing to long-context requests.

## Machine-readable output

`report --format json` and `compare --format json` emit objects with `schema_version: 1`. `estimated_cost_per_success_usd` and `cost_per_success_change_percent` can be `null` when undefined. JSON reports include costs per task; preserve them as CI artifacts.

Input JSON should have unique object keys. Like Go's standard JSON decoder, v0.1 uses the last occurrence of a duplicate object key. Duplicate request IDs and task results are separately rejected.
