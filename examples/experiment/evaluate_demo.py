"""Synthetic evaluator: no model calls, no benchmark/savings claims."""
import json
import os
from pathlib import Path

profile = json.loads(Path(os.environ["TRACEFRUGAL_PROFILE"]).read_text())
cap = profile["max_output_tokens"]
events = []
for task in ["explain-error", "extract-fields"]:
    events.append({
        "type": "request", "task_id": task, "request_id": task,
        "model": "demo/model",
        "tokens": {"input": 500, "cached_input": 2000, "output": cap},
    })
    # Deliberately simulates quality loss at a smaller budget.
    events.append({"type": "task_result", "task_id": task, "success": cap >= 256})
Path(os.environ["TRACEFRUGAL_TRACE"]).write_text(
    "".join(json.dumps(event) + "\n" for event in events)
)
