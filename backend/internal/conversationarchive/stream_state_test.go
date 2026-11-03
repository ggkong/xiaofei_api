package conversationarchive

import (
	"strings"
	"testing"
)

func TestStreamStateHandlesEveryByteBoundaryAndLineEnding(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n", "\r"} {
		raw := "data: {\"choices\":[{\"delta\":{\"content\":\"not a terminal: [DONE]\"}}]}" + ending + ending + "data: [DONE]" + ending + ending
		for split := 0; split <= len(raw); split++ {
			var state StreamState
			_, _ = state.Write([]byte(raw[:split]))
			_, _ = state.Write([]byte(raw[split:]))
			if !state.Complete() || state.Failed() {
				t.Fatalf("ending=%q split=%d: completion not observed", ending, split)
			}
		}
	}
}

func TestStreamStateDoesNotTreatTextOrPartialEventAsCompletion(t *testing.T) {
	for _, raw := range []string{
		"data: {\"choices\":[{\"delta\":{\"content\":\"[DONE]\"}}]}\n\n",
		"data: [DONE]\n",
		"event: response.completed\n\n",
		"event: response.completed\ndata: {broken}\n\n",
	} {
		var state StreamState
		_, _ = state.Write([]byte(raw))
		if state.Complete() {
			t.Fatalf("false completion: %q", raw)
		}
	}
}

func TestStreamStateRemembersFailureBeforeLaterDone(t *testing.T) {
	for _, failure := range []string{
		"data: {\"error\":{\"message\":\"failed\"}}\n\n",
		"event: error\ndata: {}\n\n",
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"failed\"}}\n\n",
	} {
		var state StreamState
		_, _ = state.Write([]byte(failure))
		_, _ = state.Write([]byte(strings.Repeat(": keepalive\n\n", 10000)))
		_, _ = state.Write([]byte("data: [DONE]\n\n"))
		if state.Complete() || !state.Failed() {
			t.Fatal("a later terminal hid an earlier error")
		}
	}
	var state StreamState
	_, _ = state.Write([]byte("data: {\"type\":\"response.incomplete\"}\n\ndata: [DONE]\n\n"))
	if state.Complete() || state.Failed() {
		t.Fatal("incomplete response must remain interrupted")
	}
}

func TestStreamStateIsBoundedAndRecognizesLargeNamedTerminal(t *testing.T) {
	var state StreamState
	_, _ = state.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":\""))
	chunk := []byte(strings.Repeat("x", 4096))
	for i := 0; i < 1024; i++ {
		_, _ = state.Write(chunk)
		if len(state.line) > streamStateLimit || len(state.data) > streamStateLimit {
			t.Fatal("state grew past its event limit")
		}
	}
	_, _ = state.Write([]byte("\"}\n\n"))
	if !state.Complete() {
		t.Fatal("large named completion was lost")
	}
	var unnamed StreamState
	_, _ = unnamed.Write([]byte("data: " + strings.Repeat("x", streamStateLimit*2) + "\n\ndata: [DONE]\n\n"))
	if !unnamed.Complete() {
		t.Fatal("oversized event prevented a later terminal")
	}
}
