// TraceFrugal compares agent runs by estimated cost per successful task.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/niceysam/tracefrugal/internal/ledger"
)

var version = "dev"

const help = `TraceFrugal — fewer tokens is not always a cheaper agent.

Usage:
  tracefrugal report --trace run.jsonl --prices prices.json [--format text|json|html]
  tracefrugal compare --baseline before.jsonl --candidate after.jsonl --prices prices.json [--max-increase 5] [--format text|json|html]
  tracefrugal normalize --provider openai|anthropic --task TASK --response response.json [--request-id ID]
  tracefrugal serve --trace run.jsonl --prices prices.json [--port 8765]
  tracefrugal serve --state runs/state [--port 8765]
  tracefrugal experiment --config experiment.json --state runs/state [--every 1h]
  tracefrugal rollback --state runs/state --id EXPERIMENT_ID
  tracefrugal resume --state runs/state
  tracefrugal version

Use "-" as an input path to read stdin (one input only).
Prices are explicit USD per million tokens. No built-in price guesses.
Exit codes: 0 success, 1 regression, 2 invalid input or usage.
`

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintln(stderr, "tracefrugal:", err)
		return 2
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(stdout, help)
		return 0
	}
	if args[0] == "version" {
		resolved := version
		if resolved == "dev" {
			if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
				resolved = info.Main.Version
			}
		}
		fmt.Fprintln(stdout, resolved)
		return 0
	}
	if args[0] == "serve" {
		return serve(args[1:], stderr)
	}
	if args[0] == "experiment" || args[0] == "rollback" || args[0] == "resume" {
		return experimentCommand(args[0], args[1:], stdout, stderr)
	}
	if args[0] != "report" && args[0] != "compare" && args[0] != "normalize" {
		return fail(fmt.Errorf("unknown command %q; run tracefrugal --help", args[0]))
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	var trace, prices, baseline, candidate, format, provider, task, response, requestID string
	var threshold float64
	if args[0] == "normalize" {
		fs.StringVar(&provider, "provider", "", "openai or anthropic")
		fs.StringVar(&task, "task", "", "stable benchmark task ID")
		fs.StringVar(&response, "response", "", "one final response JSON file")
		fs.StringVar(&requestID, "request-id", "", "override response ID")
	} else {
		fs.StringVar(&prices, "prices", "", "explicit price book")
		fs.StringVar(&format, "format", "text", "text, json or html")
		if args[0] == "report" {
			fs.StringVar(&trace, "trace", "", "normalized JSONL trace")
		} else {
			fs.StringVar(&baseline, "baseline", "", "baseline JSONL")
			fs.StringVar(&candidate, "candidate", "", "candidate JSONL")
			fs.Float64Var(&threshold, "max-increase", 5, "allowed percent increase in cost per success")
		}
	}
	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		return fail(fmt.Errorf("unexpected positional arguments"))
	}
	stdinUsed := false
	open := func(path string) (io.ReadCloser, error) {
		if path == "" {
			return nil, fmt.Errorf("required input path is missing; run tracefrugal %s -h", args[0])
		}
		if path == "-" {
			if stdinUsed {
				return nil, fmt.Errorf("stdin can be used for only one input")
			}
			stdinUsed = true
			return io.NopCloser(stdin), nil
		}
		return os.Open(path)
	}
	emit := func(v any) error {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(v)
	}
	if args[0] == "normalize" {
		f, err := open(response)
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		event, err := ledger.Normalize(f, provider, task, requestID)
		if err != nil {
			return fail(err)
		}
		// JSONL: one compact event per line.
		if err := json.NewEncoder(stdout).Encode(event); err != nil {
			return fail(err)
		}
		return 0
	}
	if format != "text" && format != "json" && format != "html" {
		return fail(fmt.Errorf("format must be text, json or html"))
	}
	f, err := open(prices)
	if err != nil {
		return fail(err)
	}
	p, err := ledger.ReadPrices(f)
	f.Close()
	if err != nil {
		return fail(err)
	}
	analyze := func(path string) (ledger.Report, error) {
		file, err := open(path)
		if err != nil {
			return ledger.Report{}, err
		}
		defer file.Close()
		return ledger.Analyze(file, p)
	}
	if args[0] == "report" {
		r, err := analyze(trace)
		if err != nil {
			return fail(err)
		}
		if format == "json" {
			if err := emit(r); err != nil {
				return fail(err)
			}
		} else if format == "html" {
			if err := ledger.WriteReportHTML(stdout, r); err != nil {
				return fail(err)
			}
		} else {
			printReport(stdout, r)
		}
		return 0
	}
	a, err := analyze(baseline)
	if err != nil {
		return fail(fmt.Errorf("baseline: %w", err))
	}
	b, err := analyze(candidate)
	if err != nil {
		return fail(fmt.Errorf("candidate: %w", err))
	}
	c, err := ledger.Compare(a, b, threshold)
	if err != nil {
		return fail(err)
	}
	if format == "json" {
		if err := emit(c); err != nil {
			return fail(err)
		}
	} else if format == "html" {
		if err := ledger.WriteComparisonHTML(stdout, c); err != nil {
			return fail(err)
		}
	} else {
		label := "PASS"
		if !c.Passed {
			label = "FAIL"
		}
		fmt.Fprintf(stdout, "%s — cost and task-outcome regression gate\n", label)
		fmt.Fprintf(stdout, "Prices: %s (USD per million tokens)\n", p.Label)
		fmt.Fprintf(stdout, "Baseline:  $%.6f | %d/%d successful | %d requests\n", a.CostUSD, a.Successes, len(a.Tasks), a.Requests)
		fmt.Fprintf(stdout, "Candidate: $%.6f | %d/%d successful | %d requests\n", b.CostUSD, b.Successes, len(b.Tasks), b.Requests)
		if c.CostChangePct != nil {
			fmt.Fprintf(stdout, "Cost per success: %+.2f%% (allowed increase: %.2f%%)\n", *c.CostChangePct, threshold)
		}
		for _, reason := range c.Reasons {
			fmt.Fprintln(stdout, "- "+reason)
		}
		for _, id := range c.NewlyFailedTasks {
			fmt.Fprintln(stdout, "  Newly failed task: "+id)
		}
		fmt.Fprintln(stdout, "Estimated token cost only; not an invoice or a statistical significance test.")
	}
	if !c.Passed {
		return 1
	}
	return 0
}

func printReport(w io.Writer, r ledger.Report) {
	fmt.Fprintf(w, "TraceFrugal — %s\n", r.PriceLabel)
	fmt.Fprintf(w, "Requests: %d | Tasks: %d | Success: %d | Failed: %d | Unknown: %d\n",
		r.Requests, len(r.Tasks), r.Successes, r.Failures, r.Unknown)
	fmt.Fprintf(w, "Input: %d uncached + %d cached + %d cache-write + %d cache-write-1h\n",
		r.Tokens.Input, r.Tokens.CachedInput, r.Tokens.CacheWrite, r.Tokens.CacheWrite1h)
	fmt.Fprintf(w, "Output: %d\nEstimated token cost: $%.6f USD\n", r.Tokens.Output, r.CostUSD)
	if r.CostPerSuccess != nil {
		fmt.Fprintf(w, "Estimated cost per success: $%.6f USD\n", *r.CostPerSuccess)
	} else {
		fmt.Fprintln(w, "Cost per success: unavailable (missing outcomes or zero successes)")
	}
	fmt.Fprintln(w, "Includes failed-task spend. Excludes tool charges, infrastructure and taxes.")
}
