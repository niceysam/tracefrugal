# Archive large MCP results, then recall what you need

The opt-in **stdio MCP proxy** transforms actual tool results without another
LLM. It archives a qualifying result locally and returns a short head/tail
excerpt, an explicit omission warning, a SHA-256 handle and an exact recall
tool. It does not edit your host configuration automatically.

This is an experimental integration. Start with a read-only search or file
inspection tool whose omitted evidence can be retrieved before acting.

## Connect one existing MCP server

Suppose your host already starts an MCP server with this configuration:

```json
{"command": "/absolute/path/to/your-mcp-server", "args": ["--stdio"]}
```

Replace that server's **command and args** with:

```json
{
  "command": "/absolute/path/to/tracefrugal",
  "args": [
    "pack",
    "--state", "/absolute/path/to/private-trials/search-day-1",
    "--allow", "search_docs",
    "--duration", "24h",
    "--",
    "/absolute/path/to/your-mcp-server", "--stdio"
  ]
}
```

Keep its existing environment/authentication configuration in your host.
Names and paths above are placeholders. Use the exact tool name in the
upstream server's `tools/list`, not the host's displayed `mcp__...` prefix.
Do not include credentials in argv or committed files.

Both conditions must hold: the name appears in your `--allow` list **and**
the server advertises `annotations.readOnlyHint: true`. This metadata is a
server assertion, not proof of safety; choose the server and tools yourself.
Tools lacking the hint pass through unchanged.

Connect the visual dashboard to the same private state directory:

```sh
tracefrugal watch --pack-state /absolute/path/to/private-trials/search-day-1
```

The trial begins when the proxy is first started, not when the dashboard opens.
Before the first qualifying result, record baseline satisfaction in the
dashboard or run:

```sh
tracefrugal pack-rate --state /absolute/path/to/private-trials/search-day-1 --before --rating 4
```

Use your agent normally. Open **Changes & results** to see actual transformation
receipts, hourly result bytes including recall, and before/after satisfaction.
You can also run `pack-status --state PATH` for JSON.

## Exactly what changes

* Only a **single text content block**, over 10 KiB and at most 16 MiB, with
  no error, structured result, annotations, extra fields, image or other
  semantic content is eligible.
* Its original UTF-8 text is stored in a private `archives/` directory with
  a SHA-256 filename. Newly created files use mode 0600; directories 0700
  where the OS supports POSIX permissions.
* The replacement contains up to 1 KiB of head/tail text plus recall
  instructions. The excerpt is incomplete and says so.
* `tracefrugal_recall` returns exact pages of up to 16 KiB. Offsets are UTF-8
  byte offsets; use the returned `next_offset` until it is null.
* Every packing and recall operation has a metadata receipt. An archive or
  journal failure leaves the original result unchanged.
* No prompt, tool arguments or original result body is written to the event
  journal. **Archive files do contain the complete tool text** and must be
  treated like the original data. The dashboard never serves archive bodies.

The proxy forwards initialization, request IDs, errors, notifications,
server requests and paginated tool lists. The recall tool is added to the last
page. `tracefrugal_recall` is reserved. Non-stdio transports, JSON-RPC batches,
oversized wire frames and non-JSON upstream stdout are not supported.

## Expiry, undo and retained history

After 24 hours (or the shorter configured duration), the proxy returns future
results unchanged. Restarting with the same state **does not reset the clock**.
**Undo · stop future packing** stops sooner. Equivalent CLI:

```sh
tracefrugal pack-stop --state /absolute/path/to/private-trials/search-day-1
```

Keep the proxy connected while your conversation still needs archived results:
recall continues after expiry and stop. Removing the proxy from host settings
also removes its recall tool. Archives and history are not automatically
deleted. Undo does not restore previous results inside an existing conversation,
remove the added recall schema, reverse tool actions or refund usage.

A new trial requires a new state directory. Only one proxy process may use a
trial directory. After an unclean process exit, inspect whether the old process
is still running before manually removing a stale `running.lock` directory.

## What the graph proves

The hourly graph compares **serialized result payload bytes** before and after
packing, and includes recall traffic. The journal records transformations at
the proxy output boundary; it cannot prove that the host consumed the output.
It excludes tool schema overhead and request messages. Recall can eliminate
or reverse the apparent byte reduction.

Bytes are not token counts, dollars, or a correctness score. Provider tokenizers,
host history behavior and caching affect actual billed usage. The native usage
graph is collected separately and is not automatically causally matched to the
proxy trial. Use comparable tasks, outcome checks and satisfaction before
claiming a benefit. The [API experiment gate](experiments.md) can reject newly
failed tasks even if cost drops.

## Relationship to SoL-Pi

The archive/excerpt/exact-recall idea is informed by NVlabs'
[ObservationPack](https://github.com/NVlabs/SoL-Pi/tree/d7ecfc089944f0d04b80122a0a9a6ca0d786f3d0/src/sol-pi/extensions/observation-pack).
This is an original Go implementation at an MCP boundary. It does **not**
reproduce Pi's host context hook: results are shortened on return, not after
two provider requests, and past history is not rewritten.

See [design comparison and next steps](sol-pi-design.md).
