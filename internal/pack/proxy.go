package pack

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}
type pending struct{ method, tool string }

// Run transparently relays newline-delimited MCP JSON-RPC over stdio. It
// preserves IDs, notifications, errors and server-to-client requests. Commands
// are explicit argv, never evaluated by a shell.
func Run(ctx context.Context, in io.Reader, out, stderr io.Writer, e *Engine, command []string) error {
	if len(command) == 0 {
		return errors.New("upstream command required")
	}
	lock := filepath.Join(e.Root, "running.lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		return errors.New("trial already running, or stale running.lock; inspect the process before removing the lock")
	}
	defer os.Remove(lock)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Stderr = stderr
	upIn, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	upOut, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	var writes, syncState sync.Mutex
	requests := map[string]pending{}
	readOnly := map[string]bool{}
	send := func(m message) error {
		b, err := json.Marshal(m)
		if err != nil {
			return err
		}
		writes.Lock()
		defer writes.Unlock()
		_, err = out.Write(append(b, '\n'))
		return err
	}
	read := func(reader io.Reader, fn func(message) error) error {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64<<10), 32<<20)
		for scanner.Scan() {
			var m message
			if json.Unmarshal(scanner.Bytes(), &m) != nil || m.JSONRPC != "2.0" {
				return errors.New("invalid MCP JSON-RPC frame")
			}
			if err := fn(m); err != nil {
				return err
			}
		}
		return scanner.Err()
	}
	inputDone := make(chan error, 1)
	go func() {
		err := read(in, func(m message) error {
			if m.Method == "tools/call" {
				var p struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				}
				if json.Unmarshal(m.Params, &p) != nil {
					return errors.New("invalid tool call")
				}
				if p.Name == RecallTool {
					var args struct {
						Handle string `json:"handle"`
						Offset int    `json:"offset"`
						Limit  int    `json:"limit"`
					}
					var result json.RawMessage
					err := json.Unmarshal(p.Arguments, &args)
					if err == nil {
						result, err = e.Recall(args.Handle, args.Offset, args.Limit)
					}
					if err != nil {
						result, _ = json.Marshal(map[string]any{"isError": true, "content": []map[string]string{{"type": "text", "text": "Recall refused: invalid range, missing archive, integrity failure, or journal unavailable."}}})
					}
					return send(message{JSONRPC: "2.0", ID: m.ID, Result: result})
				}
				syncState.Lock()
				requests[string(m.ID)] = pending{method: m.Method, tool: p.Name}
				syncState.Unlock()
			} else if m.Method == "tools/list" {
				syncState.Lock()
				requests[string(m.ID)] = pending{method: m.Method}
				syncState.Unlock()
			}
			b, err := json.Marshal(m)
			if err != nil {
				return err
			}
			_, err = upIn.Write(append(b, '\n'))
			return err
		})
		upIn.Close()
		if err != nil {
			cancel()
		}
		inputDone <- err
	}()
	err = read(upOut, func(m message) error {
		if m.Method != "" {
			if m.Method == "notifications/tools/list_changed" {
				syncState.Lock()
				readOnly = map[string]bool{}
				syncState.Unlock()
			}
			return send(m)
		} // Server-initiated request/notification.
		syncState.Lock()
		p, ok := requests[string(m.ID)]
		delete(requests, string(m.ID))
		readonly := readOnly[p.tool]
		syncState.Unlock()
		if ok && len(m.Error) == 0 && len(m.Result) > 0 {
			if p.method == "tools/list" {
				var result map[string]json.RawMessage
				var tools []json.RawMessage
				if json.Unmarshal(m.Result, &result) != nil || json.Unmarshal(result["tools"], &tools) != nil {
					return errors.New("invalid tools/list response")
				}
				for _, raw := range tools {
					var tool struct {
						Name        string `json:"name"`
						Annotations struct {
							ReadOnly bool `json:"readOnlyHint"`
						} `json:"annotations"`
					}
					if json.Unmarshal(raw, &tool) != nil {
						return errors.New("invalid tool metadata")
					}
					if tool.Name == RecallTool {
						return errors.New("upstream uses reserved tracefrugal_recall tool name")
					}
					syncState.Lock()
					readOnly[tool.Name] = tool.Annotations.ReadOnly
					syncState.Unlock()
				}
				// Advertise recall once, on the final page of a paged list.
				var next string
				json.Unmarshal(result["nextCursor"], &next)
				if next == "" {
					recall, _ := json.Marshal(map[string]any{
						"name": RecallTool, "description": "Read an exact UTF-8 page from a TraceFrugal archived tool result. Follow next_offset until null for the full original. Works after the packing trial stops.",
						"annotations": map[string]bool{"readOnlyHint": true, "destructiveHint": false, "openWorldHint": false},
						"inputSchema": map[string]any{"type": "object", "required": []string{"handle"}, "properties": map[string]any{
							"handle": map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"},
							"offset": map[string]any{"type": "integer", "minimum": 0, "default": 0},
							"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": RecallBytes, "default": RecallBytes},
						}},
					})
					tools = append(tools, recall)
				}
				result["tools"], _ = json.Marshal(tools)
				m.Result, _ = json.Marshal(result)
			} else if p.method == "tools/call" {
				m.Result = e.Transform(p.tool, readonly, m.Result)
			}
		}
		return send(m)
	})
	if err != nil {
		cancel()
	}
	waitErr := cmd.Wait()
	select {
	case inputErr := <-inputDone:
		if err == nil {
			err = inputErr
		}
	default:
	}
	if err != nil {
		return err
	}
	return waitErr
}
