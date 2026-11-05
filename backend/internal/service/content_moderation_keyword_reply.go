package service

import (
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/tidwall/gjson"
)

func validateKeywordReplyConfig(cfg *ContentModerationConfig) error {
	mode := cfg.KeywordMatchMode
	if mode != "" && mode != "contains" && mode != "exact" && mode != "literal" {
		return infraerrors.BadRequest("INVALID_KEYWORD_MATCH_MODE", "请选择有效的关键词匹配方式")
	}
	if utf8.RuneCountInString(cfg.KeywordReply) > 4000 {
		return infraerrors.BadRequest("INVALID_KEYWORD_REPLY", "自动回复最多 4000 个字符")
	}
	if mode == "exact" || mode == "literal" {
		if len(cfg.BlockedKeywords) > maxContentModerationBlockedKeywords {
			return infraerrors.BadRequest("INVALID_KEYWORD_LIST", "关键词数量超过上限")
		}
		for _, word := range cfg.BlockedKeywords {
			if utf8.RuneCountInString(strings.TrimSpace(word)) > maxContentModerationBlockedKeywordRunes {
				return infraerrors.BadRequest("INVALID_KEYWORD_LENGTH", "精确关键词最多 200 个字符，不会自动截短")
			}
		}
	}
	if cfg.Enabled && cfg.KeywordAutoReplyEnabled {
		if mode != "exact" && mode != "literal" {
			return infraerrors.BadRequest("PRECISE_MATCH_REQUIRED", "自动回复必须选择精确匹配或原文包含匹配")
		}
		if strings.TrimSpace(cfg.KeywordReply) == "" {
			return infraerrors.BadRequest("KEYWORD_REPLY_REQUIRED", "请填写命中关键词后的回复内容")
		}
	}
	return nil
}

func normalizePreciseKeywords(words []string) []string {
	result := make([]string, 0, len(words))
	seen := make(map[string]bool)
	for _, raw := range words {
		word := strings.TrimSpace(raw)
		if word != "" && !seen[word] {
			seen[word] = true
			result = append(result, word)
		}
	}
	return result
}

func matchPreciseKeyword(text string, words []string, mode string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	for _, word := range words {
		if word != "" && ((mode == "exact" && text == word) || (mode == "literal" && strings.Contains(text, word))) {
			return word, true
		}
	}
	return "", false
}

// Retain original casing, punctuation and interior whitespace for exact rules.
// The current-user boundaries match the existing moderation input collector.
func extractLiteralKeywordText(protocol string, body []byte) string {
	if !gjson.ValidBytes(body) {
		return ""
	}
	collector := moderationTextCollector{filterReminders: false, preserveWhitespace: true}
	var parts, images []string
	switch protocol {
	case ContentModerationProtocolAnthropicMessages:
		collector.collectLastAnthropicUserMessage(gjson.GetBytes(body, "messages"), &parts, &images)
	case ContentModerationProtocolOpenAIChat:
		collector.collectLastRoleMessage(gjson.GetBytes(body, "messages"), "user", &parts, &images)
	case ContentModerationProtocolOpenAIResponses:
		collector.collectLastResponsesInput(gjson.GetBytes(body, "input"), &parts, &images)
	case ContentModerationProtocolGemini:
		collector.collectLastGeminiContent(gjson.GetBytes(body, "contents"), &parts, &images)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func supportsKeywordReplyProtocol(protocol string) bool {
	switch protocol {
	case ContentModerationProtocolOpenAIChat, ContentModerationProtocolOpenAIResponses, ContentModerationProtocolAnthropicMessages, ContentModerationProtocolGemini:
		return true
	}
	return false
}
