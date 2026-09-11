# Changelog

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
