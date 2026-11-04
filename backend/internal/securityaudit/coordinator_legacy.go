package securityaudit

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/tidwall/gjson"
)

type LegacyModerationAdapter struct {
	service *service.ContentModerationService
}

func NewLegacyModerationAdapter(svc *service.ContentModerationService) LegacyEngine {
	return &LegacyModerationAdapter{service: svc}
}

func (a *LegacyModerationAdapter) Check(ctx context.Context, req Request) (*LegacyDecision, error) {
	if a == nil || a.service == nil {
		return nil, nil
	}
	decision, err := a.service.Check(ctx, service.ContentModerationCheckInput{
		RequestID: req.RequestID, UserID: req.UserID, UserEmail: req.UserEmail,
		APIKeyID: req.APIKeyID, APIKeyName: req.APIKeyName, GroupID: cloneInt64Ptr(req.GroupID),
		GroupName: req.GroupName, Endpoint: req.Endpoint, Provider: req.Provider,
		Model: req.Model, Protocol: req.Protocol, Body: req.Body,
	})
	if err != nil || decision == nil {
		return nil, err
	}
	result := &LegacyDecision{
		Allowed: decision.Allowed, Blocked: decision.Blocked, Flagged: decision.Flagged,
		Message: decision.Message, StatusCode: decision.StatusCode,
		ErrorCode: "content_policy_violation", Action: decision.Action,
	}
	if decision.Action == service.ContentModerationActionKeywordReply {
		result.Reply = &LocalKeywordReply{Text: decision.Message, Model: req.Model, Protocol: req.Protocol, Stream: gjson.GetBytes(req.Body, "stream").Bool() || req.Stage == "first_turn" || req.Stage == "subsequent_turn" || strings.Contains(req.Endpoint, "streamGenerateContent")}
	}
	return result, nil
}
