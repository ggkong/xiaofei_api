// Package keywordrules implements independently managed, deterministic text rules.
package keywordrules

import (
	"context"
	"errors"
	"time"
)

const SettingKey = "keyword_rules_config"
const ContextManagerKey = "sub2api.keyword_rules.manager"
const ContextOriginalBodyKey = "sub2api.keyword_rules.original_body"
const ContextHitsKey = "sub2api.keyword_rules.hits"
const ContextEffectiveBodyKey = "sub2api.keyword_rules.effective_body"
const ContextReplyKey = "sub2api.keyword_rules.reply"
const MaxRules = 500
const MaxKeywords = 50000
const MaxKeywordRunes = 512
const MaxTextBytes = 16 << 20
const MaxPolicyBytes = 4 << 20

var ErrIntercepted = errors.New("request handled by keyword rules")

var ErrConflict = errors.New("keyword rules changed; reload before saving")
var ErrUnavailable = errors.New("keyword rules unavailable")

type Rule struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Category          string   `json:"category"`
	Enabled           bool     `json:"enabled"`
	Priority          int      `json:"priority"`
	Keywords          []string `json:"keywords"`
	ExcludeKeywords   []string `json:"exclude_keywords"`
	MatchMode         string   `json:"match_mode"`
	KeywordLogic      string   `json:"keyword_logic"`
	CaseSensitive     *bool    `json:"case_sensitive"`
	ScanScope         string   `json:"scan_scope"`
	Action            string   `json:"action"`
	Reply             string   `json:"reply"`
	Injection         string   `json:"injection"`
	InjectionPosition string   `json:"injection_position"`
	StopOnMatch       bool     `json:"stop_on_match"`
	GroupIDs          []int64  `json:"group_ids"`
	UserIDs           []int64  `json:"user_ids"`
	ExcludeUserIDs    []int64  `json:"exclude_user_ids"`
	Models            []string `json:"models"`
	ExcludeModels     []string `json:"exclude_models"`
	Protocols         []string `json:"protocols"`
}

type Policy struct {
	Version  int    `json:"version"`
	Revision string `json:"revision"`
	Enabled  bool   `json:"enabled"`
	Rules    []Rule `json:"rules"`
	Legacy   bool   `json:"legacy"`
}

type Input struct {
	Protocol string `json:"protocol"`
	Model    string `json:"model"`
	UserID   int64  `json:"user_id"`
	GroupID  int64  `json:"group_id"`
	Body     []byte `json:"-"`
	Text     string `json:"text,omitempty"`
}

type Hit struct {
	RuleID            string   `json:"rule_id"`
	RuleName          string   `json:"rule_name"`
	Action            string   `json:"action"`
	Keywords          []string `json:"keywords"`
	Injection         string   `json:"injection,omitempty"`
	InjectionPosition string   `json:"injection_position,omitempty"`
}

type Result struct {
	Configured    bool   `json:"configured"`
	Enabled       bool   `json:"enabled"`
	Hits          []Hit  `json:"hits"`
	Action        string `json:"action"`
	Reply         string `json:"reply,omitempty"`
	EffectiveBody []byte `json:"-"`
}

type ValueStore interface {
	GetMultiple(context.Context, []string) (map[string]string, error)
	Set(context.Context, string, string) error
}

type managedKey struct{}

func WithManaged(ctx context.Context) context.Context {
	return context.WithValue(ctx, managedKey{}, true)
}
func Managed(ctx context.Context) bool { value, _ := ctx.Value(managedKey{}).(bool); return value }

type Stats struct {
	RuleID    string     `json:"rule_id"`
	Count     uint64     `json:"count"`
	LastHitAt *time.Time `json:"last_hit_at,omitempty"`
}
