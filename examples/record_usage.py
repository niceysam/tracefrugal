"""Single-process SDK recorder. Uses only the standard library and TraceFrugal.

Call record_response("openai", task_id, response) after responses.create(), or
record_response("anthropic", task_id, message) after messages.create().
For streaming, pass the SDK's assembled final response, never token deltas.
Only model, ID and usage leave this function. Prompts and answers are not saved.
"""
import json
import subprocess
import threading
from pathlib import Path

_lock = threading.Lock()


def record_response(provider, task_id, response, trace="run.jsonl"):
    raw = response if isinstance(response, dict) else response.model_dump(mode="json")
    usage_only = {key: raw.get(key) for key in ("id", "model", "usage")}
    result = subprocess.run(
        ["tracefrugal", "normalize", "--provider", provider, "--task", task_id,
         "--response", "-"],
        input=json.dumps(usage_only), text=True, capture_output=True, check=True,
    )
    _append(trace, json.loads(result.stdout))


def record_outcome(task_id, success, trace="run.jsonl"):
    """Call once per task, after your evaluator runs; HTTP success is not quality."""
    if not isinstance(success, bool):
        raise TypeError("success must be a bool from your evaluator")
    _append(trace, {"type": "task_result", "task_id": task_id, "success": success})


def _append(trace, event):
    # Thread-safe in this process. Use one file per process for multiple workers.
    with _lock:
        with Path(trace).open("a", encoding="utf-8") as stream:
            stream.write(json.dumps(event, separators=(",", ":")) + "\n")
