# Real CLI validation — 2026-09-12

**The local setting/receipt/restore cycle works in the tested environment.
Broad productivity or cost improvement has not been established.**

This is a real Claude CLI/model experiment with an authored synthetic inventory,
not a mocked provider demo and not a day of real development work.

Environment: Claude Code **2.1.268**, macOS arm64, Bedrock,
`global.anthropic.claude-opus-5[1m]`. Fresh sessions used the same selected model,
credentials, project and read-only fixture MCP server. The optimizer changed
`MAX_MCP_OUTPUT_TOKENS` to `6000` and installed its local receipt hook. Original
settings were restored through the browser twice.

## All executed model trials

| Run | Fixture records | Exact-answer pass | Model responses | Total input¹ | Output | Seconds | CLI list-cost estimate² |
|---|---:|---|---:|---:|---:|---:|---:|
| Initial retrieval pilot | 350 | **No** | 4 | 20,843 | 1,741 | 32.82 | $0.095595 |
| Small result, before | 100 | Yes | 2 | 15,239 | 160 | 9.88 | $0.074343 |
| Small result, applied | 100 | Yes | 2 | 15,241 | 170 | 11.03 | $0.043590 |
| Larger result, before | 180 | Yes | 2 | 18,681 | 296 | 12.18 | $0.068240 |
| Larger result, applied | 180 | **No** | 4 | 25,244 | 672 | 21.85 | $0.045984 |
| Format-aware prompt, applied | 180 | Yes | 3 | 18,166 | 544 | 17.33 | $0.039223 |
| Same format-aware prompt, restored | 180 | Yes | 2 | 18,709 | 238 | 10.85 | $0.066761 |

¹ Sum of CLI root `usage.input_tokens`, cache creation and cache reads across the
run. This is repeated processed input, not a maximum context-window size.
² CLI `total_cost_usd`, explicitly reported with `costBasis: list`. **Not a
provider invoice.** It can include auxiliary CLI model usage beyond the root
conversation; do not equate it with the native dashboard's conversation estimate.

The initial pilot assumed content line numbers matched the saved JSON file's
line numbers. They did not, and the available reader could not extract the
footer. This failure is retained.

The small result remained inline. Input did **not** decrease. Its lower cost
coincided with more cache reads and fewer cache writes; that is not demonstrated
compression savings.

The first larger applied run archived the result, tried JSON decoding on a
plain-text file, then recovered. It found the correct value but appended a note,
violating the predeclared exact-output requirement. A cheap answer that fails
the output contract is a failure.

We then changed the prompt **for both final runs** to handle plain text or JSON
in one read and require only the answer. Both passed. Applied input was **2.90%
lower**, but model responses increased **2 → 3**, and elapsed time increased
**10.85 → 17.33 seconds**. The list-cost estimate was 41.25% lower, with different
cache composition. This is **not an attributable 41.25% optimization saving**.
The applied run occurred first in this pair; order was not randomized.

## Accounting and activation checks

All seven sessions / **19 model responses** were imported independently by
TraceFrugal. Every disjoint token category matched the CLI root usage totals:

| Category | CLI root total | TraceFrugal imported total |
|---|---:|---:|
| New input | 38 | 38 |
| Cache reads | 92,193 | 92,193 |
| Cache writes (5-minute) | 39,892 | 39,892 |
| Cache writes (1-hour) | 0 | 0 |
| Output | 3,821 | 3,821 |

The native reader deduplicated 17 repeated assistant snapshots. No malformed,
partial, unreadable or conflicting fixture records were reported. Model
`thinking_tokens` in the root report is not added to output again.

The browser Apply action created the expected local setting. Real `SessionStart`
hooks recorded `6000`, and their sessions matched the corresponding native usage.
The after sample excludes unverified sessions and mismatched CLI versions.
Restore removed the settings file that did not exist beforehand. A fresh
post-restore CLI run still passed the task. Reapply produced a distinct history
entry without overwriting the earlier trial.

Automated tests additionally cover existing-file byte preservation, foreign
edits, interrupted changes, backup tampering, unknown versions, missing usage,
wrong project/setting/version, retained observations and cross-origin writes.
Synthetic unit tests are separate evidence from the real CLI runs above.

## Reproduce and inspect

* [Fixture and bounded-run instructions](../examples/optimizer/README.md)
* [All measured token categories, CSV](../examples/optimizer/measured-2026-09-12.csv)
* [Validation scope and fixture hash](../examples/optimizer/validation.json)
* [Local optimizer workflow](local-optimizer.md) / [한국어](local-optimizer.ko.md)

No credentials, user configuration, real project logs or raw session IDs are
published. These are maintainer-run measurements, not independently reproduced
results. Timing and cache-dependent costs will vary.

The next evidence needed is a representative, task-matched day of development,
with retries, acceptance tests, retained quality and multiple users/providers.
Until then this is a **reversible experimental optimizer**, not a proven default
setting for every developer.

**한국어:** 실제 CLI 7세션·응답 19회의 토큰 집계는 일치했고 적용·원복·재적용도
확인했습니다. 마지막 동일 지시 비교는 양쪽 정답을 통과했지만 입력 2.9% 감소와
함께 호출·시간이 증가했습니다. 캐시 때문에 낮아진 추정 비용을 최적화 효과로
확정하지 않습니다. 실패 기록을 포함하며 실제 하루 개발 생산성 검증은 아직 없습니다.
