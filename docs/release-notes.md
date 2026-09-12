TraceFrugal v0.9.0 makes **cache reuse and whole-workload costs** easier to compare.

* See cache-token share beside estimated cache-read, new-input, cache-write and
  output costs. Partial pricing is explicit; native Codex remains unpriced.
* Compare total input, model responses, input per response and total estimated
  cost. Warnings identify smaller requests with more input overall, or fewer
  input tokens with higher estimated cost.
* Review cumulative input/cache reads in local optimizer trials, together with
  answer satisfaction, missing evidence and existing Keep / Restore controls.
* Historical records without new cost/agent breakdowns show unavailable
  details instead of invented zero values.
* English/Korean UI and guides, with deterministic checks and no extra LLM calls.

The local settings recipe remains experimental and version-gated to Claude Code
2.1.268. No provider prices or supported optimization versions changed.
Comparison warnings describe observations; they do not establish matched-task
savings, cache-miss causes, completed tasks, active working time or productivity.
Recorded subagents count; unrecorded work cannot be recovered.

[English guide](https://github.com/niceysam/tracefrugal/blob/main/docs/cache-economics.md) ·
[한국어 안내](https://github.com/niceysam/tracefrugal/blob/main/docs/cache-economics.ko.md) ·
[Prior real CLI measurements](https://github.com/niceysam/tracefrugal/blob/main/docs/optimizer-validation.md)

**한국어:** 입력을 줄였어도 호출이 늘거나 추정 비용이 오를 수 있습니다.
캐시 비중·비용 구성·전체 사용량을 함께 비교하고, 품질을 확인한 뒤 유지 또는
원복하도록 개선했습니다. 캐싱 자체를 낭비로 판정하거나 절감액을 만들어내지 않습니다.

No API key is needed by TraceFrugal. Your normal Claude usage keeps its normal
costs. Verify downloads with `checksums.txt`. macOS binaries are not notarized.
