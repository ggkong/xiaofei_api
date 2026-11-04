package conversationarchive

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

type requestCaptureKey struct{}

// WithRequestCapture observes the body the gateway already reads. It avoids
// buffering a second, potentially large copy in the archive middleware.
func WithRequestCapture(ctx context.Context, capture func([]byte)) context.Context {
	return context.WithValue(ctx, requestCaptureKey{}, capture)
}

func ObserveRequestBody(ctx context.Context, body []byte) {
	if capture, ok := ctx.Value(requestCaptureKey{}).(func([]byte)); ok {
		capture(body)
	}
}

func boundedText(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\x00", "\uFFFD"))
	if len(text) <= limit {
		return text
	}
	text = text[:limit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text + "…"
}

func contentText(value gjson.Result, limit int) string {
	return contentTextAtDepth(value, limit, 0)
}

func contentTextAtDepth(value gjson.Result, limit, depth int) string {
	if depth > 16 {
		return ""
	}
	if value.Type == gjson.String {
		return boundedText(value.String(), limit)
	}
	if value.IsObject() {
		typ := value.Get("type").String()
		if typ != "" && typ != "text" && typ != "input_text" && typ != "output_text" {
			return ""
		}
		return boundedText(value.Get("text").String(), limit)
	}
	var result strings.Builder
	if value.IsArray() {
		value.ForEach(func(_, part gjson.Result) bool {
			text := contentTextAtDepth(part, limit, depth+1)
			if text != "" {
				if result.Len() > 0 {
					result.WriteByte('\n')
				}
				result.WriteString(text)
			}
			return result.Len() < limit
		})
	}
	return boundedText(result.String(), limit)
}

// PromptText selects the latest actual user question, skipping system messages,
// assistant turns, tool results and embedded media.
func PromptText(raw []byte, limit int) string {
	root := gjson.ParseBytes(raw)
	input := root.Get("input")
	if input.Type == gjson.String {
		return boundedText(input.String(), limit)
	}
	items := root.Get("messages")
	if !items.IsArray() {
		items = input
	}
	var latest string
	items.ForEach(func(_, message gjson.Result) bool {
		role := message.Get("role").String()
		if role == "user" || (role == "" && message.Get("content").Exists() && (message.Get("type").String() == "message" || message.Get("type").String() == "")) {
			if text := contentText(message.Get("content"), limit); text != "" {
				latest = text
			}
		}
		return true
	})
	return latest
}

func ReplyText(raw []byte, limit int) string {
	root := gjson.ParseBytes(raw)
	if text := root.Get("text"); text.Type == gjson.String && text.String() != "" {
		return boundedText(text.String(), limit)
	}
	if text := contentText(root.Get("choices.0.message.content"), limit); text != "" {
		return text
	}
	var result strings.Builder
	root.Get("output").ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "message" || item.Get("role").String() == "assistant" {
			result.WriteString(contentText(item.Get("content"), limit))
		}
		return result.Len() < limit
	})
	if result.Len() > 0 {
		return boundedText(result.String(), limit)
	}
	return contentText(root.Get("content"), limit)
}

func BlockedResponse(raw []byte) bool {
	root := gjson.ParseBytes(raw)
	for _, path := range []string{"error.code", "error.type", "code"} {
		if root.Get(path).String() == "content_policy_violation" {
			return true
		}
	}
	return false
}

// Keep a useful, bounded question and the requested model when coding clients
// send context larger than the full-body retention limit.
func RequestSnapshot(raw []byte, maxBytes int) json.RawMessage {
	if len(raw) <= maxBytes {
		return Snapshot(raw, false)
	}
	if !gjson.ValidBytes(raw) {
		return json.RawMessage(`{"omitted":true}`)
	}
	limit := (maxBytes - 2048) / 8 // Reserve space for the model and JSON escaping.
	if limit > 8192 {
		limit = 8192
	}
	prompt := PromptText(raw, limit)
	model := boundedText(gjson.GetBytes(raw, "model").String(), 255)
	data, _ := json.Marshal(map[string]any{
		"model": model, "stream": gjson.GetBytes(raw, "stream").Bool(),
		"context_omitted": true,
		"messages":        []map[string]string{{"role": "user", "content": prompt}},
	})
	return data
}
