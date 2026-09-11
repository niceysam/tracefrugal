package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/niceysam/tracefrugal/internal/experiment"
)

func experimentCommand(action string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet(action, flag.ContinueOnError)
	fs.SetOutput(stderr)
	state := fs.String("state", "", "managed local state directory")
	var config, id string
	var every time.Duration
	maxRuns := 1
	if action == "experiment" {
		fs.StringVar(&config, "config", "", "experiment config with evaluator command")
		fs.DurationVar(&every, "every", 0, "repeat interval, e.g. 1h; foreground process")
		fs.IntVar(&maxRuns, "max-runs", 24, "maximum evaluations per recurring invocation")
	}
	if action == "rollback" {
		fs.StringVar(&id, "id", "", "applied experiment ID")
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 || *state == "" || (action == "experiment" && config == "") || every < 0 || (every > 0 && every < time.Minute) || maxRuns < 1 {
		fmt.Fprintln(stderr, "requires --state, --config for experiment, and --every of at least 1m if supplied")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	encoder := json.NewEncoder(stdout)
	var ticks <-chan time.Time
	if every > 0 {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		ticks = ticker.C
		fmt.Fprintln(stderr, "Recurring evaluations execute your command twice and may incur API charges. Keep this process running; Ctrl-C stops it.")
	}
	for runNumber := 1; ; runNumber++ {
		var e experiment.Entry
		var err error
		switch action {
		case "experiment":
			e, err = experiment.Run(ctx, config, *state)
		case "rollback":
			e, err = experiment.Rollback(*state, id)
		case "resume":
			e, err = experiment.Resume(*state)
		}
		if e.ID != "" {
			if writeErr := encoder.Encode(e); writeErr != nil {
				fmt.Fprintln(stderr, writeErr)
				return 2
			}
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		if every == 0 || runNumber >= maxRuns {
			if e.Status == "rejected" {
				return 1
			}
			return 0
		}
		select {
		case <-ctx.Done():
			return 0
		case <-ticks:
		}
	}
}
