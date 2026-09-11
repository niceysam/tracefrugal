TraceFrugal v0.5.0 helps Claude Code users **understand large inputs, try smaller context, and compare useful results**.

Download the archive for your computer, extract it, and run `./tracefrugal` (Windows: `.\tracefrugal.exe`). Your browser opens and local sessions appear. No API key, recorder integration, or runtime dependency.

[Interactive sample](https://niceysam.github.io/tracefrugal/) · [Setup and limitations](https://github.com/niceysam/tracefrugal/blob/main/docs/claude-code.md)

- Input, cache, output, estimated-cost, and session graphs for 24 hours, 7 days, or 30 days.
- Largest observed tool results, MCP result bytes, repeated calls, and responses per user turn.
- Three previewable context rules: smaller tool results, focused MCP discovery, and fewer redundant round trips.
- Prior 24 hours vs. next 24 hours, hourly graphs, and normalized input/cost comparisons.
- Before/after answer and reasoning satisfaction; a falling rating is highlighted.
- Undo the rule while keeping history; external edits are preserved.
- macOS, Linux, and Windows binaries for amd64 and arm64.

Trials add one instruction file; they do not change the LLM or reasoning effort. Confirm loading with `/context` in a fresh Claude session. The rule remains until removed. Instructions are not enforced compression, and observational decreases do not establish causal savings.

Tool-result bytes are clues, not token attribution. Cached reads are cheaper reuse, not automatically waste. Prices are dated standard-list estimates, not subscription bills. Unknown pricing stays unpriced. Ratings are supplied by you, not an automated judge.

Native discovery supports local Claude Code logs. API applications retain the separate OpenAI/Anthropic recorder and evaluator workflow. Codex, ChatGPT, and Claude Desktop are not natively imported.

The public demo is synthetic. No measured one-day savings or adoption claim is made. Verify downloads with `checksums.txt`; macOS releases are not notarized.
