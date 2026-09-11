import json
import tempfile
import unittest
from pathlib import Path
from record_usage import record_response, record_outcome


class RecorderTest(unittest.TestCase):
    def test_roundtrip_does_not_store_content(self):
        with tempfile.TemporaryDirectory() as tmp:
            trace = Path(tmp) / "run.jsonl"
            record_response("openai", "task-1", {
                "id": "r1", "model": "test",
                "output": "PRIVATE ANSWER",
                "usage": {"input_tokens": 100, "output_tokens": 10,
                          "input_tokens_details": {"cached_tokens": 60}},
            }, trace)
            record_response("anthropic", "task-2", {
                "id": "r2", "model": "test",
                "content": "PRIVATE ANSWER",
                "usage": {"input_tokens": 40, "output_tokens": 10,
                          "cache_read_input_tokens": 60},
            }, trace)
            record_outcome("task-1", True, trace)
            text = trace.read_text()
            self.assertNotIn("PRIVATE", text)
            rows = [json.loads(line) for line in text.splitlines()]
            self.assertEqual(rows[0]["tokens"], rows[1]["tokens"])
            self.assertEqual(rows[0]["tokens"]["input"], 40)
            self.assertTrue(rows[2]["success"])


if __name__ == "__main__":
    unittest.main()
