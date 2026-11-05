package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/conversationarchive"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKeywordRepliesUseNormalJSONAndStreamingProtocols(t *testing.T) {
	text := "自定义回复 <script>不会执行</script>\n第二行。"
	for _, protocol := range []string{service.ContentModerationProtocolOpenAIChat, service.ContentModerationProtocolOpenAIResponses, service.ContentModerationProtocolAnthropicMessages, service.ContentModerationProtocolGemini} {
		for _, stream := range []bool{false, true} {
			t.Run(protocol+map[bool]string{true: "-stream", false: "-json"}[stream], func(t *testing.T) {
				response := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(response)
				decision := &securityaudit.Decision{Legacy: &securityaudit.LegacyDecision{Reply: &securityaudit.LocalKeywordReply{Text: text, Model: "member-model", Protocol: protocol, Stream: stream}}}
				require.True(t, writeLocalKeywordReply(c, decision))
				require.Equal(t, http.StatusOK, response.Code)
				require.Equal(t, "true", response.Header().Get("X-Sub2API-Keyword-Reply"))
				if stream {
					require.Contains(t, response.Header().Get("Content-Type"), "text/event-stream")
					if protocol != service.ContentModerationProtocolGemini {
						snapshot, complete := conversationarchive.StreamSnapshot(response.Body.Bytes(), false)
						require.True(t, complete)
						require.Equal(t, text, conversationarchive.ReplyText(snapshot, 1000))
					}
				} else {
					var body map[string]any
					require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
					if protocol != service.ContentModerationProtocolGemini {
						require.Equal(t, text, conversationarchive.ReplyText(response.Body.Bytes(), 1000))
					}
				}
			})
		}
	}
}

func TestKeywordReplyWebSocketCompletesAsNormalResponse(t *testing.T) {
	decision := &securityaudit.Decision{Legacy: &securityaudit.LegacyDecision{Reply: &securityaudit.LocalKeywordReply{Text: "指定回复", Model: "member-model", Protocol: service.ContentModerationProtocolOpenAIResponses, Stream: true}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		writeSecurityAuditWSError(r.Context(), conn, decision)
		_ = conn.Close(securityAuditWSCloseStatus(decision), securityAuditWSCloseReason(decision))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := coderws.Dial(ctx, strings.Replace(server.URL, "http", "ws", 1), nil)
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	found := false
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			break
		}
		var event map[string]any
		require.NoError(t, json.Unmarshal(raw, &event))
		if event["type"] == "response.completed" {
			found = true
			require.Contains(t, string(raw), "指定回复")
		}
	}
	require.True(t, found)
	require.Equal(t, coderws.StatusNormalClosure, securityAuditWSCloseStatus(decision))
}
