TraceFrugal v0.6.0 brings **Claude Code + Codex visibility and actual MCP result packing**.

Run `./tracefrugal` (Windows: `.\tracefrugal.exe`). One local Go binary opens
your browser, discovers local stores, and shows their usage and coverage.

[Interactive sample](https://niceysam.github.io/tracefrugal/) ·
[Native setup](https://github.com/niceysam/tracefrugal/blob/main/docs/native-usage.md) ·
[MCP packing](https://github.com/niceysam/tracefrugal/blob/main/docs/mcp-pack.md)

* Multiple local Claude/Codex stores, including isolated home-directory profiles.
* Per-store freshness, coverage gaps, source filters and copied-response deduplication.
* Codex request receipts preferred over cumulative counters; labeled legacy fallback.
* Separate reasoning-token visibility: already included in output, never a quality score.
* Context, MCP discovery, tool-result and round-trip recommendations with evidence limits.
* Opt-in stdio MCP proxy: allowlist + read-only hint, private archives, exact Unicode recall.
* Hourly result-byte graph including recall traffic, satisfaction history and Undo.
* Enforced expiry at 24 hours or sooner; restarting never extends the trial.
* Existing scoped Claude advisory rules and API task-outcome gates remain available.

Native Codex costs remain unpriced. Bytes are not tokens or dollars. Neither
usage nor satisfaction alone proves a causal saving. The proxy cannot rewrite
host history or unload schemas. Actual archive contents stay private.

Tested with synthetic native logs and a real child-process stdio MCP fixture,
including Unicode recall, deduplication, counter resets, error preservation,
expiry, stop and scoped local controls. No paid model call or real 24-hour
savings result is claimed.

The public demo is synthetic. Verify downloads with `checksums.txt`.
macOS releases are not notarized.
