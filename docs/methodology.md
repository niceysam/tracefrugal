# Measuring savings without fooling yourself

## Separate four quantities

1. Context occupancy: content available to one model request.
2. Run usage: token usage summed across model calls.
3. Cached computation: previously computed prefix state reused by a provider.
4. Estimated cost: usage multiplied by the applicable prices.

These quantities are related but not interchangeable. Passing a previous response ID may reduce client-side transmission without eliminating billable prior input.

References:

- [OpenAI conversation state](https://developers.openai.com/api/docs/guides/conversation-state)
- [OpenAI prompt caching](https://developers.openai.com/api/docs/guides/prompt-caching)
- [Anthropic prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)
- [Claude Code costs](https://code.claude.com/docs/en/costs)

Provider behavior evolves. Verify your model, billing mode, and response fields against current documentation before preparing a price book or adapter.

## Experimental protocol

Hold the task inputs, model configuration, available tools, and evaluator fixed unless they are the variable under test. Keep both the raw usage records and the experiment metadata. Do not upload private prompts as part of a public benchmark.

For each task, capture all billable model calls, including retries, retrieval follow-ups, grading calls if they are part of your chosen accounting boundary, and subagents. Use the same boundary for both runs.

Evaluate task success independently. Useful task checks include:

- Does the answer identify the correct error and its original line?
- Are amounts associated with the correct account, currency, and period?
- Does a negative condition remain negative?
- Does the selected tool and its argument set satisfy the task?
- Do the changed program's relevant tests pass?

These are suggestions for **your evaluator**, not built-in TraceFrugal graders.

Cache conditions can dominate results. Compare warm against warm and cold against cold, or publish both. Do not warm only the optimized run. Record request ordering and idle gaps where they affect cache eligibility.

Repeat nondeterministic tasks. Report the number of repetitions and distributions. For repeated cases, use IDs such as `invoice-001/rep-01` consistently across runs. v0.1 does not compute confidence intervals or account for correlation.

## Gate policy

The cost metric is total estimated token cost divided by successful tasks. Failed-task cost is retained.

A comparison fails if:

- Any baseline-successful task becomes unsuccessful.
- Either run has zero successful tasks.
- Cost per success rises above `--max-increase` (5% by default).

Missing outcomes, unmatched task sets, unsupported token fields, or missing prices are input errors, not regression findings. Error exit code 2 fails a normal CI step just as exit code 1 does.

A lower cost per success does not prove every individual task became cheaper. Inspect the task-level report. Per-task cost thresholds are not part of v0.1.

## Limits of the number

The estimate excludes non-token fees and cannot reconcile a provider invoice. Subscription usage is not automatically a per-token charge. A price book can represent applicable token rates but not all subscription economics.

Tool output compression is not equivalent to whole-run cost reduction. Output tokens and cache writes may have different prices; additional model calls can erase savings.

The bundled fixtures demonstrate accounting and gate behavior only. They are not live model evaluations and do not support a claim that any compressor saves a particular percentage.
