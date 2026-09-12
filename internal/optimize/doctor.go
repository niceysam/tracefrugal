// Package optimize manages reversible, project-local Claude Code experiments.
// It does not change models, credentials, permissions, or global MCP settings.
package optimize

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const Limit = "6000"
const Setting = "MAX_MCP_OUTPUT_TOKENS"
const ReviewedVersion = "2.1.268"

type Config struct {
	Project string
	Store   string
	Binary  string
	Self    string
	State   string
}

type Document struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}
type Diagnosis struct {
	Version       string     `json:"version"`
	Binary        string     `json:"binary"`
	Project       string     `json:"project"`
	Target        string     `json:"target"`
	Checked       time.Time  `json:"checked"`
	Documents     []Document `json:"documents"`
	Ready         bool       `json:"ready"`
	Reason        string     `json:"reason"`
	Current       string     `json:"current"`
	Proposed      string     `json:"proposed"`
	ToolSearch    string     `json:"tool_search"`
	Warnings      []string   `json:"warnings"`
	Plan          string     `json:"plan"`
	SettingsHash  string     `json:"-"`
	SettingsExist bool       `json:"-"`
}

func Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func Normalize(c Config) (Config, error) {
	var err error
	for _, p := range []*string{&c.Project, &c.Store, &c.State, &c.Self} {
		if *p == "" {
			return c, errors.New("project, store, state and executable are required")
		}
		*p, err = filepath.Abs(*p)
		if err != nil {
			return c, err
		}
	}
	c.Project, err = filepath.EvalSymlinks(c.Project)
	if err != nil {
		return c, errors.New("choose an existing project directory")
	}
	if info, err := os.Stat(c.Project); err != nil || !info.IsDir() {
		return c, errors.New("project is not a directory")
	}
	if c.Binary == "" {
		c.Binary = "claude"
	}
	c.Binary, err = exec.LookPath(c.Binary)
	if err != nil {
		return c, errors.New("Claude Code not found; use --claude-bin with your CLI path")
	}
	c.Binary, err = filepath.Abs(c.Binary)
	return c, err
}

func target(c Config) string { return filepath.Join(c.Project, ".claude", "settings.local.json") }

// Reject symlink targets and oversized/non-regular settings before reading or
// replacing. The caller explicitly selects the canonical project directory.
func readSettings(c Config) ([]byte, bool, error) {
	dir := filepath.Dir(target(c))
	if i, err := os.Lstat(dir); err == nil && (!i.IsDir() || i.Mode()&os.ModeSymlink != 0) {
		return nil, false, errors.New(".claude must be a real directory, not a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, false, err
	}
	i, err := os.Lstat(target(c))
	if os.IsNotExist(err) {
		return []byte("{}\n"), false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !i.Mode().IsRegular() || i.Size() > 1<<20 {
		return nil, false, errors.New("settings must be a regular file smaller than 1 MiB")
	}
	b, err := os.ReadFile(target(c))
	if err == nil {
		_, err = object(b)
	}
	return b, true, err
}

func object(b []byte) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil || m == nil {
		return nil, errors.New("settings must contain a JSON object")
	}
	return m, nil
}

func envValue(b []byte, key string) string {
	var v struct {
		Env map[string]string `json:"env"`
	}
	if json.Unmarshal(b, &v) != nil {
		return "(invalid env object)"
	}
	return v.Env[key]
}

var versionRE = regexp.MustCompile(`^(\d+\.\d+\.\d+) \(Claude Code\)\s*$`)

func probe(c Config) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Binary, "--version")
	cmd.Dir = c.Project
	b, err := cmd.Output()
	if err != nil || len(b) > 512 {
		return "", errors.New("Claude version probe failed")
	}
	m := versionRE.FindStringSubmatch(string(b))
	if m == nil {
		return "", errors.New("CLI version is not a recognized public Claude Code release")
	}
	return m[1], nil
}

var docSpecs = []struct {
	url     string
	markers []string
}{
	{"https://code.claude.com/docs/en/mcp.md", []string{"MAX_MCP_OUTPUT_TOKENS", "saves it to a file", "anthropic/maxResultSizeChars"}},
	{"https://code.claude.com/docs/en/settings.md", []string{"settings.local.json", "env"}},
	{"https://code.claude.com/docs/en/hooks.md", []string{"SessionStart", "session_id", "cwd"}},
	{"https://raw.githubusercontent.com/anthropics/claude-code/main/CHANGELOG.md", []string{"## " + ReviewedVersion}},
}

// Fetch only fixed official public URLs. No project/configuration data is sent.
// Documentation validates a compiled recipe; downloaded prose is never executed.
func OfficialDocuments(ctx context.Context) ([]Document, error) {
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return errors.New("documentation redirect refused")
	}}
	type result struct {
		d Document
		e error
	}
	results := make(chan result, len(docSpecs))
	for _, spec := range docSpecs {
		go func(url string, markers []string) {
			req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
			if e != nil {
				results <- result{e: e}
				return
			}
			response, e := client.Do(req)
			if e != nil {
				results <- result{e: e}
				return
			}
			defer response.Body.Close()
			b, e := io.ReadAll(io.LimitReader(response.Body, 2<<20))
			if e != nil || response.StatusCode != 200 || len(b) == 2<<20 {
				results <- result{e: errors.New("official document unavailable")}
				return
			}
			for _, marker := range markers {
				if !strings.Contains(string(b), marker) {
					results <- result{e: errors.New("official guidance changed; recipe needs review")}
					return
				}
			}
			results <- result{d: Document{URL: url, SHA256: Hash(string(b))}}
		}(spec.url, spec.markers)
	}
	out := []Document{}
	var first error
	for range docSpecs {
		r := <-results
		if r.e != nil {
			first = r.e
		} else {
			out = append(out, r.d)
		}
	}
	return out, first
}

func Diagnose(c Config, documents []Document, docsErr error, now time.Time) Diagnosis {
	d := Diagnosis{Binary: c.Binary, Project: c.Project, Target: target(c), Checked: now, Documents: documents, Proposed: Limit}
	b, exists, err := readSettings(c)
	if err != nil {
		d.Reason = err.Error()
		return d
	}
	d.SettingsHash, d.SettingsExist = Hash(string(b)), exists
	d.Current = envValue(b, Setting)
	d.ToolSearch = envValue(b, "ENABLE_TOOL_SEARCH")
	// Resolve ordinary settings without exposing unrelated environment values.
	// Shell/managed/--settings overrides are ultimately checked by the hook.
	known := ""
	for _, path := range []string{filepath.Join(c.Store, "settings.json"), filepath.Join(c.Project, ".claude", "settings.json")} {
		if data, e := os.ReadFile(path); e == nil {
			if v := envValue(data, Setting); v != "" {
				known = v
			}
		}
	}
	if d.Current != "" {
		known = d.Current
	}
	if value := os.Getenv(Setting); value != "" {
		d.Warnings = append(d.Warnings, "The launch environment sets the same variable. A session receipt must confirm the effective value.")
		if n, e := strconv.Atoi(value); e == nil && n > 0 && n < 6000 {
			known = value
		}
	}
	if d.Current == "" && known != "" {
		d.Current = known + " (inherited)"
	}
	if d.Current == "" {
		d.Current = "inherited / CLI default"
	}
	if d.ToolSearch == "" {
		d.ToolSearch = "inherited / CLI default; unchanged"
	}
	d.Version, err = probe(c)
	switch {
	case err != nil:
		d.Reason = err.Error()
	case d.Version != ReviewedVersion:
		d.Reason = "This recipe was verified on Claude Code " + ReviewedVersion + ". This CLI version needs a compatibility review; settings were not changed."
	case docsErr != nil || len(documents) != len(docSpecs):
		d.Reason = "Official documentation could not be verified. Retry diagnosis with internet access."
	case d.Current == Limit:
		d.Reason = "This project already has the proposed limit. No additional saving is assumed."
	case d.Current == "(invalid env object)":
		d.Reason = "The existing env object needs review."
	default:
		if n, e := strconv.Atoi(known); e == nil && n > 0 && n <= 6000 {
			d.Reason = "An equal or smaller limit is already configured. No optimization is proposed."
		} else {
			d.Ready = true
			d.Reason = "Preview one project-local change. Start a fresh Claude session after applying."
		}
	}
	d.Plan = Hash(fmt.Sprintf("%s:%s:%t:%s:%s", c.Project, d.SettingsHash, exists, d.Version, Limit))
	return d
}
