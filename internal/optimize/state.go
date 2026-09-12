package optimize

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/niceysam/tracefrugal/internal/claude"
)

type Trial struct {
	ID           string     `json:"id"`
	Status       string     `json:"status"`
	Version      string     `json:"version"`
	Started      time.Time  `json:"started"`
	Ended        *time.Time `json:"ended,omitempty"`
	BeforeHash   string     `json:"before_hash"`
	AfterHash    string     `json:"after_hash"`
	BeforeExists bool       `json:"before_exists"`
	BeforeMode   uint32     `json:"before_mode"`
	BeforeRating int        `json:"before_rating"`
	AfterRating  int        `json:"after_rating"`
	Outcome      string     `json:"outcome"`
	Documents    []Document `json:"documents"`
	Events       []Event    `json:"events"`
}
type Event struct {
	Time   time.Time `json:"time"`
	Action string    `json:"action"`
}
type journal struct {
	Project string  `json:"project_hash"`
	Trials  []Trial `json:"trials"`
}
type Receipt struct {
	Session string    `json:"session"`
	Time    time.Time `json:"time"`
	Limit   string    `json:"limit"`
	Source  string    `json:"source"`
}

var idRE = regexp.MustCompile(`^[a-f0-9]{24}$`)

func privateDir(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("state must be a real directory")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.MkdirAll(path, 0700)
}
func atomic(path string, b []byte) error {
	if err := privateDir(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tracefrugal-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func save(c Config, j journal) error {
	b, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return atomic(filepath.Join(c.State, "history.json"), b)
}
func load(c Config) (journal, error) {
	j := journal{Project: Hash(c.Project), Trials: []Trial{}}
	b, err := os.ReadFile(filepath.Join(c.State, "history.json"))
	if os.IsNotExist(err) {
		return j, nil
	}
	if err != nil {
		return j, err
	}
	if len(b) > 4<<20 || json.Unmarshal(b, &j) != nil || j.Project != Hash(c.Project) {
		return j, errors.New("invalid history or state belongs to another project")
	}
	for _, t := range j.Trials {
		if !idRE.MatchString(t.ID) {
			return j, errors.New("invalid trial ID in history")
		}
	}
	return j, nil
}
func locked(c Config, action func() error) error {
	if err := privateDir(c.State); err != nil {
		return err
	}
	p := filepath.Join(c.State, ".lock")
	if err := os.Mkdir(p, 0700); err != nil {
		return errors.New("optimizer is busy; after an interrupted process, inspect history before removing its .lock")
	}
	defer os.Remove(p)
	return action()
}

func shellQuote(s string) (string, error) {
	if strings.ContainsAny(s, "\r\n\x00") {
		return "", errors.New("unsupported newline in path")
	}
	if runtime.GOOS == "windows" {
		if strings.ContainsAny(s, "\"%!\u0026|<>^") {
			return "", errors.New("Windows hook paths cannot contain shell metacharacters")
		}
		return `"` + s + `"`, nil
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'", nil
}
func afterSettings(c Config, before []byte, id string) ([]byte, error) {
	m, err := object(before)
	if err != nil {
		return nil, err
	}
	env := map[string]json.RawMessage{}
	if b, ok := m["env"]; ok && (json.Unmarshal(b, &env) != nil || env == nil) {
		return nil, errors.New("env must be an object")
	}
	env[Setting], _ = json.Marshal(Limit)
	m["env"], _ = json.Marshal(env)
	hooks := map[string]json.RawMessage{}
	if b, ok := m["hooks"]; ok && (json.Unmarshal(b, &hooks) != nil || hooks == nil) {
		return nil, errors.New("hooks must be an object")
	}
	start := []json.RawMessage{}
	if b, ok := hooks["SessionStart"]; ok && json.Unmarshal(b, &start) != nil {
		return nil, errors.New("SessionStart hooks must be an array")
	}
	binary, err := shellQuote(c.Self)
	if err != nil {
		return nil, err
	}
	state, err := shellQuote(c.State)
	if err != nil {
		return nil, err
	}
	hook, _ := json.Marshal(map[string]any{"matcher": "startup|resume|clear|compact|fork", "hooks": []any{
		map[string]any{"type": "command", "command": binary + " optimizer-receipt --state " + state + " --id " + id, "timeout": 5},
	}})
	start = append(start, hook)
	hooks["SessionStart"], _ = json.Marshal(start)
	m["hooks"], _ = json.Marshal(hooks)
	out, err := json.MarshalIndent(m, "", "  ")
	return append(out, '\n'), err
}

// Apply is a compare-and-swap against the preview. Snapshots and intent are
// durable before the settings file changes. Interrupted commits are recoverable.
func Apply(c Config, d Diagnosis, plan string, rating int, baseline []claude.Request, now time.Time) (Trial, error) {
	var t Trial
	err := locked(c, func() error {
		if !d.Ready || plan != d.Plan || now.Sub(d.Checked) > 15*time.Minute || rating < 0 || rating > 5 {
			return errors.New("refresh diagnosis; ratings must be 1–5 or left unmeasured")
		}
		version, err := probe(c)
		if err != nil || version != d.Version {
			return errors.New("Claude changed since diagnosis; refresh the preview")
		}
		j, err := load(c)
		if err != nil {
			return err
		}
		for _, old := range j.Trials {
			if old.Status != "restored" {
				return errors.New("restore or recover the existing trial first")
			}
		}
		before, exists, err := readSettings(c)
		if err != nil || exists != d.SettingsExist || Hash(string(before)) != d.SettingsHash {
			return errors.New("settings changed since preview; refresh diagnosis")
		}
		t = Trial{ID: Hash(now.UTC().Format(time.RFC3339Nano) + d.Plan)[:24], Status: "prepared", Version: version,
			Started: now, BeforeHash: Hash(string(before)), BeforeExists: exists, BeforeRating: rating,
			Documents: d.Documents, Outcome: "unknown", Events: []Event{{Time: now, Action: "prepared"}}}
		if exists {
			info, err := os.Stat(target(c))
			if err != nil {
				return err
			}
			t.BeforeMode = uint32(info.Mode().Perm())
		}
		after, err := afterSettings(c, before, t.ID)
		if err != nil {
			return err
		}
		t.AfterHash = Hash(string(after))
		for name, b := range map[string][]byte{"before.json": before, "after.json": after} {
			if err := atomic(filepath.Join(c.State, t.ID, name), b); err != nil {
				return err
			}
		}
		if err := SaveBaseline(c, t.ID, baseline, now); err != nil {
			return err
		}
		j.Trials = append(j.Trials, t)
		if err := save(c, j); err != nil {
			return err
		}
		current, currentExists, err := readSettings(c)
		if err != nil || currentExists != exists || Hash(string(current)) != t.BeforeHash {
			return errors.New("settings changed during apply; recover the prepared trial")
		}
		if err := atomic(target(c), after); err != nil {
			return err
		}
		t.Status = "active"
		t.Events = append(t.Events, Event{Time: now, Action: "applied"})
		j.Trials[len(j.Trials)-1] = t
		return save(c, j)
	})
	return t, err
}

// Restore restores exact original bytes, or removes the newly created file.
// Intervening manual edits are never overwritten. Reapplying creates a new trial.
func Restore(c Config, id string, now time.Time) error {
	return locked(c, func() error {
		j, err := load(c)
		if err != nil {
			return err
		}
		for i := range j.Trials {
			t := &j.Trials[i]
			if t.ID != id {
				continue
			}
			if t.Status == "restored" {
				return nil
			}
			current, exists, err := readSettings(c)
			if err != nil {
				return err
			}
			sameBefore := exists == t.BeforeExists && Hash(string(current)) == t.BeforeHash
			if !sameBefore && (!exists || Hash(string(current)) != t.AfterHash) {
				return errors.New("settings were edited elsewhere; automatic restore refused to preserve those edits")
			}
			before, err := os.ReadFile(filepath.Join(c.State, id, "before.json"))
			if err != nil || Hash(string(before)) != t.BeforeHash {
				return errors.New("backup is missing or changed")
			}
			t.Status = "restoring"
			if err := save(c, j); err != nil {
				return err
			}
			if !sameBefore {
				if t.BeforeExists {
					err = atomic(target(c), before)
					if err == nil && t.BeforeMode != 0 {
						err = os.Chmod(target(c), os.FileMode(t.BeforeMode))
					}
				} else {
					err = os.Remove(target(c))
				}
				if err != nil {
					return err
				}
			}
			t.Status, t.Ended = "restored", &now
			t.Events = append(t.Events, Event{Time: now, Action: "restored"})
			return save(c, j)
		}
		return errors.New("trial not found")
	})
}

func Rate(c Config, id string, rating int, outcome string, now time.Time) error {
	if rating < 1 || rating > 5 || outcome != "good" && outcome != "regression" && outcome != "unknown" && outcome != "keep" {
		return errors.New("choose a rating and outcome")
	}
	return locked(c, func() error {
		j, err := load(c)
		if err != nil {
			return err
		}
		for i := range j.Trials {
			if j.Trials[i].ID == id {
				action := "rated"
				if outcome == "keep" {
					if j.Trials[i].Status != "active" {
						return errors.New("only an active setting can be kept")
					}
					outcome, action = "good", "kept"
				}
				j.Trials[i].AfterRating, j.Trials[i].Outcome = rating, outcome
				j.Trials[i].Events = append(j.Trials[i].Events, Event{Time: now, Action: action})
				return save(c, j)
			}
		}
		return errors.New("trial not found")
	})
}

// Record runs as a SessionStart hook, emits no text, and never opens the supplied
// transcript path. Only hashed identity and the effective setting are retained.
func Record(state, id string, input io.Reader, effective string, now time.Time) error {
	if !idRE.MatchString(id) {
		return errors.New("invalid trial")
	}
	var event struct {
		Session string `json:"session_id"`
		CWD     string `json:"cwd"`
		Source  string `json:"source"`
		Event   string `json:"hook_event_name"`
	}
	if json.NewDecoder(io.LimitReader(input, 64<<10)).Decode(&event) != nil || event.Session == "" || event.Event != "SessionStart" {
		return errors.New("invalid SessionStart event")
	}
	project, err := filepath.EvalSymlinks(event.CWD)
	if err != nil {
		return err
	}
	c := Config{Project: project, State: state}
	return locked(c, func() error {
		j, err := load(c)
		if err != nil {
			return err
		}
		for _, t := range j.Trials {
			if t.ID == id && t.Status == "active" {
				r := Receipt{Session: Hash(event.Session)[:16], Time: now, Limit: effective, Source: event.Source}
				b, _ := json.Marshal(r)
				// One receipt per start: resume never overwrites the earlier boundary.
				name := fmt.Sprintf("%d-%s.json", now.UnixNano(), r.Session)
				return atomic(filepath.Join(state, id, "receipts", name), b)
			}
		}
		return errors.New("trial is not active")
	})
}
