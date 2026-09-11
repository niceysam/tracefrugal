# See your usage while your agent runs

TraceFrugal shows recorded token counts, estimated cost, cache usage and
task outcomes in a local browser. It suggests experiments; it does not change
your prompts or automatically compress context.

## 1. Connect your application

Install TraceFrugal and copy [record_usage.py](../examples/record_usage.py)
next to your application. After a successful SDK response:

```python
from record_usage import record_response, record_outcome

# OpenAI Responses API:
response = client.responses.create(model=your_model, input=your_input)
record_response("openai", "task-001", response)

# Or Anthropic Messages API:
message = client.messages.create(
    model=your_model, max_tokens=1024, messages=your_messages
)
record_response("anthropic", "task-002", message)

# Optional: after YOUR evaluator, once per task:
record_outcome("task-001", success=evaluator_passed)
```

This writes `run.jsonl`. Only response ID, model and usage are passed to the
normalizer; prompts and answers are not stored. Task IDs should be opaque.
Do not record the same response twice. Use a separate file per worker process.
The recorder raises an error on unsupported usage instead of silently dropping it.
Decide how your application handles that error; capture failures mean the
dashboard may not include all spend.

Streaming requires the SDK's assembled final response. Failed API calls that
provide no usage and native application logs are not captured by this example.
Anthropic cache writes require a 5-minute/1-hour breakdown.

## 2. Supply your prices

Copy `examples/prices.json`, replace the synthetic rates, and use the exact
model keys in your recorded events (e.g. `openai/<returned-model>`).
Rates are USD per million tokens, specific to your provider and billing plan.
Missing prices are errors. These are estimates, not subscription invoices.

## 3. Open the dashboard

```bash
tracefrugal serve --trace run.jsonl --prices prices.json
```

Open **http://127.0.0.1:8765/**. Its charts update every three seconds without reloading the page. It binds only
to local IPv4 loopback. No cloud account, upload or telemetry is involved.
The server rereads the complete file; use a fresh trace per run. This initial
implementation is intended for bounded runs, not indefinite log retention.
Partial writes show a temporary validation error and retry.

## 4. Optimize and verify

Start with the most expensive task. Check cache reuse for repeated prefixes;
try shorter answer formats or remove unnecessary context in your application.
Save a baseline, rerun the same tasks after the change, and compare:

```bash
tracefrugal compare --baseline before.jsonl --candidate after.jsonl \
  --prices prices.json --format html > comparison.html
```

Usage totals do **not** reveal how many tokens came from history, tool schemas
or tool results. That requires instrumentation of request construction.
No savings or quality improvement is claimed until your experiment shows it.

## Compatibility

| Source | Connection today |
|---|---|
| OpenAI Responses API | Recorder + normalizer |
| Anthropic Messages API | Recorder + normalizer |
| Other providers | Emit the documented normalized JSONL and supply prices |
| Claude Code / Codex native session logs | No automatic importer yet |
| ChatGPT / Claude subscription UI | No integration or invoice inference |

The accounting engine is shared; collection formats are provider-specific.
