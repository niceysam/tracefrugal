# Reproduce a bounded local optimizer trial

This optional fixture needs Python 3 and an already authenticated Claude Code.
Running Claude **makes paid/model-plan calls**. TraceFrugal itself makes none.
Keep your normal permission controls. Do not upload settings or raw logs.

1. Put `fixture_mcp.py` in a stable local folder. It returns only generated sample
   inventory and the footer `CHECK-A = ORCHID-731`. The checked-in version returns
   180 records; the validation report also records earlier 350/100-record pilots.
2. In a clean scratch project, create `fixture.json` with **your actual paths**:

   ```json
   {"mcpServers":{"fixture":{"command":"/path/to/python3","args":["/path/to/fixture_mcp.py"]}}}
   ```

3. Run a fresh session before applying. Select your normal model consistently
   and preserve the same prompt/settings for the after run:

   ```sh
   claude --print --output-format json --max-turns 6 --max-budget-usd 1 \
     --strict-mcp-config --mcp-config fixture.json \
     --tools Read,Bash \
     --allowedTools 'Read,Bash(python3 *),mcp__fixture__read_inventory' -- \
     "Call the fixture read_inventory tool once. Find its verification footer. If the result is stored in a file, use Bash with python3 to load it: try JSON decoding and extract its text content, falling back to plain text if JSON decoding fails. Print only the final footer line. JSON file line offsets are not content line offsets. Your final answer must be exactly the value of CHECK-A, with no explanation or note. Do not infer the value." > before.json
   ```

   These are CLI turn/budget options, not an external hard billing cap.
   This permission scope is for the authored fixture in a scratch directory.
4. Run `tracefrugal optimize --project /path/to/scratch-project`. Choose the
   same CLI binary/store using its flags if needed. Click Apply.
5. Repeat step 3 as a fresh session, saving `after.json`. Check `result` is
   **exactly** `ORCHID-731`, not merely that the process exited successfully.
   Compare root usage, model responses, model identity and elapsed time.
6. Click Restore and run once more. Confirm the original settings are restored.
   Include failures and all retries in your evaluation.

Use the dashboard for observational history, not a controlled task-matched
savings claim. It includes all eligible work during each observation window.
The checked-in `measured-2026-09-12.csv` records every completed maintainer model
run, including the failed pilots; `validation.json` states the tested scope.
