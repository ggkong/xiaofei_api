package keywordrules

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func textPart(value gjson.Result, depth int) string {
	if depth > 24 {
		return ""
	}
	if value.Type == gjson.String {
		return value.String()
	}
	if value.IsArray() {
		parts := []string{}
		value.ForEach(func(_, part gjson.Result) bool {
			if text := textPart(part, depth+1); text != "" {
				parts = append(parts, text)
			}
			return true
		})
		return strings.Join(parts, "\n")
	}
	if value.IsObject() {
		switch value.Get("type").String() {
		case "", "text", "input_text", "output_text":
			return value.Get("text").String()
		case "tool_result":
			return textPart(value.Get("content"), depth+1)
		}
	}
	return ""
}
func extractTexts(input Input, needed map[string]bool) (map[string]string, error) {
	texts := map[string]string{"latest_user": "", "all_user": "", "context": ""}
	if input.Text != "" {
		for scope := range needed {
			texts[scope] = input.Text
		}
		if len(input.Text) > MaxTextBytes {
			return nil, fmt.Errorf("test text is too large")
		}
		return texts, nil
	}
	root := gjson.ParseBytes(input.Body)
	context, users := []string{}, []string{}
	if needed["context"] {
		for _, field := range []string{"system", "instructions", "systemInstruction.parts", "system_instruction.parts"} {
			if text := textPart(root.Get(field), 0); text != "" {
				context = append(context, text)
			}
		}
	}
	items := root.Get("messages")
	if input.Protocol == "openai_responses" {
		items = root.Get("input")
		if items.Type == gjson.String {
			for scope := range needed {
				texts[scope] = items.String()
			}
			if needed["context"] {
				texts["context"] = strings.Join(append(context, items.String()), "\n")
			}
			return boundedTexts(texts, needed)
		}
	}
	if input.Protocol == "gemini" {
		items = root.Get("contents")
	}
	array := items.Array()
	if items.IsObject() {
		array = []gjson.Result{items}
	}
	lastUser := len(array) - 1
	if input.Protocol == "anthropic_messages" {
		for lastUser >= 0 && array[lastUser].Get("role").String() == "system" {
			lastUser--
		}
	}
	for index, item := range array {
		role := item.Get("role").String()
		kind := item.Get("type").String()
		isUser := role == "user" || (role == "" && (kind == "" || kind == "message" || kind == "input_text"))
		if !needed["context"] && !(isUser && needed["all_user"]) && !(isUser && needed["latest_user"] && index == lastUser) {
			continue
		}
		content := item.Get("content")
		if input.Protocol == "gemini" {
			content = item.Get("parts")
		}
		text := textPart(content, 0)
		if kind == "input_text" {
			text = item.Get("text").String()
		}
		if kind == "function_call_output" && needed["context"] {
			text = textPart(item.Get("output"), 0)
		}
		if text != "" {
			if needed["context"] {
				context = append(context, text)
			}
			if isUser && needed["all_user"] {
				users = append(users, text)
			}
			if isUser && index == lastUser && needed["latest_user"] {
				texts["latest_user"] = text
			}
		}
	}
	if needed["all_user"] {
		texts["all_user"] = strings.Join(users, "\n")
	}
	if needed["context"] {
		texts["context"] = strings.Join(context, "\n")
	}
	return boundedTexts(texts, needed)
}
func boundedTexts(texts map[string]string, needed map[string]bool) (map[string]string, error) {
	for scope := range needed {
		if len(texts[scope]) > MaxTextBytes {
			return nil, fmt.Errorf("selected scan text exceeds 16 MiB")
		}
	}
	return texts, nil
}
func protocolArray(raw json.RawMessage) ([]json.RawMessage, error) {
	var items []json.RawMessage
	if len(raw) == 0 {
		return []json.RawMessage{}, nil
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("system context must be an array")
	}
	return items, nil
}

// Inject only adds system instructions; user questions and tool structures are
// preserved. RE2 captures and user text are never interpolated into instructions.
func Inject(protocol string, body []byte, hits []Hit) ([]byte, error) {
	prefix, suffix := []string{}, []string{}
	size := 0
	for _, hit := range hits {
		if hit.Action != "inject" {
			continue
		}
		size += len(hit.Injection)
		if hit.InjectionPosition == "prepend" {
			prefix = append(prefix, hit.Injection)
		} else {
			suffix = append(suffix, hit.Injection)
		}
	}
	if size == 0 {
		return nil, nil
	}
	if size > 64<<10 {
		return nil, fmt.Errorf("combined injection exceeds 64 KiB")
	}
	var root map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&root) != nil || root == nil {
		return nil, fmt.Errorf("invalid injection request")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("invalid injection request")
	}
	switch protocol {
	case "openai_chat_completions":
		messages, err := protocolArray(root["messages"])
		if err != nil {
			return nil, err
		}
		makeMessage := func(text string) json.RawMessage {
			raw, _ := json.Marshal(map[string]string{"role": "system", "content": text})
			return raw
		}
		before, after := []json.RawMessage{}, []json.RawMessage{}
		for _, text := range prefix {
			before = append(before, makeMessage(text))
		}
		for _, text := range suffix {
			after = append(after, makeMessage(text))
		}
		lastSystem := 0
		for i, message := range messages {
			role := gjson.GetBytes(message, "role").String()
			if role == "system" || role == "developer" {
				lastSystem = i + 1
			} else {
				break
			}
		}
		all := append([]json.RawMessage{}, before...)
		all = append(all, messages[:lastSystem]...)
		all = append(all, after...)
		all = append(all, messages[lastSystem:]...)
		root["messages"], _ = json.Marshal(all)
	case "openai_responses":
		original := ""
		if raw := root["instructions"]; len(raw) > 0 && string(raw) != "null" {
			if json.Unmarshal(raw, &original) != nil {
				return nil, fmt.Errorf("instructions must be text")
			}
		}
		parts := append([]string{}, prefix...)
		if original != "" {
			parts = append(parts, original)
		}
		parts = append(parts, suffix...)
		root["instructions"], _ = json.Marshal(strings.Join(parts, "\n\n"))
	case "anthropic_messages":
		original := root["system"]
		blocks := []json.RawMessage{}
		var text string
		if len(original) > 0 && string(original) != "null" {
			if json.Unmarshal(original, &text) == nil {
				raw, _ := json.Marshal(map[string]string{"type": "text", "text": text})
				blocks = append(blocks, raw)
			} else {
				var err error
				blocks, err = protocolArray(original)
				if err != nil {
					return nil, err
				}
			}
		}
		all := []json.RawMessage{}
		for _, text := range prefix {
			raw, _ := json.Marshal(map[string]string{"type": "text", "text": text})
			all = append(all, raw)
		}
		all = append(all, blocks...)
		for _, text := range suffix {
			raw, _ := json.Marshal(map[string]string{"type": "text", "text": text})
			all = append(all, raw)
		}
		root["system"], _ = json.Marshal(all)
	case "gemini":
		field := "systemInstruction"
		if _, ok := root["system_instruction"]; ok {
			field = "system_instruction"
		}
		system := map[string]json.RawMessage{}
		if raw := root[field]; len(raw) > 0 && string(raw) != "null" {
			if json.Unmarshal(raw, &system) != nil {
				return nil, fmt.Errorf("invalid system instruction")
			}
		}
		blocks, err := protocolArray(system["parts"])
		if err != nil {
			return nil, err
		}
		all := []json.RawMessage{}
		for _, text := range prefix {
			raw, _ := json.Marshal(map[string]string{"text": text})
			all = append(all, raw)
		}
		all = append(all, blocks...)
		for _, text := range suffix {
			raw, _ := json.Marshal(map[string]string{"text": text})
			all = append(all, raw)
		}
		system["parts"], _ = json.Marshal(all)
		root[field], _ = json.Marshal(system)
	default:
		return nil, fmt.Errorf("prompt injection is unsupported for this protocol")
	}
	// Replace only the instruction field. Preserve duplicate model fields and
	// untouched numeric lexemes so admission and downstream parsing remain aligned.
	var field string
	switch protocol {
	case "openai_chat_completions":
		field = "messages"
	case "openai_responses":
		field = "instructions"
	case "anthropic_messages":
		field = "system"
	case "gemini":
		field = "systemInstruction"
		if _, ok := root["system_instruction"]; ok {
			field = "system_instruction"
		}
	}
	count := 0
	ambiguous := false
	gjson.ParseBytes(body).ForEach(func(key, value gjson.Result) bool {
		if strings.EqualFold(key.String(), field) {
			count++
			if key.String() != field {
				ambiguous = true
			}
		}
		return true
	})
	if count > 1 || ambiguous {
		return nil, fmt.Errorf("ambiguous instruction fields in request")
	}
	return sjson.SetRawBytes(body, field, root[field])
}
