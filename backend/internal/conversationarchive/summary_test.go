package conversationarchive

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestQuestionSummarySkipsSystemAndToolResults(t *testing.T) {
	for _, body := range []string{
		`{"messages":[{"role":"system","content":"hidden system instructions"},{"role":"user","content":[{"type":"text","text":"成员的问题"}]},{"role":"assistant","content":"old reply"},{"role":"user","content":[{"type":"tool_result","content":"not a new question"}]}]}`,
		`{"input":[{"role":"user","content":[{"type":"input_text","text":"成员的问题"},{"type":"input_image","image_url":"secret media"}]},{"type":"function_call_output","output":"tool result"}]}`,
		`{"input":"成员的问题"}`,
	} {
		if got := PromptText([]byte(body), 600); got != "成员的问题" {
			t.Fatalf("unexpected question: %q", got)
		}
	}
}

func TestLargeRequestKeepsLatestQuestionAndModelWithinLimit(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"model": "coding-model", "stream": true,
		"messages": []map[string]any{
			{"role": "system", "content": strings.Repeat("large context ", 3000)},
			{"role": "user", "content": "这次真正的问题"},
		},
		"api_key": "must-not-be-retained",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := RequestSnapshot(raw, 4096)
	if len(snapshot) > 4096 || !json.Valid(snapshot) || PromptText(snapshot, 600) != "这次真正的问题" {
		t.Fatalf("large request lost its question or exceeded the limit: %s", snapshot)
	}
	if !strings.Contains(string(snapshot), "coding-model") || strings.Contains(string(snapshot), "must-not-be-retained") || !strings.Contains(string(snapshot), `"context_omitted":true`) {
		t.Fatal("compact snapshot has incorrect metadata or retains extra fields")
	}
	for _, question := range []string{strings.Repeat("汉字", 5000), strings.Repeat("\x01", 8000)} {
		raw, _ := json.Marshal(map[string]any{"input": question, "model": "test"})
		if got := RequestSnapshot(raw, 4096); len(got) > 4096 || !json.Valid(got) || !utf8.Valid(got) {
			t.Fatal("escaped or multilingual question broke the memory bound")
		}
	}
}

func TestReplyPreviewsAndBlockedResponses(t *testing.T) {
	for _, body := range []string{
		`{"choices":[{"message":{"content":"回答内容"}}]}`,
		`{"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"回答内容"}]}]}`,
		`{"content":[{"type":"text","text":"回答内容"}]}`,
		`{"text":"回答内容","events":[]}`,
	} {
		if got := ReplyText([]byte(body), 800); got != "回答内容" {
			t.Fatalf("reply missing: %q", got)
		}
	}
	if !BlockedResponse([]byte(`{"error":{"code":"content_policy_violation"}}`)) || BlockedResponse([]byte(`{"error":{"code":"rate_limit_exceeded"}}`)) {
		t.Fatal("content blocks were confused with other gateway errors")
	}
}

func TestRequestObservationUsesAlreadyReadBody(t *testing.T) {
	var got string
	ctx := WithRequestCapture(context.Background(), func(body []byte) { got = string(body) })
	ObserveRequestBody(ctx, []byte("observed"))
	if got != "observed" {
		t.Fatal("request capture hook was not called")
	}
	ObserveRequestBody(context.Background(), nil)
}
