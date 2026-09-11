package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/niceysam/tracefrugal/internal/experiment"
	"github.com/niceysam/tracefrugal/internal/ledger"
)

// demoEvaluator is an internal synthetic subprocess, never a model invocation.
func demoEvaluator(stderr io.Writer) int {
	raw, err := os.ReadFile(os.Getenv("TRACEFRUGAL_PROFILE"))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	var profile struct {
		Cap int64 `json:"max_output_tokens"`
	}
	if json.Unmarshal(raw, &profile) != nil || profile.Cap < 1 {
		return 2
	}
	f, err := os.OpenFile(os.Getenv("TRACEFRUGAL_TRACE"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return 2
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	for _, id := range []string{"explain-error", "extract-fields"} {
		e := ledger.Event{Type: "request", TaskID: id, RequestID: id, Model: "demo/model", Tokens: &ledger.Tokens{Input: 500, CachedInput: 2000, Output: profile.Cap}}
		if err = encoder.Encode(e); err != nil {
			return 2
		}
		success := profile.Cap >= 256
		if err = encoder.Encode(ledger.Event{Type: "task_result", TaskID: id, Success: &success}); err != nil {
			return 2
		}
	}
	return 0
}

func demo(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.SetOutput(stderr)
	port := fs.Int("port", 8765, "local dashboard port")
	noServe := fs.Bool("no-serve", false, "generate the demo without starting the dashboard")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 || *port < 1 || *port > 65535 {
		return 2
	}
	root := filepath.Join("runs", "demo-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	root, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	binary, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	factor := 0.5
	config := experiment.Config{BaselineProfile: "base.json", Prices: "prices.json", Command: []string{binary, "demo-evaluate"}, TimeoutSeconds: 30, AutoApply: true, OutputTokenFactor: &factor, MinOutputTokens: 64}
	configBytes, _ := json.MarshalIndent(config, "", "  ")
	for name, data := range map[string][]byte{
		"base.json":       []byte(`{"max_output_tokens":1024}`),
		"prices.json":     []byte(`{"schema_version":1,"currency":"USD","label":"SYNTHETIC demo rates — no model calls","models":{"demo/model":{"input":5,"cached_input":0.5,"output":15}}}`),
		"experiment.json": configBytes,
		"synthetic-demo":  []byte("Synthetic demonstration. No real model calls or savings.\n"),
	} {
		if err = os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
	}
	fmt.Fprintln(stdout, "TraceFrugal demo — synthetic usage, no API key, no model calls.")
	for i := 0; i < 3; i++ {
		e, err := experiment.Run(context.Background(), filepath.Join(root, "experiment.json"), root)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		fmt.Fprintf(stdout, "Experiment %d: %s\n", i+1, e.Status)
	}
	fmt.Fprintf(stdout, "Demo files: %s\n", root)
	fmt.Fprintln(stdout, "Try the restore button in the browser. It changes only this demo's profile.")
	if *noServe {
		return 0
	}
	return serve([]string{"--state", root, "--port", fmt.Sprint(*port)}, stderr)
}
