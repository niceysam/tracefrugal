package ledger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Normalize accepts one final, non-streaming provider response, not a session log.
// It deliberately does not infer task success from an HTTP or model response.
func Normalize(r io.Reader, provider, taskID, requestID string) (Event, error) {
	var raw struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage *struct {
			Input        *int64 `json:"input_tokens"`
			Output       *int64 `json:"output_tokens"`
			InputDetails *struct {
				Cached              int64  `json:"cached_tokens"`
				CacheWrite          int64  `json:"cache_write_tokens"`
				UnsupportedCreation *int64 `json:"cache_creation_tokens"`
			} `json:"input_tokens_details"`
			CacheRead  *int64 `json:"cache_read_input_tokens"`
			CacheWrite *int64 `json:"cache_creation_input_tokens"`
			Creation   *struct {
				FiveMinutes *int64 `json:"ephemeral_5m_input_tokens"`
				OneHour     *int64 `json:"ephemeral_1h_input_tokens"`
			} `json:"cache_creation"`
		} `json:"usage"`
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxLineBytes+1))
	if err != nil {
		return Event{}, err
	}
	if len(data) > MaxLineBytes {
		return Event{}, fmt.Errorf("response exceeds %d bytes", MaxLineBytes)
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(&raw); err != nil {
		return Event{}, fmt.Errorf("provider response: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return Event{}, fmt.Errorf("expected one final provider response, not JSONL or stream events")
	}
	if taskID == "" || raw.Model == "" || raw.Usage == nil || raw.Usage.Input == nil || raw.Usage.Output == nil {
		return Event{}, fmt.Errorf("requires task ID, response model, usage.input_tokens and usage.output_tokens")
	}
	if requestID == "" {
		requestID = raw.ID
	}
	if requestID == "" {
		return Event{}, fmt.Errorf("response has no id; supply --request-id")
	}
	u := raw.Usage
	t := Tokens{Input: *u.Input, Output: *u.Output}
	switch provider {
	case "openai":
		if u.CacheRead != nil || u.CacheWrite != nil || u.Creation != nil {
			return Event{}, fmt.Errorf("unexpected cache fields for OpenAI; normalize this billing schema explicitly")
		}
		if u.InputDetails != nil {
			if u.InputDetails.UnsupportedCreation != nil {
				return Event{}, fmt.Errorf("unsupported cache_creation_tokens field; expected cache_write_tokens")
			}
			t.CachedInput = u.InputDetails.Cached
			if t.CachedInput < 0 || t.CachedInput > t.Input {
				return Event{}, fmt.Errorf("cached_tokens must be between zero and input_tokens")
			}
			t.Input -= t.CachedInput
			t.CacheWrite = u.InputDetails.CacheWrite
			if t.CacheWrite < 0 || t.CacheWrite > t.Input {
				return Event{}, fmt.Errorf("cached_tokens + cache_write_tokens must not exceed input_tokens")
			}
			t.Input -= t.CacheWrite
		}
	case "anthropic":
		if u.InputDetails != nil {
			return Event{}, fmt.Errorf("unexpected input_tokens_details for Anthropic")
		}
		if u.CacheRead != nil {
			t.CachedInput = *u.CacheRead
		}
		if u.CacheWrite != nil && *u.CacheWrite < 0 {
			return Event{}, fmt.Errorf("cache_creation_input_tokens must be nonnegative")
		}
		if u.Creation != nil {
			if u.Creation.FiveMinutes == nil || u.Creation.OneHour == nil {
				return Event{}, fmt.Errorf("cache_creation requires both 5m and 1h counts")
			}
			t.CacheWrite = *u.Creation.FiveMinutes
			t.CacheWrite1h = *u.Creation.OneHour
			if t.CacheWrite < 0 || t.CacheWrite1h < 0 || t.CacheWrite > int64(^uint64(0)>>1)-t.CacheWrite1h {
				return Event{}, fmt.Errorf("invalid cache creation counts")
			}
			if u.CacheWrite != nil && *u.CacheWrite != t.CacheWrite+t.CacheWrite1h {
				return Event{}, fmt.Errorf("cache creation total does not match 5m + 1h counts")
			}
		} else if u.CacheWrite != nil && *u.CacheWrite != 0 {
			return Event{}, fmt.Errorf("cache write TTL is unknown; provide the 5m/1h breakdown or normalize manually")
		}
	default:
		return Event{}, fmt.Errorf("provider must be openai or anthropic")
	}
	for _, n := range t.values() {
		if n < 0 {
			return Event{}, fmt.Errorf("token counts must be nonnegative")
		}
	}
	return Event{Type: "request", TaskID: taskID, RequestID: requestID,
		Model: provider + "/" + raw.Model, Tokens: &t}, nil
}
