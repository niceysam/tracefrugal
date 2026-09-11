TraceFrugal checks whether an agent change reduces estimated token cost **per successful task**.

This initial release includes:

- `report`: analyze normalized request-level JSONL with explicit prices.
- `compare`: fail CI on a cost increase or a newly failed task.
- `normalize`: convert supported final OpenAI Responses or Anthropic Messages usage.
- Synthetic examples showing why fewer input tokens can cost more.
- macOS, Linux, and Windows binaries for amd64 and arm64.

Download the archive for your system and verify it against `checksums.txt`.

All demo prices and traces are synthetic. This is a local accounting and policy tool, not a compressor, automatic grader, provider invoice, or live benchmark. See the README for adapter boundaries and methodology.
