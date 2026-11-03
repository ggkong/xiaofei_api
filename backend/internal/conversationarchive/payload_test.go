package conversationarchive

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSnapshotDoesNotRetainCredentialsOrEmbeddedMedia(t *testing.T) {
	raw := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"hello"},{"type":"image_url","image_url":{"url":"data:image/png;base64,PRIVATE"}}]}],"api_key":"secret-key","nested":{"Authorization":"Bearer secret","password":"secret-password"}}`)
	got := string(Snapshot(raw, false))
	for _, secret := range []string{"PRIVATE", "secret-key", "Bearer secret", "secret-password"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret retained: %s", secret)
		}
	}
	if !strings.Contains(got, "hello") {
		t.Fatal("text input was lost")
	}
	for _, raw := range [][]byte{[]byte(`{"api_key":"partial`), []byte(`{"hello":"world"}`)} {
		if string(Snapshot(raw, true)) != `{"omitted":true}` {
			t.Fatal("truncated body must not retain raw preview")
		}
	}
}

func TestStreamSnapshotPreservesTextToolsAndCompletion(t *testing.T) {
	raw := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\r\n\r\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"world\",\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"lookup\",\"arguments\":\"{}\"}}]}}]}\n\n" +
		"data: [DONE]\n\n"
	got, complete := StreamSnapshot([]byte(raw), false)
	if !complete {
		t.Fatal("terminal marker not recognized")
	}
	if !strings.Contains(string(got), "Hello world") || !strings.Contains(string(got), "lookup") {
		t.Fatalf("missing content: %s", got)
	}
	for _, suffix := range []string{"data: [DONE]", "data: {\"type\":\"response.completed\"}"} {
		_, complete = StreamSnapshot([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"+suffix), false)
		if complete {
			t.Fatal("unterminated SSE frame was treated as complete")
		}
	}
}

func TestStreamSnapshotFailureAndProviderEvents(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		complete bool
		text     string
	}{
		{"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\"}\n\n", true, "ok"},
		{"data: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"anthropic\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n", true, "anthropic"},
		{"data: {\"error\":{\"message\":\"failed\"}}\n\ndata: [DONE]\n\n", false, ""},
	} {
		raw, complete := StreamSnapshot([]byte(tc.raw), false)
		if complete != tc.complete {
			t.Errorf("completion=%v want %v", complete, tc.complete)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		if out["text"] != tc.text {
			t.Errorf("text=%v", out["text"])
		}
	}
}

func TestPayloadsCannotContainPostgresNullCharacters(t *testing.T) {
	raw := Snapshot([]byte(`{"messages":[{"role":"user","content":"hello\u0000world"}],"bad\u0000key":"value"}`), false)
	if strings.Contains(string(raw), `\u0000`) {
		t.Fatal("JSONB-incompatible null character retained")
	}
	stream, _ := StreamSnapshot([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\\u0000world\"}\n\ndata: {\"type\":\"response.completed\"}\n\n"), false)
	if strings.Contains(string(stream), `\u0000`) {
		t.Fatal("stream retained a JSONB-incompatible null character")
	}
}

func TestSnapshotPreservesLargeToolResultNumbers(t *testing.T) {
	raw := []byte(`{"messages":[{"role":"tool","content":{"record_id":9007199254740993}}]}`)
	if !strings.Contains(string(Snapshot(raw, false)), "9007199254740993") {
		t.Fatal("tool result number lost precision")
	}
	if string(Snapshot([]byte(`{} {}`), false)) != `{"omitted":true}` {
		t.Fatal("multiple JSON documents were accepted")
	}
}

func TestResponseErrorEnvelopeIsNotConfusedWithToolOutput(t *testing.T) {
	if !ResponseHasError([]byte(`{"error":{"message":"failed"}}`)) {
		t.Fatal("HTTP-200 error envelope was missed")
	}
	if ResponseHasError([]byte(`{"error":null,"choices":[]}`)) || ResponseHasError([]byte(`{"choices":[{"message":{"content":"error"}}]}`)) {
		t.Fatal("valid output was classified as an error")
	}
}
