package keywordrules

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type compiledRule struct {
	rule               Rule
	positive, negative []string
	expressions        []*regexp.Regexp
}
type compiledPolicy struct {
	policy Policy
	rules  []compiledRule
	scans  map[string]*automaton
}
type pattern struct {
	rule, index int
	text        string
	negative    bool
}

func boolean(value bool) *bool { return &value }
func validString(value string, max int) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= max
}
func uniqueWords(words []string, sensitive bool) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, word := range words {
		word = strings.TrimSpace(word)
		if word == "" {
			continue
		}
		if !validString(word, MaxKeywordRunes) {
			return nil, fmt.Errorf("each keyword must contain at most %d valid characters", MaxKeywordRunes)
		}
		key := word
		if !sensitive {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, word)
		}
	}
	return out, nil
}
func uniqueIDs(ids []int64) ([]int64, error) {
	out := []int64{}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id < 1 {
			return nil, fmt.Errorf("scope IDs must be positive")
		}
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out, nil
}

func compile(policy Policy) (*compiledPolicy, error) {
	if policy.Version != 0 && policy.Version != 1 {
		return nil, fmt.Errorf("unsupported keyword rules version")
	}
	if len(policy.Rules) > MaxRules {
		return nil, fmt.Errorf("at most %d rules are supported", MaxRules)
	}
	policy.Version = 1
	policy.Legacy = false
	if policy.Rules == nil {
		policy.Rules = []Rule{}
	}
	ids := map[string]bool{}
	total, characters, regexCount := 0, 0, 0
	for i := range policy.Rules {
		r := &policy.Rules[i]
		if r.ID == "" || !validString(r.ID, 80) || ids[r.ID] {
			return nil, fmt.Errorf("rule IDs must be nonempty and unique")
		}
		ids[r.ID] = true
		r.Name = strings.TrimSpace(r.Name)
		r.Category = strings.TrimSpace(r.Category)
		if r.Name == "" || !validString(r.Name, 100) || !validString(r.Category, 60) {
			return nil, fmt.Errorf("rule name is required (up to 100 characters)")
		}
		if r.Priority < 0 || r.Priority > 100000 {
			return nil, fmt.Errorf("rule priority must be between 0 and 100000")
		}
		if r.CaseSensitive == nil {
			r.CaseSensitive = boolean(true)
		}
		if r.MatchMode == "" {
			r.MatchMode = "exact"
		}
		if r.KeywordLogic == "" {
			r.KeywordLogic = "any"
		}
		if r.ScanScope == "" {
			r.ScanScope = "latest_user"
		}
		if r.InjectionPosition == "" {
			r.InjectionPosition = "append"
		}
		switch r.MatchMode {
		case "exact", "contains", "word", "prefix", "suffix", "regex":
		default:
			return nil, fmt.Errorf("invalid match mode in %s", r.Name)
		}
		if r.KeywordLogic != "any" && r.KeywordLogic != "all" {
			return nil, fmt.Errorf("invalid keyword condition in %s", r.Name)
		}
		if r.ScanScope != "latest_user" && r.ScanScope != "all_user" && r.ScanScope != "context" {
			return nil, fmt.Errorf("invalid scan scope in %s", r.Name)
		}
		switch r.Action {
		case "reply":
			if strings.TrimSpace(r.Reply) == "" {
				return nil, fmt.Errorf("reply text is required in %s", r.Name)
			}
		case "inject":
			if strings.TrimSpace(r.Injection) == "" {
				return nil, fmt.Errorf("injection text is required in %s", r.Name)
			}
		case "block":
		default:
			return nil, fmt.Errorf("invalid action in %s", r.Name)
		}
		if !validString(r.Reply, 8000) || !validString(r.Injection, 16000) {
			return nil, fmt.Errorf("reply or injection text is too long in %s", r.Name)
		}
		if r.InjectionPosition != "prepend" && r.InjectionPosition != "append" {
			return nil, fmt.Errorf("invalid injection position in %s", r.Name)
		}
		var err error
		if r.MatchMode == "regex" {
			r.Keywords, err = uniquePatterns(r.Keywords)
		} else {
			r.Keywords, err = uniqueWords(r.Keywords, *r.CaseSensitive)
		}
		if err != nil {
			return nil, err
		}
		r.ExcludeKeywords, err = uniqueWords(r.ExcludeKeywords, *r.CaseSensitive)
		if err != nil {
			return nil, err
		}
		if len(r.Keywords) == 0 || len(r.Keywords) > 5000 {
			return nil, fmt.Errorf("each rule needs 1–5000 keywords")
		}
		total += len(r.Keywords) + len(r.ExcludeKeywords)
		for _, word := range append(append([]string{}, r.Keywords...), r.ExcludeKeywords...) {
			characters += len(word)
		}
		if total > MaxKeywords || characters > 2<<20 {
			return nil, fmt.Errorf("the keyword library exceeds its capacity (50000 keywords / 2 MiB text)")
		}
		if r.MatchMode == "regex" {
			regexCount += len(r.Keywords)
			if regexCount > 100 {
				return nil, fmt.Errorf("at most 100 regular expressions are supported")
			}
		}
		r.GroupIDs, err = uniqueIDs(r.GroupIDs)
		if err != nil {
			return nil, err
		}
		r.UserIDs, err = uniqueIDs(r.UserIDs)
		if err != nil {
			return nil, err
		}
		r.ExcludeUserIDs, err = uniqueIDs(r.ExcludeUserIDs)
		if err != nil {
			return nil, err
		}
		if r.Models == nil {
			r.Models = []string{}
		}
		if r.ExcludeModels == nil {
			r.ExcludeModels = []string{}
		}
		if r.Protocols == nil {
			r.Protocols = []string{}
		}
		for _, model := range append(append([]string{}, r.Models...), r.ExcludeModels...) {
			if !validString(model, 255) || strings.TrimSpace(model) == "" || strings.ContainsAny(model, "\r\n") {
				return nil, fmt.Errorf("invalid model scope")
			}
		}
		for _, protocol := range r.Protocols {
			switch protocol {
			case "openai_chat_completions", "openai_responses", "anthropic_messages", "gemini":
			default:
				return nil, fmt.Errorf("invalid protocol scope")
			}
		}
	}
	sort.SliceStable(policy.Rules, func(i, j int) bool { return policy.Rules[i].Priority < policy.Rules[j].Priority })
	compiled := &compiledPolicy{policy: policy, scans: map[string]*automaton{}}
	patterns := map[string][]pattern{}
	for i, r := range policy.Rules {
		cr := compiledRule{rule: r}
		for _, word := range r.Keywords {
			if !*r.CaseSensitive {
				word = strings.ToLower(word)
			}
			cr.positive = append(cr.positive, word)
		}
		for _, word := range r.ExcludeKeywords {
			if !*r.CaseSensitive {
				word = strings.ToLower(word)
			}
			cr.negative = append(cr.negative, word)
		}
		if r.MatchMode == "regex" {
			for _, word := range r.Keywords {
				if !*r.CaseSensitive {
					word = "(?i:" + word + ")"
				}
				re, err := regexp.Compile(word)
				if err != nil {
					return nil, fmt.Errorf("invalid regular expression in %s: %s", r.Name, err)
				}
				cr.expressions = append(cr.expressions, re)
			}
		}
		compiled.rules = append(compiled.rules, cr)
		if !r.Enabled {
			continue
		}
		base := r.ScanScope + fmt.Sprint(*r.CaseSensitive)
		for n, word := range cr.negative {
			patterns[base+"contains"] = append(patterns[base+"contains"], pattern{rule: i, index: n, text: word, negative: true})
		}
		if r.MatchMode == "contains" || r.MatchMode == "word" {
			for n, word := range cr.positive {
				patterns[base+r.MatchMode] = append(patterns[base+r.MatchMode], pattern{rule: i, index: n, text: word})
			}
		}
	}
	for key, items := range patterns {
		compiled.scans[key] = newAutomaton(items)
	}
	return compiled, nil
}

func uniquePatterns(words []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, word := range words {
		if word == "" {
			continue
		}
		if !validString(word, MaxKeywordRunes) {
			return nil, fmt.Errorf("regular expression is too long")
		}
		if !seen[word] {
			seen[word] = true
			out = append(out, word)
		}
	}
	return out, nil
}
