package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	root := filepath.Join("..", "..", "examples")
	prices := filepath.Join(root, "prices.json")
	baseline := filepath.Join(root, "baseline.jsonl")
	cases := []struct {
		name string
		args []string
		code int
	}{
		{"help", []string{"--help"}, 0},
		{"version", []string{"version"}, 0},
		{"unknown", []string{"nope"}, 2},
		{"missing", []string{"report"}, 2},
		{"format", []string{"report", "--format", "xml"}, 2},
		{"report", []string{"report", "--trace", baseline, "--prices", prices, "--format", "json"}, 0},
		{"pass", []string{"compare", "--baseline", baseline, "--candidate", filepath.Join(root, "candidate-good.jsonl"), "--prices", prices}, 0},
		{"cache regression", []string{"compare", "--baseline", baseline, "--candidate", filepath.Join(root, "candidate-cache-miss.jsonl"), "--prices", prices}, 1},
		{"quality regression", []string{"compare", "--baseline", baseline, "--candidate", filepath.Join(root, "candidate-quality-loss.jsonl"), "--prices", prices, "--format", "json"}, 1},
		{"invalid threshold", []string{"compare", "--baseline", baseline, "--candidate", baseline, "--prices", prices, "--max-increase", "NaN"}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, err bytes.Buffer
			if code := run(tc.args, strings.NewReader(""), &out, &err); code != tc.code {
				t.Fatalf("exit %d want %d, stdout %s, stderr %s", code, tc.code, out.String(), err.String())
			}
			if len(tc.args) > 1 && tc.args[len(tc.args)-1] == "json" && !json.Valid(out.Bytes()) {
				t.Fatalf("invalid JSON: %s", out.String())
			}
		})
	}
}

func TestNormalizeCLIStdin(t *testing.T) {
	var out, err bytes.Buffer
	data := `{"id":"r","model":"m","usage":{"input_tokens":100,"output_tokens":20,"input_tokens_details":{"cached_tokens":80}}}`
	code := run([]string{"normalize", "--provider", "openai", "--task", "t", "--response", "-"}, strings.NewReader(data), &out, &err)
	if code != 0 || !json.Valid(out.Bytes()) || strings.Count(out.String(), "\n") != 1 {
		t.Fatalf("normalize failed: %d %s %s", code, out.String(), err.String())
	}
}
