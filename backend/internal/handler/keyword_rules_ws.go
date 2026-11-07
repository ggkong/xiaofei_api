package handler

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/keywordrules"
	"github.com/Wei-Shaw/sub2api/internal/pkg/requestmodel"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

func applyWSKeywordRules(c *gin.Context, conn *coderws.Conn, key *service.APIKey, model string, body []byte) ([]byte, error) {
	value, exists := c.Get(keywordrules.ContextManagerKey)
	if !exists {
		return body, nil
	}
	manager, ok := value.(*keywordrules.Manager)
	if !ok || manager == nil || key == nil || key.GroupID == nil {
		return body, nil
	}
	if blocked := blockedModelAllowlistCandidate(key.Group, append([]string{model}, requestmodel.FromBodyCandidates("", "application/json", body)...)); blocked != "" {
		middleware2.MarkIngressRejected(c, middleware2.IngressRejectModelNotAllowed)
		return nil, service.NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "model unavailable for this group", nil)
	}
	result, err := manager.Evaluate(c.Request.Context(), keywordrules.Input{Protocol: "openai_responses", Model: model, UserID: key.UserID, GroupID: *key.GroupID, Body: body})
	if err != nil {
		return nil, service.NewOpenAIWSClientCloseError(coderws.StatusInternalError, "keyword rules unavailable", err)
	}
	if result.Configured {
		c.Request = c.Request.WithContext(keywordrules.WithManaged(c.Request.Context()))
		c.Set(keywordrules.ContextOriginalBodyKey, body)
	}
	c.Set(keywordrules.ContextHitsKey, result.Hits)
	if result.Action == "reply" {
		keywordrules.WriteWS(c.Request.Context(), conn, &keywordrules.Reply{Text: result.Reply, Model: model, Protocol: "openai_responses", Stream: true})
		return nil, service.NewOpenAIWSClientCloseError(coderws.StatusNormalClosure, "keyword reply delivered", keywordrules.ErrIntercepted)
	}
	if result.Action == "block" {
		message := result.Reply
		if message == "" {
			message = "Request blocked by a keyword rule"
		}
		decision := &securityaudit.Decision{Kind: securityaudit.DecisionBlock, HTTPStatus: http.StatusForbidden, ErrorCode: "content_policy_violation", ClientMessage: message, Legacy: &securityaudit.LegacyDecision{Blocked: true, Message: message, StatusCode: http.StatusForbidden, ErrorCode: "content_policy_violation"}}
		writeSecurityAuditWSError(c.Request.Context(), conn, decision)
		return nil, service.NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "keyword request blocked", keywordrules.ErrIntercepted)
	}
	if result.EffectiveBody != nil {
		c.Set(keywordrules.ContextEffectiveBodyKey, result.EffectiveBody)
		return result.EffectiveBody, nil
	}
	return body, nil
}
