package keywordrules

import (
	"fmt"
	"strings"
	"unicode"
)

type node struct {
	next         map[rune]int
	fail, output int
	tags         []int
}
type automaton struct {
	nodes    []node
	patterns []pattern
	lengths  []int
}

func newNode() node { return node{next: map[rune]int{}, output: -1} }
func newAutomaton(patterns []pattern) *automaton {
	a := &automaton{nodes: []node{newNode()}, patterns: patterns}
	for index, p := range patterns {
		state := 0
		word := []rune(p.text)
		a.lengths = append(a.lengths, len(word))
		for _, letter := range word {
			next, ok := a.nodes[state].next[letter]
			if !ok {
				next = len(a.nodes)
				a.nodes = append(a.nodes, newNode())
				a.nodes[state].next[letter] = next
			}
			state = next
		}
		a.nodes[state].tags = append(a.nodes[state].tags, index)
	}
	queue := []int{}
	for _, child := range a.nodes[0].next {
		queue = append(queue, child)
	}
	for head := 0; head < len(queue); head++ {
		state := queue[head]
		for letter, child := range a.nodes[state].next {
			failure := a.nodes[state].fail
			for failure != 0 {
				if _, ok := a.nodes[failure].next[letter]; ok {
					break
				}
				failure = a.nodes[failure].fail
			}
			if fallback, ok := a.nodes[failure].next[letter]; ok {
				a.nodes[child].fail = fallback
			}
			fallback := a.nodes[child].fail
			if len(a.nodes[fallback].tags) > 0 {
				a.nodes[child].output = fallback
			} else {
				a.nodes[child].output = a.nodes[fallback].output
			}
			queue = append(queue, child)
		}
	}
	return a
}
func wordCharacter(letter rune) bool {
	return unicode.IsLetter(letter) || unicode.IsNumber(letter) || letter == '_'
}

// Each valid pattern is reported once. Completed output nodes are skipped for
// the remainder of the scan, bounding repeated/overlapping keyword work.
func (a *automaton) scan(text []rune, word bool, applicable []bool, matches []map[int]bool, excluded []bool) {
	state := 0
	seen := make([]bool, len(a.patterns))
	skip := make([]int, len(a.nodes))
	for i := range skip {
		skip[i] = -2
	}
	var unseen func(int) int
	unseen = func(index int) int {
		if index < 0 {
			return -1
		}
		if skip[index] == -2 {
			return index
		}
		skip[index] = unseen(skip[index])
		return skip[index]
	}
	for index, letter := range text {
		for state != 0 {
			if _, ok := a.nodes[state].next[letter]; ok {
				break
			}
			state = a.nodes[state].fail
		}
		if next, ok := a.nodes[state].next[letter]; ok {
			state = next
		}
		if word && index+1 < len(text) && wordCharacter(text[index+1]) {
			continue
		}
		output := state
		if len(a.nodes[output].tags) == 0 {
			output = a.nodes[output].output
		}
		for output = unseen(output); output >= 0; {
			complete := true
			for _, tag := range a.nodes[output].tags {
				if seen[tag] {
					continue
				}
				p := a.patterns[tag]
				if !applicable[p.rule] {
					seen[tag] = true
					continue
				}
				start := index + 1 - a.lengths[tag]
				if word && start > 0 && wordCharacter(text[start-1]) {
					complete = false
					continue
				}
				seen[tag] = true
				if p.negative {
					excluded[p.rule] = true
				} else {
					matches[p.rule][p.index] = true
				}
			}
			next := a.nodes[output].output
			if complete {
				skip[output] = unseen(next)
			}
			output = unseen(next)
		}
	}
}
func containsID(values []int64, value int64) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func applies(r Rule, input Input) bool {
	return r.Enabled && (len(r.GroupIDs) == 0 || containsID(r.GroupIDs, input.GroupID)) && (len(r.UserIDs) == 0 || containsID(r.UserIDs, input.UserID)) && !containsID(r.ExcludeUserIDs, input.UserID) && (len(r.Models) == 0 || containsString(r.Models, input.Model)) && !containsString(r.ExcludeModels, input.Model) && (len(r.Protocols) == 0 || containsString(r.Protocols, input.Protocol))
}
func (c *compiledPolicy) evaluate(input Input, preview bool) (Result, error) {
	result := Result{Enabled: c.policy.Enabled, Hits: []Hit{}, Action: "forward"}
	if !preview && !c.policy.Enabled {
		return result, nil
	}
	applicable := make([]bool, len(c.rules))
	matches := make([]map[int]bool, len(c.rules))
	excluded := make([]bool, len(c.rules))
	needed := map[string]bool{}
	for i, r := range c.rules {
		applicable[i] = applies(r.rule, input)
		matches[i] = map[int]bool{}
		if applicable[i] {
			needed[r.rule.ScanScope] = true
		}
	}
	texts, err := extractTexts(input, needed)
	if err != nil {
		return result, err
	}
	normalized := map[string]string{}
	for _, scope := range []string{"latest_user", "all_user", "context"} {
		for _, sensitive := range []bool{true, false} {
			key := scope + fmt.Sprint(sensitive)
			text := strings.TrimSpace(texts[scope])
			if !sensitive {
				text = strings.ToLower(text)
			}
			normalized[key] = text
			if a := c.scans[key+"contains"]; a != nil {
				a.scan([]rune(text), false, applicable, matches, excluded)
			}
			if a := c.scans[key+"word"]; a != nil {
				a.scan([]rune(text), true, applicable, matches, excluded)
			}
		}
	}
	for i, cr := range c.rules {
		if !applicable[i] || excluded[i] {
			continue
		}
		r := cr.rule
		text := normalized[r.ScanScope+fmt.Sprint(*r.CaseSensitive)]
		if text == "" && r.MatchMode != "regex" {
			continue
		}
		switch r.MatchMode {
		case "exact", "prefix", "suffix":
			for index, word := range cr.positive {
				if (r.MatchMode == "exact" && text == word) || (r.MatchMode == "prefix" && strings.HasPrefix(text, word)) || (r.MatchMode == "suffix" && strings.HasSuffix(text, word)) {
					matches[i][index] = true
				}
			}
		case "regex":
			text = texts[r.ScanScope]
			for index, re := range cr.expressions {
				if re.MatchString(text) {
					matches[i][index] = true
				}
			}
		}
		if len(matches[i]) == 0 || (r.KeywordLogic == "all" && len(matches[i]) != len(r.Keywords)) {
			continue
		}
		hit := Hit{RuleID: r.ID, RuleName: r.Name, Action: r.Action, Keywords: []string{}}
		for index, word := range r.Keywords {
			if matches[i][index] && len(hit.Keywords) < 50 {
				hit.Keywords = append(hit.Keywords, word)
			}
		}
		if r.Action == "inject" {
			hit.Injection, hit.InjectionPosition = r.Injection, r.InjectionPosition
		}
		result.Hits = append(result.Hits, hit)
		if r.Action == "reply" || r.Action == "block" {
			result.Action, result.Reply = r.Action, r.Reply
			return result, nil
		}
		if r.StopOnMatch {
			break
		}
	}
	result.EffectiveBody, err = Inject(input.Protocol, input.Body, result.Hits)
	return result, err
}
