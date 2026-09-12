package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/niceysam/tracefrugal/internal/pack"
)

func packCommand(action string, args []string, in io.Reader, out, stderr io.Writer) int {
	fs := flag.NewFlagSet(action, flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("state", "", "private trial directory (required)")
	allow := fs.String("allow", "", "comma-separated read-only upstream tool names")
	duration := fs.Duration("duration", 24*time.Hour, "packing duration, at most 24h; restarting never extends it")
	rating := fs.Int("rating", 0, "answer and reasoning satisfaction, 1–5")
	before := fs.Bool("before", false, "record a baseline rating before the first transformation")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	fail := func(err error) int { fmt.Fprintln(stderr, "TraceFrugal:", err); return 2 }
	if *root == "" {
		return fail(fmt.Errorf("--state is required"))
	}
	switch action {
	case "pack":
		tools := strings.Split(*allow, ",")
		for i := range tools {
			tools[i] = strings.TrimSpace(tools[i])
		}
		engine, err := pack.Open(*root, tools, fs.Args(), time.Now().UTC(), *duration)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintln(stderr, "TraceFrugal MCP proxy: explicit allowlist + upstream readOnlyHint required. Pure text only. No model calls.")
		if err := pack.Run(context.Background(), in, out, stderr, engine, fs.Args()); err != nil {
			return fail(err)
		}
	case "pack-status":
		report, err := pack.ReadReport(*root, time.Now())
		if err != nil {
			return fail(err)
		}
		if err := json.NewEncoder(out).Encode(report); err != nil {
			return fail(err)
		}
	case "pack-stop":
		if err := pack.Stop(*root); err != nil {
			return fail(err)
		}
		fmt.Fprintln(out, "Packing stopped. Future results pass through. Recall and history remain available.")
	case "pack-rate":
		if err := pack.Rate(*root, *before, *rating); err != nil {
			return fail(err)
		}
		fmt.Fprintln(out, "Satisfaction recorded. This is your assessment, not an automated quality score.")
	}
	return 0
}
