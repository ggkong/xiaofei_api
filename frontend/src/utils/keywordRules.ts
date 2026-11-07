import type { KeywordRule, KeywordPolicy, KeywordAction, KeywordMatchMode, KeywordProtocol } from '@/api/admin/keywordRules'

export class KeywordImportError extends Error {
  constructor(public key: string) { super(key) }
}
export const ruleID = () => `rule_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 10)}`
export function createKeywordRule(name = ''): KeywordRule {
  return { id: ruleID(), name, description: '', category: '', enabled: true, priority: 100, keywords: [], exclude_keywords: [], match_mode: 'exact', keyword_logic: 'any', case_sensitive: true, scan_scope: 'latest_user', action: 'reply', reply: '', injection: '', injection_position: 'append', stop_on_match: false, group_ids: [], user_ids: [], exclude_user_ids: [], models: [], exclude_models: [], protocols: [] }
}
export const copyPolicy = (policy: KeywordPolicy): KeywordPolicy => JSON.parse(JSON.stringify(policy))
export const splitKeywords = (text: string, preserveWhitespace = false) => text.split(/\r?\n/).map(value => preserveWhitespace ? value : value.trim()).filter(Boolean)
export function deduplicateKeywords(words: string[], sensitive = true, preserveWhitespace = false): string[] {
  const seen = new Set<string>()
  return words.map(word => preserveWhitespace ? word : word.trim()).filter(word => {
    if (!word) return false
    const key = sensitive ? word : word.toLowerCase()
    if (seen.has(key)) return false
    seen.add(key); return true
  })
}

// RFC 4180 quoting, including escaped quotes, commas and multiline fields.
export function parseKeywordCSV(text: string): string[][] {
  const rows: string[][] = []
  let row: string[] = [], cell = '', quoted = false, closed = false
  text = text.replace(/^\uFEFF/, '')
  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    if (quoted) {
      if (c === '"') { if (text[i + 1] === '"') { cell += '"'; i++ } else { quoted = false; closed = true } }
      else cell += c
      continue
    }
    if (c === '"') { if (cell || closed) throw new KeywordImportError('importInvalidCSV'); quoted = true }
    else if (c === ',') { row.push(cell); cell = ''; closed = false }
    else if (c === '\n' || c === '\r') {
      if (c === '\r' && text[i + 1] === '\n') i++
      row.push(cell); if (row.some(value => value.trim())) rows.push(row)
      row = []; cell = ''; closed = false
    } else { if (closed && c.trim()) throw new KeywordImportError('importInvalidCSV'); cell += c }
  }
  if (quoted) throw new KeywordImportError('importInvalidCSV')
  row.push(cell); if (row.some(value => value.trim())) rows.push(row)
  return rows
}
function strings(value: unknown): string[] {
  if (!Array.isArray(value) || value.some(word => typeof word !== 'string')) throw new KeywordImportError('importInvalidJSON')
  return value
}
function record(value: unknown): Record<string, unknown> | null { return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null }
function normalizeRule(value: unknown): KeywordRule {
  const source = record(value)
  if (!source) throw new KeywordImportError('importInvalidJSON')
  const rule = createKeywordRule(typeof source.name === 'string' ? source.name : '')
  const stringFields = ['id', 'name', 'description', 'category', 'reply', 'injection'] as const
  for (const field of stringFields) if (source[field] !== undefined) { if (typeof source[field] !== 'string') throw new KeywordImportError('importInvalidJSON'); rule[field] = source[field] }
  for (const field of ['enabled', 'case_sensitive', 'stop_on_match'] as const) if (source[field] !== undefined) { if (typeof source[field] !== 'boolean') throw new KeywordImportError('importInvalidJSON'); rule[field] = source[field] }
  rule.keywords = strings(source.keywords ?? [])
  rule.exclude_keywords = strings(source.exclude_keywords ?? [])
  for (const field of ['models', 'exclude_models'] as const) rule[field] = strings(source[field] ?? [])
  for (const field of ['group_ids', 'user_ids', 'exclude_user_ids'] as const) {
    const ids = source[field] ?? []
    if (!Array.isArray(ids) || ids.some(id => !Number.isSafeInteger(id) || Number(id) < 1)) throw new KeywordImportError('importInvalidJSON')
    rule[field] = ids as number[]
  }
  if (source.priority !== undefined) { if (!Number.isInteger(source.priority)) throw new KeywordImportError('importInvalidJSON'); rule.priority = Number(source.priority) }
  const modes: KeywordMatchMode[] = ['exact', 'contains', 'word', 'prefix', 'suffix', 'regex']
  const actions: KeywordAction[] = ['reply', 'block', 'inject']
  if (source.match_mode !== undefined) { if (!modes.includes(source.match_mode as KeywordMatchMode)) throw new KeywordImportError('importInvalidJSON'); rule.match_mode = source.match_mode as KeywordMatchMode }
  if (source.action !== undefined) { if (!actions.includes(source.action as KeywordAction)) throw new KeywordImportError('importInvalidJSON'); rule.action = source.action as KeywordAction }
  if (source.keyword_logic !== undefined) { if (!['any', 'all'].includes(String(source.keyword_logic))) throw new KeywordImportError('importInvalidJSON'); rule.keyword_logic = source.keyword_logic as 'any' | 'all' }
  if (source.scan_scope !== undefined) { if (!['latest_user', 'all_user', 'context'].includes(String(source.scan_scope))) throw new KeywordImportError('importInvalidJSON'); rule.scan_scope = source.scan_scope as KeywordRule['scan_scope'] }
  if (source.injection_position !== undefined) { if (!['prepend', 'append'].includes(String(source.injection_position))) throw new KeywordImportError('importInvalidJSON'); rule.injection_position = source.injection_position as KeywordRule['injection_position'] }
  const protocols = strings(source.protocols ?? [])
  if (protocols.some(value => !['openai_chat_completions', 'openai_responses', 'anthropic_messages', 'gemini'].includes(value))) throw new KeywordImportError('importInvalidJSON')
  rule.protocols = protocols as KeywordProtocol[]
  return rule
}
const headers: Record<string, string> = { keyword: 'keyword', keywords: 'keyword', '关键词': 'keyword', '触发词': 'keyword', name: 'name', '名称': 'name', '规则名称': 'name', reply: 'reply', '回复': 'reply', injection: 'injection', '注入': 'injection', action: 'action', '动作': 'action', match_mode: 'match_mode', '匹配方式': 'match_mode', category: 'category', '分类': 'category' }
export interface KeywordImport { rules: KeywordRule[]; keywords: string[] }
export function parseKeywordImport(text: string, format: 'txt' | 'csv' | 'json', mode: 'keywords' | 'rules', preserveWhitespace = false): KeywordImport {
  if (new TextEncoder().encode(text).length > 4 * 1024 * 1024) throw new KeywordImportError('importTooLarge')
  text = text.replace(/^\uFEFF/, '')
  if (!text.trim()) throw new KeywordImportError('importEmpty')
  if (format === 'txt') return { rules: [], keywords: splitKeywords(text, preserveWhitespace) }
  if (format === 'json') {
    let json: unknown
    try { json = JSON.parse(text) } catch { throw new KeywordImportError('importInvalidJSON') }
    if (Array.isArray(json) && json.every(value => typeof value === 'string')) return { rules: [], keywords: json as string[] }
    const object = record(json)
    if (object?.version !== undefined && object.version !== 1) throw new KeywordImportError('importInvalidJSON')
    const values = Array.isArray(json) ? json : object?.rules
    if (!Array.isArray(values)) throw new KeywordImportError('importInvalidJSON')
    const rules = values.map(normalizeRule)
    return mode === 'rules' ? { rules, keywords: [] } : { rules: [], keywords: rules.flatMap(rule => rule.keywords) }
  }
  const rows = parseKeywordCSV(text)
  if (!rows.length) throw new KeywordImportError('importEmpty')
  const columns = rows[0].map(value => headers[value.trim().toLowerCase()] ?? '')
  const hasHeaders = columns.includes('keyword')
  if (mode === 'keywords') return { rules: [], keywords: (hasHeaders ? rows.slice(1) : rows).map(row => row[hasHeaders ? columns.indexOf('keyword') : 0] ?? '').filter(word => word.trim()) }
  if (!hasHeaders) throw new KeywordImportError('importCSVHeaders')
  const rules = rows.slice(1).map(row => {
    const item: Record<string, unknown> = {}
    columns.forEach((name, i) => { if (name) item[name] = row[i] ?? '' })
    const keyword = String(item.keyword ?? '')
    item.keywords = [keyword]
    item.name = item.name || [...keyword].slice(0, 40).join('')
    item.action = ({ '回复': 'reply', '直接回复': 'reply', '拦截': 'block', '拦截请求': 'block', '注入': 'inject', '注入提示词': 'inject' } as Record<string, string>)[String(item.action)] || item.action || (item.injection ? 'inject' : item.reply ? 'reply' : 'block')
    item.match_mode = ({ '完全一致': 'exact', '完整提问完全一致': 'exact', '包含': 'contains', '原文包含': 'contains', '完整词语': 'word', '前缀': 'prefix', '后缀': 'suffix', '正则': 'regex' } as Record<string, string>)[String(item.match_mode)] || item.match_mode
    if (!item.match_mode) delete item.match_mode
    return normalizeRule(item)
  })
  return { rules, keywords: [] }
}
export function validateKeywordPolicy(policy: KeywordPolicy): string | null {
  if (policy.rules.length > 500) return 'capacityError'
  if (policy.rules.reduce((total, rule) => total + rule.keywords.length + rule.exclude_keywords.length, 0) > 50000) return 'capacityError'
  for (const rule of policy.rules) {
    if (![...rule.group_ids, ...rule.user_ids, ...rule.exclude_user_ids].every(id => Number.isSafeInteger(id) && id > 0) || !Number.isInteger(rule.priority) || rule.priority < 0 || rule.priority > 100000) return 'scopeError'
    if (!rule.name.trim() || !rule.keywords.length || (rule.action === 'reply' && !rule.reply.trim()) || (rule.action === 'inject' && !rule.injection.trim())) return 'incompleteRule'
    if ([...(rule.description || '')].length > 2000) return 'capacityError'
    if (rule.keywords.length > 5000 || [...rule.keywords, ...rule.exclude_keywords].some(word => [...word].length > 512)) return 'capacityError'
  }
  return null
}
