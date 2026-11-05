package routes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/keywordrules"
	"github.com/Wei-Shaw/sub2api/internal/pkg/adminaccess"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func registerKeywordRuleRoutes(admin *gin.RouterGroup, settings *service.SettingService) {
	var manager *keywordrules.Manager
	if settings != nil {
		manager = settings.KeywordRules()
	}
	rules := admin.Group("/keyword-rules")
	rules.Use(middleware.AdminOnly(), func(c *gin.Context) {
		if c.GetString("auth_method") != "jwt" || adminaccess.FromContext(c.Request.Context()) != nil {
			c.Abort()
			response.Forbidden(c, "Administrator browser login required")
			return
		}
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	rules.GET("", func(c *gin.Context) {
		policy, err := manager.Snapshot(c.Request.Context())
		if err != nil {
			response.Error(c, 503, "Keyword rules unavailable")
			return
		}
		response.Success(c, policy)
	})
	rules.PUT("", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, keywordrules.MaxPolicyBytes)
		var policy keywordrules.Policy
		if err := c.ShouldBindJSON(&policy); err != nil {
			response.BadRequest(c, "Invalid keyword rules document")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		saved, err := manager.Save(ctx, policy)
		if errors.Is(err, keywordrules.ErrConflict) {
			response.Error(c, 409, err.Error())
			return
		}
		if errors.Is(err, keywordrules.ErrUnavailable) {
			response.Error(c, 503, "Keyword rules unavailable")
			return
		}
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		response.Success(c, saved)
	})
	rules.POST("/test", func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, keywordrules.MaxPolicyBytes+(256<<10))
		var req struct {
			Policy   keywordrules.Policy `json:"policy"`
			Text     string              `json:"text"`
			Request  json.RawMessage     `json:"request"`
			Protocol string              `json:"protocol"`
			Model    string              `json:"model"`
			GroupID  int64               `json:"group_id"`
			UserID   int64               `json:"user_id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid rule test")
			return
		}
		if req.Protocol == "" {
			req.Protocol = "openai_chat_completions"
		}
		switch req.Protocol {
		case "openai_chat_completions", "openai_responses", "anthropic_messages", "gemini":
		default:
			response.BadRequest(c, "Unsupported test protocol")
			return
		}
		if req.GroupID < 0 || req.UserID < 0 || len(req.Text) > 65536 {
			response.BadRequest(c, "Invalid test scope or text exceeds 64 KiB")
			return
		}
		body := req.Request
		if len(body) == 0 {
			var payload any
			switch req.Protocol {
			case "openai_responses":
				payload = gin.H{"model": req.Model, "input": req.Text}
			case "gemini":
				payload = gin.H{"contents": []any{gin.H{"role": "user", "parts": []any{gin.H{"text": req.Text}}}}}
			default:
				payload = gin.H{"model": req.Model, "messages": []any{gin.H{"role": "user", "content": req.Text}}}
			}
			body, _ = json.Marshal(payload)
		}
		result, err := keywordrules.Preview(req.Policy, keywordrules.Input{Protocol: req.Protocol, Model: req.Model, UserID: req.UserID, GroupID: req.GroupID, Body: body})
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		response.Success(c, gin.H{"enabled": result.Enabled, "hits": result.Hits, "action": result.Action, "reply": result.Reply, "effective_request": json.RawMessage(result.EffectiveBody)})
	})
}
