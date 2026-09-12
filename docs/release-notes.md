TraceFrugal v0.8.0 adds a **local, reversible Claude Code optimizer**.

```sh
tracefrugal optimize --project /path/to/your/project
```

Check your installed CLI and official docs, preview a project-local MCP result
limit, apply with a backup, work in a fresh Claude session, then compare hourly
usage and quality. Keep, restore, and reapply decisions remain in local history.
The browser is a loopback control panel for the Go binary.

* Version-gated recipe for Claude Code **2.1.268**; other versions stay read-only.
* Actual SessionStart evidence and exact project/session/version usage matching.
* Original settings preserved; external edits protected; interrupted changes recoverable.
* English/Korean controls and guides.
* Seven real CLI sessions / 19 responses reconciled against root usage in every
  token category. Published failed trials alongside successful ones.

**Experimental:** the final bounded comparison passed both exact-answer checks
and reduced input by 2.9%, but added a model response and took longer. Cost
differences also reflect cache behavior. This is not a proven default
optimization or a day-long productivity result. Codex stays available for usage
visibility; this settings recipe is Claude-specific.

[English guide](https://github.com/niceysam/tracefrugal/blob/main/docs/local-optimizer.md) ·
[한국어 사용법](https://github.com/niceysam/tracefrugal/blob/main/docs/local-optimizer.ko.md) ·
[Measured results and failures](https://github.com/niceysam/tracefrugal/blob/main/docs/optimizer-validation.md)

**한국어:** 그래프 조회에서 실제 환경의 설정 적용·비교·유지·원복까지 이어지는
기능을 추가했습니다. 설정이 저장됐는지와 새 세션에 적용됐는지를 구분합니다.
실제 시험에서는 입력 감소와 호출 증가가 함께 나타났으므로 무조건 절약된다고
표시하지 않습니다. 현재 검증한 CLI 버전에만 적용을 허용합니다.

No API key is needed by TraceFrugal. Your normal Claude usage keeps its normal
costs. Verify downloads with `checksums.txt`. macOS binaries are not notarized.
