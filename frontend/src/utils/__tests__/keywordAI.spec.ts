import { describe, expect, it } from 'vitest'
import { applyKeywordAISelection, keywordAIExampleRule, keywordAIMessages, parseKeywordAI, KeywordAIError, type KeywordAIBrief } from '../keywordAI'
import { createKeywordRule } from '../keywordRules'

const brief: KeywordAIBrief = { goal: '精确识别余额查询', content: '原样回复\n第二行', action: 'reply', matchMode: 'exact', count: 3, language: 'zh' }
function document(extra = {}) {
  return { summary: '只对完整提问进行精确匹配。', rules: [{ name: '余额查询', keywords: ['查询余额', '我的余额是多少'], reply: 'AI draft', injection: '', reason: '使用完整提问，避免“余额”误触发其他问题。', examples: [{ text: '查询余额', should_match: true, explanation: '完整一致' }, { text: '如何充值余额', should_match: false, explanation: '不是配置的完整提问' }], ...extra }] }
}
describe('AI keyword drafts', () => {
  it('treats AI fields as untrusted and pins the administrator’s action and exact matching', () => {
    const result = parseKeywordAI(JSON.stringify(document({ enabled: true, action: 'inject', match_mode: 'contains', group_ids: [9], user_ids: [1], id: 'existing', case_sensitive: false })), brief)
    const rule = result.suggestions[0].rule
    expect(rule).toMatchObject({ action: 'reply', match_mode: 'exact', enabled: false, case_sensitive: true, scan_scope: 'latest_user', group_ids: [], user_ids: [], description: result.suggestions[0].reason, reply: brief.content })
    expect(rule.id).not.toBe('existing')
    expect(result.suggestions[0].examples.map(example => example.should_match)).toEqual([true, false])
  })
  it('preserves literal phrases and regular expression escape semantics', () => {
    const result = parseKeywordAI(JSON.stringify(document({ keywords: ['\\D', '\\d', '  ^X$  '] })), { ...brief, matchMode: 'regex' })
    expect(result.suggestions[0].keywords).toEqual(['\\D', '\\d', '  ^X$  '])
    expect(parseKeywordAI('```json\n'+JSON.stringify(document())+'\n```', brief).suggestions).toHaveLength(1)
  })
  it('rejects invalid, oversized, or uninformative generation results', () => {
    for (const value of ['text', '{}', JSON.stringify(document({ keywords: [123] })), JSON.stringify(document({ keywords: ['x'.repeat(513)] })), JSON.stringify(document({ examples: [] })), JSON.stringify({ ...document(), rules: Array(4).fill(document().rules[0]) })]) expect(() => parseKeywordAI(value, brief)).toThrow(KeywordAIError)
    expect(() => parseKeywordAI(JSON.stringify(document({ examples: document().rules[0].examples.map(example => ({ ...example, should_match: true })) })), brief)).toThrow(KeywordAIError)
  })
  it('appends new phrases while keeping current behavior and rejecting stale targets', () => {
    const target = { ...createKeywordRule('已有规则'), keywords: ['查询余额'], reply: 'Existing reply', group_ids: [3], keyword_logic: 'all' as const, case_sensitive: false }
    const result = parseKeywordAI(JSON.stringify(document({ keywords: ['查询余额', '剩余额度'] })), { ...brief, target })
    const suggestion = result.suggestions[0]
    expect(suggestion.keywords).toEqual(['剩余额度'])
    expect(suggestion.rule).toMatchObject({ keywords: ['查询余额', '剩余额度'], reply: 'Existing reply', group_ids: [3], keyword_logic: 'all', case_sensitive: false })
    const policy = { version: 1, revision: 'rev', legacy: false, enabled: false, rules: [target] }
    const selection = { rules: [], targetId: target.id, targetSnapshot: JSON.stringify(target), keywords: suggestion.keywords, description: suggestion.reason }
    const next = applyKeywordAISelection(policy, selection)
    expect(next.enabled).toBe(false)
    expect(next.rules[0]).toMatchObject({ reply: target.reply, enabled: true, keywords: ['查询余额', '剩余额度'], description: suggestion.reason })
    expect(policy.rules[0].keywords).toEqual(['查询余额'])
    expect(() => applyKeywordAISelection({ ...policy, rules: [{ ...target, reply: 'Edited' }] }, selection)).toThrow('aiTargetChanged')
  })
  it('never activates new rules or changes the global switch when accepting suggestions', () => {
    const result = parseKeywordAI(JSON.stringify(document()), brief)
    const policy = { version: 1, revision: 'rev', legacy: false, enabled: true, rules: [] }
    const next = applyKeywordAISelection(policy, { rules: result.suggestions.map(s => s.rule) })
    expect(next.enabled).toBe(true)
    expect(next.rules[0].enabled).toBe(false)
    expect(next.rules[0].description).toContain('避免')
    expect(policy.rules).toEqual([])
  })
  it('sends only the explicit brief and bounded current rule, with no member history or credentials', () => {
    const input = keywordAIMessages({ ...brief, target: { ...createKeywordRule('Expand'), keywords: Array(200).fill('word') } })
    const payload = JSON.parse(input[1].content)
    expect(payload.existing_rule.keywords).toHaveLength(100)
    expect(payload.count).toBe(1)
    expect(payload).not.toHaveProperty('key')
    expect(payload).not.toHaveProperty('conversations')
    const scoped = { ...createKeywordRule('Scoped'), user_ids: [1], group_ids: [2], models: ['alias'] }
    expect(keywordAIExampleRule(scoped)).toMatchObject({ enabled: true, user_ids: [], group_ids: [], models: [] })
    expect(scoped.user_ids).toEqual([1])
  })
})
