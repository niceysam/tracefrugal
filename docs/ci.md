# GitHub Actions integration

Your evaluator must produce the baseline and candidate traces. TraceFrugal does not call models or run the benchmark.

After that evaluator step, use:

```yaml
- uses: actions/setup-go@v5
  with:
    go-version: '1.23.x'
- name: Install TraceFrugal
  run: go install github.com/niceysam/tracefrugal/cmd/tracefrugal@v0.1.1
- name: Check cost and task outcomes
  run: |
    tracefrugal compare \
      --baseline runs/baseline.jsonl \
      --candidate runs/candidate.jsonl \
      --prices benchmark/prices.json \
      --max-increase 5 \
      --format json > comparison.json
- name: Preserve comparison
  if: always()
  uses: actions/upload-artifact@v4
  with:
    name: agent-cost-comparison
    path: comparison.json
```

Pin the CLI version to keep accounting behavior stable. Keep the applicable price book in version control with a descriptive label and review price changes explicitly.

Use a trusted baseline artifact from your default branch or a controlled evaluation job. Replacing both baseline and candidate with identical traces can defeat any comparison tool.

Do not append `|| true` to the comparison step. Exit `1` means regression and `2` means unusable input.

For untrusted pull requests, do not expose provider credentials. Use synthetic or pre-recorded usage fixtures in public CI, and run paid evaluations in a separate trusted environment.
