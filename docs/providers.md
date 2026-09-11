# Will it work with my model?

**Yes, if you supply the common trace format and the applicable prices. Automatic import depends on the response format.**

There are three separate pieces:

1. **The agent app:** Claude Code, Codex, your script, or another agent.
2. **The model API:** OpenAI Responses, Anthropic Messages, Bedrock Converse, Gemini, and others.
3. **The usage record:** the actual JSON fields that describe one model call.

TraceFrugal prices usage records. It does not connect to your account, inspect your installed apps, or infer your contract.

## Claude Code: automatic local discovery

Run `tracefrugal` with no arguments for the native local dashboard. It reads
Claude Code JSONL usage, deduplicates repeated responses, and shows session and
time-based graphs. Recognized public model IDs use dated list-price estimates;
custom/cloud IDs can remain unpriced. Subscription bills are not inferred.
See the [Claude Code guide](claude-code.md).

## Automatic normalization

| Format | Command | Details |
|---|---|---|
| OpenAI Responses | `normalize --provider openai` | Separates ordinary input, cached input, cache writes, and output |
| Anthropic Messages | `normalize --provider anthropic` | Preserves disjoint input/read/write counts; nonzero writes require the 5m/1h breakdown |

Example:

```sh
tracefrugal normalize \
  --provider anthropic \
  --task case-001 \
  --response final-response.json > request.jsonl
```

Add a task result from your own evaluator. A model response being complete does not mean the task succeeded.

These adapters expect **final response objects**, not native app logs or streaming fragments. See [the exact field mapping](format.md).

## Common format

Other APIs can be used by exporting their per-request usage to JSONL:

```jsonl
{"type":"request","task_id":"case-001","request_id":"call-001","model":"your-provider/your-model","tokens":{"input":1000,"cached_input":2000,"output":300}}
{"type":"task_result","task_id":"case-001","success":true}
```

Use an exact matching `your-provider/your-model` entry in the price book.

- **Gemini:** map the provider's input, cached, output and thinking accounting correctly. No built-in adapter is shipped yet.
- **Bedrock:** use the usage fields for the specific API and model. Bedrock Converse is not an Anthropic Messages response. Region and pricing tier also matter.
- **OpenAI Chat Completions:** this is a different response schema from Responses; normalize it yourself for now.
- **Local models / Ollama:** normalized usage works, but token-price estimates do not measure electricity, GPU rental, or hardware costs. Explicit zero rates mean zero estimated *token fees*, not zero infrastructure cost.
- **Claude Code:** use `tracefrugal claude`, not the final-response normalizer.
- **Codex:** no native importer yet. Do not rename cumulative totals as request usage.

## What “shared across providers” does not mean

It does not mean every provider bills the same way. It means the comparison engine uses the same disjoint billing categories **after** correct normalization.

Changing from one provider to another is a valid comparison when both runs contain the same task cases, use the same evaluator, and both models have applicable prices in the price book. Costs and task outcomes should both be compared.

The software cannot know whether a missing request was billed, whether your rates are current, or whether your task grader is correct.
