package ledger

import "fmt"

// Insights are evidence-based suggestions, not diagnoses or promised savings.
func Insights(r Report) []string {
	total := float64(r.Tokens.Input) + float64(r.Tokens.CachedInput) + float64(r.Tokens.CacheWrite) + float64(r.Tokens.CacheWrite1h)
	notes := []string{}
	if total > 0 {
		notes = append(notes, fmt.Sprintf("Cache reads cover %.1f%% of recorded input tokens. This is token share, not a request cache-hit rate.", 100*float64(r.Tokens.CachedInput)/total))
		if r.Tokens.CachedInput == 0 {
			notes = append(notes, "No cache reads were recorded. If requests share a stable prefix, check your provider's cache eligibility and prefix ordering. A one-off request may have nothing reusable.")
		}
	}
	if r.Tokens.CacheWrite > 0 || r.Tokens.CacheWrite1h > 0 {
		notes = append(notes, "Cache writes were recorded. Check whether later requests reuse them before expiry; writing a cache does not guarantee savings.")
	}
	if r.Tokens.Output > 0 {
		notes = append(notes, "Output tokens are billed separately. Try a shorter requested answer format, then compare cost and task outcomes before adopting it.")
	}
	if r.Unknown > 0 {
		notes = append(notes, "Task quality has not been evaluated for every task. Add evaluator results before calling a cheaper run an optimization.")
	}
	var top *Task
	for i := range r.Tasks {
		if top == nil || r.Tasks[i].CostUSD > top.CostUSD {
			top = &r.Tasks[i]
		}
	}
	if top != nil && r.CostUSD > 0 {
		notes = append(notes, fmt.Sprintf("Start with task %q: it accounts for %.1f%% of estimated token spend across %d calls. Inspect its inputs and outputs before reducing context.", top.ID, 100*top.CostUSD/r.CostUSD, top.Requests))
	}
	return append(notes, "Message history, tool schemas and tool results cannot be separated from usage totals alone. This report does not estimate those categories or measure context-window occupancy.")
}
