import { describe, expect, it } from 'vitest'
import { createKeywordRule, deduplicateKeywords, KeywordImportError, parseKeywordCSV, parseKeywordImport, validateKeywordPolicy } from '../keywordRules'

describe('keyword library import and validation', () => {
  it('preserves exact phrases, punctuation and casing in bulk keywords', () => {
    const parsed = parseKeywordImport('  Hello  World  \nhello\nHello\n\n', 'txt', 'keywords')
    expect(parsed.keywords).toEqual(['Hello  World', 'hello', 'Hello'])
    expect(deduplicateKeywords(parsed.keywords)).toEqual(['Hello  World', 'hello', 'Hello'])
    expect(deduplicateKeywords(parsed.keywords, false)).toEqual(['Hello  World', 'hello'])
    expect(parseKeywordImport('\\D\n\\d\n leading space', 'txt', 'keywords', true).keywords).toEqual(['\\D', '\\d', ' leading space'])
  })
  it('parses quoted CSV without splitting commas or multiline content', () => {
    expect(parseKeywordCSV('\uFEFF关键词,回复\r\n"Hello, world","line 1\nline 2 ""quoted"""')).toEqual([['关键词', '回复'], ['Hello, world', 'line 1\nline 2 "quoted"']])
    const imported = parseKeywordImport('名称,关键词,动作,回复,注入,匹配方式\n服务,帮助,回复,查看帮助中心,,完全一致\n开发,代码,注入,,请优先提供可运行的代码,包含', 'csv', 'rules')
    expect(imported.rules).toHaveLength(2)
    expect(imported.rules[0]).toMatchObject({ name: '服务', keywords: ['帮助'], action: 'reply', reply: '查看帮助中心', match_mode: 'exact' })
    expect(imported.rules[1]).toMatchObject({ name: '开发', action: 'inject', injection: '请优先提供可运行的代码', match_mode: 'contains' })
  })
  it('roundtrips exported rules including exclusions, scope and injection', () => {
    const rule = createKeywordRule('开发规则')
    Object.assign(rule, { keywords: ['code', 'multi\nline'], exclude_keywords: ['skip'], action: 'inject', injection: 'System instructions', group_ids: [3], models: ['alias'], injection_position: 'prepend' })
    const json = JSON.stringify({ version: 1, enabled: true, rules: [rule] })
    expect(parseKeywordImport(json, 'json', 'rules').rules).toEqual([rule])
    expect(parseKeywordImport(json, 'json', 'keywords').keywords).toEqual(rule.keywords)
  })
  it('rejects malformed or ambiguous imports instead of guessing', () => {
    expect(() => parseKeywordImport('"unclosed', 'csv', 'rules')).toThrow(KeywordImportError)
    expect(() => parseKeywordImport('{not json', 'json', 'rules')).toThrow(KeywordImportError)
    expect(() => parseKeywordImport('[1,2]', 'json', 'keywords')).toThrow(KeywordImportError)
    expect(() => parseKeywordImport('name,reply\na,b', 'csv', 'rules')).toThrow(KeywordImportError)
    expect(() => parseKeywordImport('{"version":2,"rules":[]}', 'json', 'rules')).toThrow(KeywordImportError)
  })
  it('validates actions and scope before sending a policy', () => {
    const rule = createKeywordRule('New rule')
    const policy = { version: 1, revision: 'revision', enabled: true, legacy: false, rules: [rule] }
    expect(validateKeywordPolicy(policy)).toBe('incompleteRule')
    rule.keywords = ['Hello']; rule.reply = 'Reply'
    expect(validateKeywordPolicy(policy)).toBeNull()
    rule.user_ids = [Number.NaN]
    expect(validateKeywordPolicy(policy)).toBe('scopeError')
  })
})
