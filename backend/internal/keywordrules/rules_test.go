package keywordrules

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func testRule() Rule {
	return Rule{ID: "one", Name: "Rule one", Enabled: true, Priority: 10, Keywords: []string{"Hello"}, MatchMode: "exact", Action: "reply", Reply: "Configured reply"}
}
func testInput(text string) Input { return Input{Protocol: "openai_chat_completions", Text: text} }
func check(t *testing.T, rules []Rule, text string) Result {
	t.Helper()
	result, err := Preview(Policy{Enabled: true, Rules: rules}, testInput(text))
	require.NoError(t, err)
	return result
}

func TestKeywordRuleMatchModesAndPrecision(t *testing.T) {
	for _, tc := range []struct {
		mode, text string
		hit        bool
	}{
		{"exact", " Hello \n", true}, {"exact", "hello", false}, {"exact", "Hello!", false}, {"exact", "say Hello", false},
		{"contains", "before Hello after", true}, {"contains", "hello", false},
		{"prefix", "Hello there", true}, {"prefix", "aHello", false}, {"suffix", "a Hello", true}, {"suffix", "Hello there", false},
		{"word", "(Hello)", true}, {"word", "HelloWorld", false}, {"word", "a_Hello", false}, {"word", "你好Hello", false},
		{"regex", "ORD-123", true}, {"regex", "ORD-12", false},
	} {
		t.Run(tc.mode+tc.text, func(t *testing.T) {
			r := testRule()
			r.MatchMode = tc.mode
			if tc.mode == "regex" {
				r.Keywords = []string{`^ORD-\d{3}$`}
			}
			got := check(t, []Rule{r}, tc.text)
			require.Equal(t, tc.hit, got.Action == "reply")
		})
	}
	r := testRule()
	r.CaseSensitive = boolean(false)
	require.Equal(t, "reply", check(t, []Rule{r}, "hello").Action)
	r.Keywords = []string{"A  B"}
	require.Empty(t, check(t, []Rule{r}, "A B").Hits)
	r.Keywords = []string{"危险"}
	require.Empty(t, check(t, []Rule{r}, "危險").Hits)
}
func TestKeywordRulesOverlapAllAndExclusions(t *testing.T) {
	r := testRule()
	r.MatchMode = "contains"
	r.KeywordLogic = "all"
	r.Keywords = []string{"he", "her", "你好"}
	require.Equal(t, "reply", check(t, []Rule{r}, "her 你好").Action)
	require.Empty(t, check(t, []Rule{r}, "he 你好").Hits)
	r.ExcludeKeywords = []string{"except"}
	require.Empty(t, check(t, []Rule{r}, "her 你好 except").Hits)
	r.MatchMode = "word"
	r.KeywordLogic = "any"
	r.Keywords = []string{"Hello"}
	require.Equal(t, "reply", check(t, []Rule{r}, "HelloWorld then Hello!").Action)
}
func TestKeywordRulesPriorityAndScope(t *testing.T) {
	one, two := testRule(), testRule()
	one.ID = "injection"
	one.Action = "inject"
	one.Injection = "System instruction"
	one.Reply = ""
	two.ID = "two"
	two.Priority = 20
	two.Reply = "Second reply"
	// A terminal rule makes earlier injections unnecessary; nothing is forwarded.
	got := check(t, []Rule{two, one}, "Hello")
	require.Equal(t, "Second reply", got.Reply)
	require.Nil(t, got.EffectiveBody)
	require.Len(t, got.Hits, 2)
	one.StopOnMatch = true
	body := []byte(`{"model":"alias","messages":[{"role":"user","content":"Hello"}]}`)
	got, err := Preview(Policy{Enabled: true, Rules: []Rule{two, one}}, Input{Protocol: "openai_chat_completions", Body: body})
	require.NoError(t, err)
	require.Equal(t, "forward", got.Action)
	require.Len(t, got.Hits, 1)
	require.Contains(t, string(got.EffectiveBody), "System instruction")
	two.UserIDs = []int64{2}
	two.GroupIDs = []int64{3}
	two.Models = []string{"alias"}
	two.Protocols = []string{"openai_chat_completions"}
	input := testInput("Hello")
	input.UserID = 2
	input.GroupID = 3
	input.Model = "alias"
	got, err = Preview(Policy{Rules: []Rule{two}}, input)
	require.NoError(t, err)
	require.Len(t, got.Hits, 1)
	input.GroupID = 4
	got, err = Preview(Policy{Rules: []Rule{two}}, input)
	require.NoError(t, err)
	require.Empty(t, got.Hits)
	input.GroupID = 3
	two.ExcludeUserIDs = []int64{2}
	got, err = Preview(Policy{Rules: []Rule{two}}, input)
	require.NoError(t, err)
	require.Empty(t, got.Hits)
}
func TestKeywordRuleScanScopesDoNotConfuseHistoryOrToolsWithCurrentQuestion(t *testing.T) {
	r := testRule()
	input := Input{Protocol: "openai_responses", Body: []byte(`{"instructions":"Hello","input":[{"role":"user","content":"Hello"},{"role":"assistant","content":"history"},{"role":"user","content":"current"}]}`)}
	got, err := Preview(Policy{Rules: []Rule{r}}, input)
	require.NoError(t, err)
	require.Empty(t, got.Hits)
	r.ScanScope = "all_user"
	r.MatchMode = "contains"
	got, err = Preview(Policy{Rules: []Rule{r}}, input)
	require.NoError(t, err)
	require.Len(t, got.Hits, 1)
	r.ScanScope = "context"
	r.Keywords = []string{"history"}
	got, err = Preview(Policy{Rules: []Rule{r}}, input)
	require.NoError(t, err)
	require.Len(t, got.Hits, 1)
	r.ScanScope = "latest_user"
	r.MatchMode = "exact"
	r.Keywords = []string{"Hello"}
	input.Body = []byte(`{"input":[{"role":"user","content":"Hello"},{"type":"function_call_output","output":"Hello"}]}`)
	got, err = Preview(Policy{Rules: []Rule{r}}, input)
	require.NoError(t, err)
	require.Empty(t, got.Hits)
	// Large historical input must not impose a limit on latest-question rules.
	large, _ := json.Marshal(map[string]any{"messages": []map[string]string{{"role": "assistant", "content": strings.Repeat("history", (MaxTextBytes/7)+1)}, {"role": "user", "content": "Hello"}}})
	input = Input{Protocol: "openai_chat_completions", Body: large}
	got, err = Preview(Policy{Rules: []Rule{r}}, input)
	require.NoError(t, err)
	require.Len(t, got.Hits, 1)
}
func TestKeywordInjectionPreservesEveryProtocolAndNumericLexeme(t *testing.T) {
	for _, tc := range []struct{ protocol, body, path string }{
		{"openai_chat_completions", `{"model":"alias","n":9007199254740993,"messages":[{"role":"system","content":"Original"},{"role":"user","content":"Hello"}]}`, "messages"},
		{"openai_responses", `{"type":"response.create","model":"alias","n":9007199254740993,"instructions":"Original","input":"Hello"}`, "instructions"},
		{"anthropic_messages", `{"model":"alias","n":9007199254740993,"system":[{"type":"text","text":"Original","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"Hello"}]}`, "system"},
		{"gemini", `{"n":9007199254740993,"systemInstruction":{"parts":[{"text":"Original"}]},"contents":[{"role":"user","parts":[{"text":"Hello"}]}]}`, "systemInstruction"},
	} {
		t.Run(tc.protocol, func(t *testing.T) {
			hits := []Hit{{Action: "inject", Injection: "Before", InjectionPosition: "prepend"}, {Action: "inject", Injection: "After", InjectionPosition: "append"}}
			body, err := Inject(tc.protocol, []byte(tc.body), hits)
			require.NoError(t, err)
			require.Contains(t, string(body), "9007199254740993")
			instructions := gjson.GetBytes(body, tc.path).Raw
			require.Less(t, strings.Index(instructions, "Before"), strings.Index(instructions, "Original"))
			require.Less(t, strings.Index(instructions, "Original"), strings.Index(instructions, "After"))
			require.Contains(t, string(body), "Hello")
			if tc.protocol == "anthropic_messages" {
				require.Contains(t, string(body), "cache_control")
			}
		})
	}
}

type memoryStore struct {
	mu     sync.Mutex
	values map[string]string
	fail   bool
}

func (s *memoryStore) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return nil, fmt.Errorf("offline")
	}
	out := map[string]string{}
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}
func (s *memoryStore) Set(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}
func TestKeywordRuleManagerRevisionImmediateSwitchesAndMigration(t *testing.T) {
	ctx := context.Background()
	store := &memoryStore{values: map[string]string{"risk_control_enabled": "true", "content_moderation_config": `{"enabled":true,"mode":"pre_block","all_groups":true,"blocked_keywords":["old"],"keyword_match_mode":"contains"}`}}
	m := NewManager(store)
	policy, err := m.Snapshot(ctx)
	require.NoError(t, err)
	require.True(t, policy.Legacy)
	require.Len(t, policy.Rules, 1)
	require.False(t, *policy.Rules[0].CaseSensitive)
	got, err := m.Evaluate(ctx, testInput("old"))
	require.NoError(t, err)
	require.False(t, got.Configured)
	policy.Rules = []Rule{testRule()}
	policy.Rules[0].Description = "Specific phrases avoid false positives."
	policy.Enabled = true
	saved, err := m.Save(ctx, policy)
	require.NoError(t, err)
	require.False(t, saved.Legacy)
	require.Equal(t, policy.Rules[0].Description, saved.Rules[0].Description)
	got, err = m.Evaluate(ctx, testInput("Hello"))
	require.NoError(t, err)
	require.Equal(t, "reply", got.Action)
	_, err = m.Save(ctx, policy)
	require.ErrorIs(t, err, ErrConflict)
	saved.Enabled = false
	saved, err = m.Save(ctx, saved)
	require.NoError(t, err)
	got, err = m.Evaluate(ctx, testInput("Hello"))
	require.NoError(t, err)
	require.Empty(t, got.Hits)
	require.True(t, got.Configured)
	saved.Rules[0].Reply = "changed externally"
	current, err := m.Snapshot(ctx)
	require.NoError(t, err)
	require.Equal(t, "Configured reply", current.Rules[0].Reply)
	require.True(t, Managed(WithManaged(ctx)))
	require.False(t, Managed(ctx))
}
func TestKeywordRuleValidationRejectsAmbiguousAndUnboundedConfiguration(t *testing.T) {
	r := testRule()
	r.MatchMode = "regex"
	r.Keywords = []string{"(?=unsupported)"}
	_, err := compile(Policy{Rules: []Rule{r}})
	require.Error(t, err)
	r = testRule()
	r.Keywords = []string{strings.Repeat("x", MaxKeywordRunes+1)}
	_, err = compile(Policy{Rules: []Rule{r}})
	require.Error(t, err)
	r = testRule()
	_, err = compile(Policy{Rules: []Rule{r, r}})
	require.Error(t, err)
	r.Injection = strings.Repeat("x", 16001)
	_, err = compile(Policy{Rules: []Rule{r}})
	require.Error(t, err)
}
func TestKeywordRuleLargeLibrary(t *testing.T) {
	rules := make([]Rule, 10)
	for i := range rules {
		r := testRule()
		r.ID = fmt.Sprint(i)
		r.MatchMode = "contains"
		r.Keywords = make([]string, 5000)
		for j := range r.Keywords {
			r.Keywords[j] = fmt.Sprintf("KEY_%d_%04d", i, j)
		}
		rules[i] = r
	}
	compiled, err := compile(Policy{Enabled: true, Rules: rules})
	require.NoError(t, err)
	got, err := compiled.evaluate(testInput(strings.Repeat("ordinary text ", 20000)+"KEY_9_4999"), false)
	require.NoError(t, err)
	require.Len(t, got.Hits, 1)
	require.Equal(t, "9", got.Hits[0].RuleID)
}

func TestKeywordRegexPreservesEscapesAndWhitespace(t *testing.T) {
	r := testRule()
	r.MatchMode, r.KeywordLogic, r.CaseSensitive = "regex", "all", boolean(false)
	r.Keywords = []string{`\D`, `\d`}
	require.Empty(t, check(t, []Rule{r}, "1").Hits)
	require.Len(t, check(t, []Rule{r}, "A1").Hits, 1)
	r.KeywordLogic, r.Keywords = "any", []string{`^  $`}
	require.Len(t, check(t, []Rule{r}, "  ").Hits, 1)
	require.Empty(t, check(t, []Rule{r}, " ").Hits)
}
func TestKeywordInjectionRejectsAmbiguousSystemFields(t *testing.T) {
	for _, body := range []string{`{"instructions":"first","instructions":"second","input":"Hello"}`, `{"Instructions":"first","instructions":"second","input":"Hello"}`} {
		_, err := Inject("openai_responses", []byte(body), []Hit{{Action: "inject", Injection: "policy"}})
		require.Error(t, err)
	}
}

func TestKeywordLegacyMigrationPreservesDisabledModeAndExemptions(t *testing.T) {
	words := make([]string, 6000)
	for i := range words {
		words[i] = fmt.Sprintf("old-%d", i)
	}
	raw, err := json.Marshal(map[string]any{"enabled": true, "mode": "pre_block", "keyword_blocking_mode": "api_only", "all_groups": true, "blocked_keywords": words, "keyword_match_mode": "exact", "keyword_auto_reply_enabled": true, "keyword_reply": ""})
	require.NoError(t, err)
	p := legacyPolicy(map[string]string{"content_moderation_config": string(raw), "risk_control_enabled": "true", "cyber_policy_user_allowlist": "12, 34"})
	require.False(t, p.Enabled)
	require.Len(t, p.Rules, 2)
	require.Equal(t, "block", p.Rules[0].Action)
	require.Equal(t, []int64{12, 34}, p.Rules[0].ExcludeUserIDs)
	_, err = compile(p)
	require.NoError(t, err)
}

func TestKeywordRuleDescriptionIsBoundedAndDoesNotAffectMatching(t *testing.T) {
	r := testRule()
	r.Description = "Usage notes and examples for administrators"
	require.Equal(t, "reply", check(t, []Rule{r}, "Hello").Action)
	require.Empty(t, check(t, []Rule{r}, r.Description).Hits)
	r.Description = strings.Repeat("x", 2001)
	_, err := compile(Policy{Rules: []Rule{r}})
	require.Error(t, err)
}
