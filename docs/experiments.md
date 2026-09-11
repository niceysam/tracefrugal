# Automatic experiments, hourly results, and rollback

The goal: **see usage → try an optimization → evaluate → apply or reject →
keep a history → roll back when needed**.

TraceFrugal can now generate a smaller output-token-cap profile, run your
evaluator against it, and activate it only when estimated cost per successful
task decreases with no newly failing tasks. This first strategy does not
compress prompts, remove tools, or switch models.

## Try the complete loop without API charges

The fastest way is **`tracefrugal demo`**. Open the printed local URL and use
the restore/resume buttons. It requires only the downloaded binary.
The demo runs its own built-in synthetic evaluator and stores files in a new
`runs/demo-<timestamp>` directory. Stop the dashboard with Ctrl-C; the history
remains. `tracefrugal demo --no-serve` generates the history without a server.

For a worked example of connecting an external evaluator, use the Python fixture:

From a cloned repository with TraceFrugal installed and Python 3 available:

```sh
tracefrugal experiment --config examples/experiment/experiment.json --state runs/demo
tracefrugal experiment --config examples/experiment/experiment.json --state runs/demo
tracefrugal experiment --config examples/experiment/experiment.json --state runs/demo
tracefrugal serve --state runs/demo
```

Open **http://127.0.0.1:8765/**. The synthetic evaluator produces:

| Attempt | Output cap | Expected decision |
|---|---:|---|
| First | 1024 → 512 | Apply: lower estimated cost, both tasks pass |
| Second | 512 → 256 | Apply: lower estimated cost, both tasks pass |
| Third | 256 → 128 | Reject: tasks fail; active cap remains 256 |

The third command exits 1 deliberately. These are fixtures, **not measured
model savings**. The HTML history shows timestamps, costs, decisions, profile
hashes, and a detailed report for each completed evaluation.

Copy the ID of the latest **applied** experiment:

```sh
tracefrugal rollback --state runs/demo --id EXPERIMENT_ID
```

It restores the previous profile byte for byte and pauses automation. A new
rollback event appears in the timeline. It refuses to overwrite intervening
manual or newer profile changes. Roll back applied changes from newest to oldest.
The local dashboard offers the same operation through **Undo last change → Restore & pause**
button for the currently matching profile. Static exported examples have no
controls. **Allow experiments again** clears the pause without starting a scheduler.

## Connect a real agent

Your evaluator command must:

1. Read the JSON file at `TRACEFRUGAL_PROFILE`.
2. Actually apply its settings to the tested requests.
3. Run the same fixed task IDs for both profiles.
4. Write request usage and independently evaluated `task_result` events to
   `TRACEFRUGAL_TRACE`, using the normalized schema.
5. Exit zero when evaluation completes, even when some tasks fail.

For example, map `profile["max_output_tokens"]` to OpenAI Responses'
`max_output_tokens` or Anthropic Messages' `max_tokens`. A smaller cap can
truncate answers instead of improving efficiency: **your quality evaluator
must detect that**. A successful API response is not sufficient.

Use [record_usage.py](../examples/record_usage.py) to record final API responses
and outcomes, passing `trace=os.environ["TRACEFRUGAL_TRACE"]`.

Your real application must separately read `STATE/active.json` before requests
for an activated profile to affect live behavior. TraceFrugal only writes that
managed file. Claude Code has a separate [native usage and context-trial
workflow](claude-code.md), without this task-quality gate. Codex, ChatGPT and
Claude Desktop are not automatically connected.

## Configuration

```json
{
  "baseline_profile": "base.json",
  "output_token_factor": 0.8,
  "min_output_tokens": 256,
  "prices": "prices.json",
  "command": ["python3", "evaluate.py"],
  "timeout_seconds": 300,
  "auto_apply": true
}
```

The first run initializes managed `active.json` from `baseline_profile`.
Subsequent runs use `active.json` as the baseline. The built-in strategy
multiplies its positive integer `max_output_tokens` by the factor, respecting
the floor. At the floor, no further model calls are made.

Alternatively replace `output_token_factor` and `min_output_tokens` with
`"candidate_profile": "candidate.json"` to test your own prompt or cache
configuration. Exactly one candidate strategy must be configured.
Set `auto_apply` to false for recommendations without activation.

Data paths are relative to the config file. The command runs in that directory,
without a shell. Use an absolute executable path when needed. Only run
experiment configs whose commands you trust: commands execute with your
normal local permissions and credentials. Avoid side-effecting tasks during
evaluation; rollback restores settings, not external actions.

## Hourly results

```sh
tracefrugal experiment --config experiment.json --state runs/my-agent --every 1h --max-runs 24
tracefrugal serve --state runs/my-agent
```

Run these in separate terminals. An experiment runs immediately, then on the
interval, with no overlapping evaluations. The scheduler stops after 24 runs
by default, on an error, or on Ctrl-C. Keep the process and machine running.
This is not an installed background service and does not deliver email or Slack.
Each evaluation runs the command twice and can incur model charges; both costs
are shown in its report. They are experiment overhead, not realized savings.

Rollback pauses further experiments. The waiting scheduler exits with a pause
message on its next tick. To start again:

```sh
tracefrugal resume --state runs/my-agent
tracefrugal experiment --config experiment.json --state runs/my-agent --every 1h
```

## Records and recovery

Each run gets a separate directory with timestamps, a decision journal,
before/candidate profile snapshots, frozen prices, usage traces and an HTML
comparison. Error attempts are retained. Profiles and traces are local and
may contain sensitive values you put there; keep state out of public Git.

The state lock prevents concurrent commands from modifying the same state.
After an abrupt process crash, first verify the runner has stopped, then inspect
`active.json` against the journal hashes before removing a stale `.lock`.
A `running` or `applying` journal means the run was interrupted, not completed.

The tool validates task ID equality and supplied pass/fail outcomes. It cannot
prove that a black-box evaluator used the profile correctly or that your
quality checks cover every real-world requirement. Repeated evaluations,
production monitoring and statistically robust comparisons remain your
responsibility.
