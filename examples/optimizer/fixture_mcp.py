"""Read-only public-data fixture for a real Claude CLI experiment."""
import json
import sys

rows = [
    f"Record {i:04d}: sample inventory entry with status available, category documentation, "
    "owner example-team, location sample-region, note reusable reference material."
    for i in range(1, 181)
]
text = "Synthetic benchmark only. The verification footer is at the end.\n"
text += "\n".join(rows) + "\nVerification footer: CHECK-A = ORCHID-731\n"
for line in sys.stdin:
    message = json.loads(line)
    if "id" not in message:
        continue
    method = message.get("method")
    if method == "initialize":
        result = {"protocolVersion": "2024-11-05", "capabilities": {"tools": {}},
                  "serverInfo": {"name": "tracefrugal-fixture", "version": "1"}}
    elif method == "tools/list":
        result = {"tools": [{"name": "read_inventory",
            "description": "Read the sample inventory, including its verification footer at the end.",
            "inputSchema": {"type": "object", "properties": {}, "additionalProperties": False},
            "annotations": {"readOnlyHint": True}}]}
    elif method == "tools/call":
        result = {"content": [{"type": "text", "text": text}]}
    elif method == "ping":
        result = {}
    else:
        print(json.dumps({"jsonrpc": "2.0", "id": message["id"],
                          "error": {"code": -32601, "message": "Unknown method"}}), flush=True)
        continue
    print(json.dumps({"jsonrpc": "2.0", "id": message["id"], "result": result}), flush=True)
