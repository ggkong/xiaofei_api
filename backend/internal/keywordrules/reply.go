package keywordrules

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Reply struct {
	Text     string
	Model    string
	Protocol string
	Stream   bool
}

type keywordReplyEvent struct {
	name string
	data any
}

func keywordReplyPayload(reply *Reply) (any, []keywordReplyEvent) {
	id := uuid.NewString()
	usage := gin.H{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
	switch reply.Protocol {
	case "anthropic_messages":
		message := gin.H{"id": "msg_local_" + id, "type": "message", "role": "assistant", "model": reply.Model, "content": []any{gin.H{"type": "text", "text": reply.Text}}, "stop_reason": "end_turn", "stop_sequence": nil, "usage": gin.H{"input_tokens": 0, "output_tokens": 0}}
		start := gin.H{"id": message["id"], "type": "message", "role": "assistant", "model": reply.Model, "content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": gin.H{"input_tokens": 0, "output_tokens": 0}}
		return message, []keywordReplyEvent{
			{"message_start", gin.H{"type": "message_start", "message": start}},
			{"content_block_start", gin.H{"type": "content_block_start", "index": 0, "content_block": gin.H{"type": "text", "text": ""}}},
			{"content_block_delta", gin.H{"type": "content_block_delta", "index": 0, "delta": gin.H{"type": "text_delta", "text": reply.Text}}},
			{"content_block_stop", gin.H{"type": "content_block_stop", "index": 0}},
			{"message_delta", gin.H{"type": "message_delta", "delta": gin.H{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": gin.H{"output_tokens": 0}}},
			{"message_stop", gin.H{"type": "message_stop"}},
		}
	case "openai_responses":
		part := gin.H{"type": "output_text", "text": reply.Text, "annotations": []any{}}
		item := gin.H{"id": "msg_local_" + id, "type": "message", "status": "completed", "role": "assistant", "content": []any{part}}
		response := gin.H{"id": "resp_local_" + id, "object": "response", "created_at": time.Now().Unix(), "status": "completed", "model": reply.Model, "error": nil, "output": []any{item}, "usage": gin.H{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}}
		start := gin.H{"id": response["id"], "object": "response", "created_at": response["created_at"], "status": "in_progress", "model": reply.Model, "error": nil, "output": []any{}}
		pendingItem := gin.H{"id": item["id"], "type": "message", "status": "in_progress", "role": "assistant", "content": []any{}}
		events := []keywordReplyEvent{
			{"response.created", gin.H{"response": start}},
			{"response.output_item.added", gin.H{"output_index": 0, "item": pendingItem}},
			{"response.content_part.added", gin.H{"item_id": item["id"], "output_index": 0, "content_index": 0, "part": gin.H{"type": "output_text", "text": "", "annotations": []any{}}}},
			{"response.output_text.delta", gin.H{"item_id": item["id"], "output_index": 0, "content_index": 0, "delta": reply.Text}},
			{"response.output_text.done", gin.H{"item_id": item["id"], "output_index": 0, "content_index": 0, "text": reply.Text}},
			{"response.content_part.done", gin.H{"item_id": item["id"], "output_index": 0, "content_index": 0, "part": part}},
			{"response.output_item.done", gin.H{"output_index": 0, "item": item}},
			{"response.completed", gin.H{"response": response}},
		}
		for i := range events {
			data, ok := events[i].data.(gin.H)
			if !ok {
				continue
			}
			data["type"], data["sequence_number"] = events[i].name, i
		}
		return response, events
	case "gemini":
		response := gin.H{"candidates": []any{gin.H{"index": 0, "content": gin.H{"role": "model", "parts": []any{gin.H{"text": reply.Text}}}, "finishReason": "STOP"}}, "usageMetadata": gin.H{"promptTokenCount": 0, "candidatesTokenCount": 0, "totalTokenCount": 0}, "modelVersion": reply.Model}
		return response, []keywordReplyEvent{{"", response}}
	default:
		response := gin.H{"id": "chatcmpl-local-" + id, "object": "chat.completion", "created": time.Now().Unix(), "model": reply.Model, "choices": []any{gin.H{"index": 0, "message": gin.H{"role": "assistant", "content": reply.Text}, "finish_reason": "stop"}}, "usage": usage}
		chunk := func(delta gin.H, finish any) gin.H {
			return gin.H{"id": response["id"], "object": "chat.completion.chunk", "created": response["created"], "model": reply.Model, "choices": []any{gin.H{"index": 0, "delta": delta, "finish_reason": finish}}}
		}
		return response, []keywordReplyEvent{{"", chunk(gin.H{"role": "assistant", "content": ""}, nil)}, {"", chunk(gin.H{"content": reply.Text}, nil)}, {"", chunk(gin.H{}, "stop")}, {"", "[DONE]"}}
	}
}

// WriteHTTP writes a complete local reply using the client's chat protocol.
func WriteHTTP(c *gin.Context, reply *Reply) bool {
	if reply == nil {
		return false
	}
	payload, events := keywordReplyPayload(reply)
	c.Header("X-Sub2API-Keyword-Reply", "true")
	c.Header("Cache-Control", "no-store")
	if !reply.Stream {
		c.JSON(http.StatusOK, payload)
		return true
	}
	c.Header("Content-Type", "text/event-stream")
	c.Status(http.StatusOK)
	for _, event := range events {
		var data []byte
		if marker, ok := event.data.(string); ok {
			data = []byte(marker)
		} else {
			data, _ = json.Marshal(event.data)
		}
		if event.name != "" {
			if _, err := fmt.Fprintf(c.Writer, "event: %s\n", event.name); err != nil {
				return true
			}
		}
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
			return true
		}
		c.Writer.Flush()
	}
	return true
}

// WriteWS writes a complete Responses lifecycle without calling a model.
func WriteWS(ctx context.Context, conn *coderws.Conn, reply *Reply) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	copy := *reply
	copy.Protocol = "openai_responses"
	_, events := keywordReplyPayload(&copy)
	for _, event := range events {
		payload, err := json.Marshal(event.data)
		if err != nil || conn.Write(ctx, coderws.MessageText, payload) != nil {
			return
		}
	}
}

// WriteBlock preserves the client's protocol while identifying a local policy hit.
func WriteBlock(c *gin.Context, protocol, message string) {
	if strings.TrimSpace(message) == "" {
		message = "Request blocked by a keyword rule"
	}
	c.Header("Cache-Control", "no-store")
	if protocol == "gemini" {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"code": 403, "status": "PERMISSION_DENIED", "message": message}})
		return
	}
	errorBody := gin.H{"type": "permission_error", "code": "content_policy_violation", "message": message}
	if protocol == "anthropic_messages" {
		c.JSON(http.StatusForbidden, gin.H{"type": "error", "error": errorBody})
		return
	}
	c.JSON(http.StatusForbidden, gin.H{"error": errorBody})
}
