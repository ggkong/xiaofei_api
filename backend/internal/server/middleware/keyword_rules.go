package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/conversationarchive"
	"github.com/Wei-Shaw/sub2api/internal/keywordrules"
	"github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func keywordProtocol(path string) (string, bool) {
	switch {
	case strings.HasSuffix(path, "/chat/completions"):
		return "openai_chat_completions", true
	case strings.HasSuffix(path, "/responses"):
		return "openai_responses", true
	case strings.HasSuffix(path, "/messages"):
		return "anthropic_messages", true
	case strings.HasSuffix(path, ":generateContent") || strings.HasSuffix(path, ":streamGenerateContent"):
		return "gemini", true
	}
	return "", false
}

// KeywordRules runs after authentication and model admission, before body parsing
// and forwarding. It works independently of semantic risk-control switches.
func KeywordRules(manager *keywordrules.Manager, maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		protocol, supported := keywordProtocol(c.Request.URL.Path)
		if manager != nil && supported && c.Request.Method == http.MethodGet && protocol == "openai_responses" {
			c.Set(keywordrules.ContextManagerKey, manager)
			c.Next()
			return
		}
		if manager == nil || !supported || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		key, ok := GetAPIKeyFromContext(c)
		if !ok || key == nil || key.GroupID == nil {
			c.Next()
			return
		}
		configured, enabled, err := manager.RuntimeStatus(c.Request.Context())
		if err != nil {
			c.AbortWithStatusJSON(503, gin.H{"error": gin.H{"type": "api_error", "message": "Keyword rules temporarily unavailable"}})
			return
		}
		if !configured {
			c.Next()
			return
		}
		c.Request = c.Request.WithContext(keywordrules.WithManaged(c.Request.Context()))
		if !enabled {
			c.Next()
			return
		}
		body, err := httputil.ReadLenientJSONRequestBodyWithPrealloc(c.Request, maxBytes)
		if err != nil {
			status := 400
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				status = 413
			}
			c.AbortWithStatusJSON(status, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "Unable to read keyword rule request"}})
			return
		}
		conversationarchive.ObserveRequestBody(c.Request.Context(), body)
		requestmodel.ResetRequestBody(c.Request, body)
		if !gjson.ValidBytes(body) {
			c.Next()
			return
		}
		modelField := gjson.GetBytes(body, "model")
		if protocol != "gemini" && modelField.Type != gjson.String {
			c.Next()
			return
		}
		model := modelField.String()
		if protocol == "gemini" {
			model = strings.TrimPrefix(c.Param("modelAction"), "/")
			if index := strings.LastIndex(model, ":"); index >= 0 {
				model = model[:index]
			}
		}
		if public, ok := service.RequestedPublicModelFromContext(c.Request.Context()); ok {
			model = public
		}
		if model == "" {
			c.Next()
			return
		}
		stream := gjson.GetBytes(body, "stream")
		if stream.Exists() && stream.Type != gjson.True && stream.Type != gjson.False {
			c.Next()
			return
		}
		result, err := manager.Evaluate(c.Request.Context(), keywordrules.Input{Protocol: protocol, Model: model, UserID: key.UserID, GroupID: *key.GroupID, Body: body})
		if err != nil {
			c.AbortWithStatusJSON(422, gin.H{"error": gin.H{"type": "invalid_request_error", "message": err.Error()}})
			return
		}
		if len(result.Hits) > 0 {
			c.Set(keywordrules.ContextHitsKey, result.Hits)
		}
		switch result.Action {
		case "reply":
			c.Set(keywordrules.ContextReplyKey, true)
			keywordrules.WriteHTTP(c, &keywordrules.Reply{Text: result.Reply, Model: model, Protocol: protocol, Stream: stream.Bool() || strings.Contains(c.Request.URL.Path, ":streamGenerateContent")})
			c.Abort()
			return
		case "block":
			keywordrules.WriteBlock(c, protocol, result.Reply)
			c.Abort()
			return
		}
		if result.EffectiveBody != nil {
			if maxBytes > 0 && int64(len(result.EffectiveBody)) > maxBytes {
				c.AbortWithStatusJSON(413, gin.H{"error": gin.H{"type": "invalid_request_error", "message": "Injected request exceeds the gateway body limit"}})
				return
			}
			c.Set(keywordrules.ContextOriginalBodyKey, body)
			c.Set(keywordrules.ContextEffectiveBodyKey, result.EffectiveBody)
			requestmodel.ResetRequestBody(c.Request, result.EffectiveBody)
		}
		c.Next()
	}
}
