package conversationarchive

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

// Snapshot retains text/tool structures, omitting embedded media and credentials.
// Invalid or oversized bodies are omitted in full, never stored as raw previews.
func Snapshot(raw []byte, truncated bool) json.RawMessage {
	var value any
	if truncated || !decodeJSON(raw, &value) {
		return json.RawMessage(`{"omitted":true}`)
	}
	data, err := json.Marshal(sanitize(value, 0))
	if err != nil {
		return json.RawMessage(`{"omitted":true}`)
	}
	return data
}

func decodeJSON(raw []byte, target any) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(target) != nil {
		return false
	}
	var extra any
	return decoder.Decode(&extra) == io.EOF
}

// ResponseHasError recognizes OpenAI-compatible error envelopes even when an
// upstream incorrectly returns HTTP 200. Nested tool-result fields are not errors.
func ResponseHasError(raw []byte) bool {
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	return json.Unmarshal(raw, &envelope) == nil && len(envelope.Error) > 0 && !bytes.Equal(bytes.TrimSpace(envelope.Error), []byte("null"))
}

func sanitize(v any, depth int) any {
	if depth > 64 {
		return "[depth limit]"
	}
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, item := range x {
			k = strings.ReplaceAll(k, "\x00", "\uFFFD")
			key := strings.ToLower(strings.ReplaceAll(k, "-", "_"))
			switch key {
			case "authorization", "api_key", "access_token", "refresh_token", "password", "client_secret", "token":
				out[k] = "[redacted]"
			case "image", "image_url", "input_audio", "audio", "file_data", "file_url", "inline_data", "inlinedata":
				out[k] = "[media omitted]"
			default:
				out[k] = sanitize(item, depth+1)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = sanitize(item, depth+1)
		}
		return out
	case string:
		if strings.HasPrefix(strings.TrimSpace(x), "data:") {
			return "[media omitted]"
		}
		// PostgreSQL JSONB cannot represent U+0000, even when escaped in JSON.
		return strings.ReplaceAll(x, "\x00", "\uFFFD")
	default:
		return v
	}
}

// StreamSnapshot parses complete SSE events, including events split across writes.
// It retains structured tool deltas alongside a convenient assistant text field.
func StreamSnapshot(raw []byte, truncated bool) (json.RawMessage, bool) {
	var state StreamState
	_, _ = state.Write(raw)
	return StreamSnapshotWithCompletion(raw, truncated, state.Complete()), state.Complete()
}

// StreamSnapshotWithCompletion uses the full-stream observer's status even when
// only a prefix of the response was retained for the administrator.
func StreamSnapshotWithCompletion(raw []byte, truncated, complete bool) json.RawMessage {
	events := make([]any, 0)
	var text strings.Builder
	normalized := bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
	normalized = bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))
	frames := bytes.Split(normalized, []byte("\n\n"))
	// The last element lacks a frame delimiter; a trailing partial event is not evidence of completion.
	for _, frame := range frames[:len(frames)-1] {
		var data []string
		for _, line := range bytes.Split(frame, []byte("\n")) {
			if bytes.HasPrefix(line, []byte("data:")) {
				data = append(data, strings.TrimPrefix(string(line[5:]), " "))
			}
		}
		payload := strings.Join(data, "\n")
		if payload == "[DONE]" {
			continue
		}
		var event map[string]any
		if !decodeJSON([]byte(payload), &event) {
			continue
		}
		typ, _ := event["type"].(string)
		if typ == "response.output_text.delta" {
			if delta, ok := event["delta"].(string); ok {
				_, _ = text.WriteString(delta)
			}
		}
		if delta, ok := event["delta"].(map[string]any); ok {
			if t, ok := delta["text"].(string); ok {
				_, _ = text.WriteString(t)
			}
		}
		if choices, ok := event["choices"].([]any); ok {
			for _, c := range choices {
				if choice, ok := c.(map[string]any); ok {
					if delta, ok := choice["delta"].(map[string]any); ok {
						if t, ok := delta["content"].(string); ok {
							_, _ = text.WriteString(t)
						}
					}
				}
			}
		}
		events = append(events, sanitize(event, 0))
	}
	encoded, _ := json.Marshal(map[string]any{"text": strings.ReplaceAll(text.String(), "\x00", "\uFFFD"), "events": events, "complete": complete, "truncated": truncated})
	return encoded
}
