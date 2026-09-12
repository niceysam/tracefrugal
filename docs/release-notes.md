TraceFrugal v0.7.0 makes sessions and costs understandable in **English and Korean**.

[English dashboard](https://niceysam.github.io/tracefrugal/?lang=en) ·
[한국어 대시보드](https://niceysam.github.io/tracefrugal/?lang=ko) ·
[한국어 사용 안내](https://github.com/niceysam/tracefrugal/blob/main/README.ko.md)

* Distinguish sessions with stable short IDs, models, observed dates and local names.
* Inspect input median, P95, maximum and main/subagent response counts across the selected period.
* See how each token category and effective model rate contributes to estimated dollars.
* See why a record is unpriced; unknown costs are never presented as zero.
* Compare sessions and export aggregate metadata without project names or raw events.
* Explore how short questions lead to repeated input with a diagram and request slider.
* Switch both native and API dashboards between English and Korean, entirely offline.

Run `./tracefrugal` on macOS/Linux, or `.\tracefrugal.exe` on Windows.
The browser opens with local Claude Code and Codex usage. Existing 24-hour
context trials, satisfaction history and opt-in MCP packing remain available.

**한국어:** 같은 프로젝트의 세션을 구분하고, 입력·캐시·출력별 비용 계산식을
확인할 수 있습니다. 세션 이름 지정·검색·P95 입력 분석·다른 세션과 비교를
추가했습니다. 화면 위에서 한국어를 선택하세요. 짧은 질문에도 누적 입력이
커지는 이유를 그림과 슬라이더로 설명합니다.

Prices use the existing dated standard list-rate book, not invoices or
subscription bills. Native Codex costs remain unpriced. Different-session
comparisons do not establish causal savings or equal quality. Public samples
are synthetic; no real 24-hour savings claim is made. Model IDs, code and
user-provided names stay literal in both languages.

Verify downloads with `checksums.txt`. macOS releases are not notarized.
