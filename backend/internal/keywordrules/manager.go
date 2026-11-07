package keywordrules

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Manager struct {
	store      ValueStore
	mu         sync.Mutex
	cached     *compiledPolicy
	configured bool
	loaded     time.Time
}

func NewManager(store ValueStore) *Manager { return &Manager{store: store} }
func revision(raw string) string {
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:])
}
func clonePolicy(policy Policy) Policy {
	raw, _ := json.Marshal(policy)
	var copy Policy
	_ = json.Unmarshal(raw, &copy)
	return copy
}

func legacyPolicy(values map[string]string) Policy {
	policy := Policy{Version: 1, Legacy: true, Rules: []Rule{}, Revision: revision(values["content_moderation_config"] + values["risk_control_enabled"] + values["cyber_policy_user_allowlist"])}
	var old struct {
		Enabled      bool     `json:"enabled"`
		Mode         string   `json:"mode"`
		BlockingMode string   `json:"keyword_blocking_mode"`
		Keywords     []string `json:"blocked_keywords"`
		Match        string   `json:"keyword_match_mode"`
		AutoReply    bool     `json:"keyword_auto_reply_enabled"`
		Reply        string   `json:"keyword_reply"`
		Block        string   `json:"block_message"`
		AllGroups    bool     `json:"all_groups"`
		Groups       []int64  `json:"group_ids"`
		Models       struct {
			Type   string   `json:"type"`
			Models []string `json:"models"`
		} `json:"model_filter"`
	}
	if json.Unmarshal([]byte(values["content_moderation_config"]), &old) != nil || len(old.Keywords) == 0 {
		return policy
	}
	mode := "contains"
	sensitive := false
	if old.Match == "exact" {
		mode = "exact"
		sensitive = true
	}
	if old.Match == "literal" {
		sensitive = true
	}
	r := Rule{ID: "legacy-keywords", Name: "Imported keyword rule", Enabled: true, Priority: 100, Keywords: old.Keywords, MatchMode: mode, KeywordLogic: "any", CaseSensitive: boolean(sensitive), ScanScope: "latest_user", Action: "block", Reply: old.Block, InjectionPosition: "append"}
	if old.AutoReply && strings.TrimSpace(old.Reply) != "" && (old.Match == "exact" || old.Match == "literal") {
		r.Action = "reply"
		r.Reply = old.Reply
	}
	if !old.AllGroups {
		r.GroupIDs = old.Groups
		if len(old.Groups) == 0 {
			r.Enabled = false
		}
	}
	if old.Models.Type == "include" {
		r.Models = old.Models.Models
	}
	if old.Models.Type == "exclude" {
		r.ExcludeModels = old.Models.Models
	}
	for _, field := range strings.FieldsFunc(values["cyber_policy_user_allowlist"], func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		id, err := strconv.ParseInt(field, 10, 64)
		if err != nil || id < 1 {
			r.Enabled = false
			break
		}
		r.ExcludeUserIDs = append(r.ExcludeUserIDs, id)
	}
	policy.Enabled = old.Enabled && old.Mode == "pre_block" && old.BlockingMode != "api_only" && values["risk_control_enabled"] == "true"
	for start := 0; start < len(old.Keywords); start += 5000 {
		end := start + 5000
		if end > len(old.Keywords) {
			end = len(old.Keywords)
		}
		chunk := r
		chunk.Keywords = old.Keywords[start:end]
		if start > 0 {
			chunk.ID = fmt.Sprintf("legacy-keywords-%d", start/5000+1)
			chunk.Name = fmt.Sprintf("Imported keyword rule %d", start/5000+1)
		}
		policy.Rules = append(policy.Rules, chunk)
	}
	return policy
}
func (m *Manager) loadLocked(ctx context.Context, force bool) (*compiledPolicy, bool, error) {
	if !force && m.cached != nil && time.Since(m.loaded) < time.Second {
		return m.cached, m.configured, nil
	}
	if m.store == nil {
		return nil, false, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	values, err := m.store.GetMultiple(ctx, []string{SettingKey, "content_moderation_config", "risk_control_enabled", "cyber_policy_user_allowlist"})
	if err != nil {
		if !force && m.cached != nil {
			m.loaded = time.Now()
			return m.cached, m.configured, nil
		}
		return nil, false, ErrUnavailable
	}
	raw, configured := values[SettingKey]
	var policy Policy
	if configured {
		if json.Unmarshal([]byte(raw), &policy) != nil {
			return nil, false, ErrUnavailable
		}
		policy.Revision = revision(raw)
	} else {
		policy = legacyPolicy(values)
	}
	if m.cached != nil && m.cached.policy.Revision == policy.Revision && m.configured == configured {
		m.loaded = time.Now()
		return m.cached, m.configured, nil
	}
	compiled, err := compile(policy)
	if err != nil {
		return nil, false, ErrUnavailable
	}
	compiled.policy.Legacy = !configured
	m.cached, m.configured, m.loaded = compiled, configured, time.Now()
	return compiled, configured, nil
}
func (m *Manager) Snapshot(ctx context.Context) (Policy, error) {
	if m == nil {
		return Policy{}, ErrUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	compiled, _, err := m.loadLocked(ctx, false)
	if err != nil {
		return Policy{}, err
	}
	return clonePolicy(compiled.policy), nil
}
func (m *Manager) Save(ctx context.Context, policy Policy) (Policy, error) {
	if m == nil {
		return Policy{}, ErrUnavailable
	}
	expected := policy.Revision
	compiled, err := compile(clonePolicy(policy))
	if err != nil {
		return Policy{}, err
	}
	compiled.policy.Revision = ""
	compiled.policy.Legacy = false
	raw, err := json.Marshal(compiled.policy)
	if err != nil {
		return Policy{}, err
	}
	if len(raw) > MaxPolicyBytes {
		return Policy{}, fmt.Errorf("keyword rules exceed 4 MiB")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current, _, err := m.loadLocked(ctx, true)
	if err != nil {
		return Policy{}, err
	}
	if expected == "" || expected != current.policy.Revision {
		return Policy{}, ErrConflict
	}
	if err = m.store.Set(ctx, SettingKey, string(raw)); err != nil {
		return Policy{}, ErrUnavailable
	}
	compiled.policy.Revision = revision(string(raw))
	m.cached, m.configured, m.loaded = compiled, true, time.Now()
	return clonePolicy(compiled.policy), nil
}
func (m *Manager) Evaluate(ctx context.Context, input Input) (Result, error) {
	if m == nil {
		return Result{}, ErrUnavailable
	}
	m.mu.Lock()
	compiled, configured, err := m.loadLocked(ctx, false)
	m.mu.Unlock()
	if err != nil {
		return Result{}, err
	}
	if !configured {
		return Result{Hits: []Hit{}, Action: "forward"}, nil
	}
	result, err := compiled.evaluate(input, false)
	result.Configured = true
	return result, err
}
func Preview(policy Policy, input Input) (Result, error) {
	compiled, err := compile(clonePolicy(policy))
	if err != nil {
		return Result{}, err
	}
	result, err := compiled.evaluate(input, true)
	result.Configured = true
	return result, err
}

// RuntimeStatus avoids reading a gateway body when the independent feature is off.
func (m *Manager) RuntimeStatus(ctx context.Context) (bool, bool, error) {
	if m == nil {
		return false, false, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	compiled, configured, err := m.loadLocked(ctx, false)
	if err != nil {
		return false, false, err
	}
	return configured, compiled.policy.Enabled, nil
}
