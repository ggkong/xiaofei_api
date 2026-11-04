package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func preciseReplyService(t *testing.T, mode string, words ...string) *ContentModerationService {
	t.Helper()
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.Mode = ContentModerationModePreBlock
	cfg.KeywordBlockingMode = ContentModerationKeywordModeKeywordOnly
	cfg.KeywordMatchMode = mode
	cfg.KeywordAutoReplyEnabled = true
	cfg.KeywordReply = "这是指定回复。"
	cfg.BlockedKeywords = words
	raw, err := json.Marshal(cfg)
	require.NoError(t, err)
	repo := &contentModerationRuntimeSettingRepo{values: map[string]string{SettingKeyRiskControlEnabled: "true", SettingKeyContentModerationConfig: string(raw)}}
	return runtimeCacheTestService(repo, time.Hour)
}

func replyInput(t *testing.T, text string) ContentModerationCheckInput {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"messages": []any{map[string]string{"role": "user", "content": text}}})
	require.NoError(t, err)
	return ContentModerationCheckInput{Protocol: ContentModerationProtocolOpenAIChat, Model: "test", Body: raw}
}

func TestKeywordAutoReplyRequiresExactOriginalQuestion(t *testing.T) {
	svc := preciseReplyService(t, "exact", "Hello", "A  B", "a+b", "危险")
	for _, tc := range []struct {
		text string
		hit  bool
	}{
		{"Hello", true}, {" Hello \n", true}, {"hello", false}, {"Hello!", false}, {"say Hello", false},
		{"A  B", true}, {"A B", false}, {"A\nB", false}, {"a+b", true}, {"aab", false}, {"危险", true}, {"危險", false}, {"危险吗", false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			decision, err := svc.Check(context.Background(), replyInput(t, tc.text))
			require.NoError(t, err)
			if tc.hit {
				require.True(t, decision.Blocked)
				require.Equal(t, ContentModerationActionKeywordReply, decision.Action)
				require.Equal(t, "这是指定回复。", decision.Message)
				require.Equal(t, 200, decision.StatusCode)
				require.False(t, decision.Flagged)
			} else {
				require.True(t, decision.Allowed)
				require.NotEqual(t, ContentModerationActionKeywordReply, decision.Action)
			}
		})
	}
}

func TestKeywordAutoReplyLiteralModeIsCaseSensitiveAndNotRegex(t *testing.T) {
	svc := preciseReplyService(t, "literal", "Hello", "a+b")
	for _, tc := range []struct {
		text string
		hit  bool
	}{{"before Hello after", true}, {"before hello after", false}, {"contains a+b", true}, {"contains aaab", false}} {
		decision, err := svc.Check(context.Background(), replyInput(t, tc.text))
		require.NoError(t, err)
		require.Equal(t, tc.hit, decision.Action == ContentModerationActionKeywordReply, tc.text)
	}
}

func TestKeywordAutoReplyPreservesWhitespaceBetweenTextBlocks(t *testing.T) {
	svc := preciseReplyService(t, "exact", "A\nB")
	for _, tc := range []struct {
		body string
		hit  bool
	}{
		{`{"messages":[{"role":"user","content":[{"type":"text","text":"A"},{"type":"text","text":"B"}]}]}`, true},
		{`{"messages":[{"role":"user","content":[{"type":"text","text":"A "},{"type":"text","text":" B"}]}]}`, false},
	} {
		decision, err := svc.Check(context.Background(), ContentModerationCheckInput{Protocol: ContentModerationProtocolOpenAIChat, Model: "test", Body: []byte(tc.body)})
		require.NoError(t, err)
		require.Equal(t, tc.hit, decision.Action == ContentModerationActionKeywordReply)
	}
}

func TestKeywordRulesCanBePausedWithoutRemovingConfiguration(t *testing.T) {
	svc := preciseReplyService(t, "exact", "Hello")
	decision, err := svc.Check(context.Background(), replyInput(t, "Hello"))
	require.NoError(t, err)
	require.Equal(t, ContentModerationActionKeywordReply, decision.Action)
	off := false
	view, err := svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{Enabled: &off})
	require.NoError(t, err)
	require.Equal(t, []string{"Hello"}, view.BlockedKeywords)
	require.Equal(t, "这是指定回复。", view.KeywordReply)
	decision, err = svc.Check(context.Background(), replyInput(t, "Hello"))
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	on := true
	_, err = svc.UpdateConfig(context.Background(), UpdateContentModerationConfigInput{Enabled: &on, KeywordAutoReplyEnabled: &off})
	require.NoError(t, err)
	decision, err = svc.Check(context.Background(), replyInput(t, "Hello"))
	require.NoError(t, err)
	require.Equal(t, ContentModerationActionKeywordBlock, decision.Action)
}

func TestKeywordAutoReplyValidationNeverShortensPreciseWords(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.Enabled = true
	cfg.KeywordAutoReplyEnabled = true
	cfg.KeywordReply = "reply"
	cfg.KeywordMatchMode = "exact"
	cfg.BlockedKeywords = []string{strings.Repeat("x", 201)}
	require.Error(t, validateKeywordReplyConfig(cfg))
	cfg.BlockedKeywords = []string{"ok"}
	cfg.KeywordMatchMode = "contains"
	require.Error(t, validateKeywordReplyConfig(cfg))
	cfg.Enabled = false
	require.NoError(t, validateKeywordReplyConfig(cfg))
	require.Equal(t, []string{"Hello", "hello"}, normalizePreciseKeywords([]string{"Hello", "hello", "Hello"}))
}
