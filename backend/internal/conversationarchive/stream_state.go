package conversationarchive

import (
	"bytes"
	"encoding/json"
	"strings"
)

const streamStateLimit = 64 << 10

// StreamState observes the entire client-visible stream independently of the
// saved-body limit. It only retains one bounded SSE line/event, never the reply.
// A completion marker beyond the archive limit is therefore still recognized.
type StreamState struct {
	line          []byte
	data          []byte
	event         string
	skipLF        bool
	lineOverflow  bool
	eventOverflow bool
	hasData       bool
	done          bool
	failed        bool
	incomplete    bool
}

func (s *StreamState) Write(p []byte) (int, error) {
	for _, b := range p {
		if s.skipLF {
			s.skipLF = false
			if b == '\n' {
				continue
			}
		}
		if b == '\r' || b == '\n' {
			s.finishLine()
			s.skipLF = b == '\r'
			continue
		}
		if len(s.line) < streamStateLimit {
			s.line = append(s.line, b)
		} else {
			s.lineOverflow = true
		}
	}
	return len(p), nil
}

func (s *StreamState) Complete() bool { return s.done && !s.failed && !s.incomplete }
func (s *StreamState) Failed() bool   { return s.failed }

func (s *StreamState) finishLine() {
	defer func() { s.line = s.line[:0]; s.lineOverflow = false }()
	if s.lineOverflow {
		if bytes.HasPrefix(s.line, []byte("data:")) {
			s.hasData = true
			s.eventOverflow = true
		}
		if bytes.HasPrefix(s.line, []byte("event:")) {
			s.event = ""
			s.eventOverflow = true
		}
		return
	}
	if len(s.line) == 0 {
		s.finishEvent()
		return
	}
	if s.line[0] == ':' {
		return
	}
	field, value, found := bytes.Cut(s.line, []byte(":"))
	if !found {
		value = nil
	}
	value = bytes.TrimPrefix(value, []byte(" "))
	switch string(field) {
	case "event":
		s.event = string(value)
	case "data":
		s.hasData = true
		if len(s.data)+len(value)+1 > streamStateLimit {
			s.eventOverflow = true
			return
		}
		s.data = append(s.data, value...)
		s.data = append(s.data, '\n')
	}
}

func (s *StreamState) finishEvent() {
	defer func() { s.data = s.data[:0]; s.event = ""; s.eventOverflow = false; s.hasData = false }()
	if !s.hasData {
		return
	}
	// Named Responses events can contain a very large final response object.
	// The SSE event name is still available without buffering that object.
	if s.eventOverflow {
		s.observeType(s.event)
		return
	}
	data := bytes.TrimSuffix(s.data, []byte("\n"))
	if bytes.Equal(data, []byte("[DONE]")) {
		s.observeType(s.event)
		s.done = true
		return
	}
	var event map[string]any
	if json.Unmarshal(data, &event) != nil {
		return
	}
	s.observeType(s.event)
	if typ, ok := event["type"].(string); ok {
		s.observeType(typ)
	}
	if event["error"] != nil {
		s.failed = true
	}
	if response, ok := event["response"].(map[string]any); ok {
		switch response["status"] {
		case "failed":
			s.failed = true
		case "incomplete":
			s.incomplete = true
		}
	}
}

func (s *StreamState) observeType(typ string) {
	switch strings.TrimSpace(typ) {
	case "response.completed", "message_stop":
		s.done = true
	case "response.failed", "error":
		s.failed = true
	case "response.incomplete":
		s.incomplete = true
	}
}
