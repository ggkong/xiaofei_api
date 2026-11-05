import { apiClient } from '../client'

export type KeywordMatchMode = 'exact' | 'contains' | 'word' | 'prefix' | 'suffix' | 'regex'
export type KeywordAction = 'reply' | 'block' | 'inject'
export type KeywordProtocol = 'openai_chat_completions' | 'openai_responses' | 'anthropic_messages' | 'gemini'
export interface KeywordRule {
  id: string
  name: string
  category: string
  enabled: boolean
  priority: number
  keywords: string[]
  exclude_keywords: string[]
  match_mode: KeywordMatchMode
  keyword_logic: 'any' | 'all'
  case_sensitive: boolean
  scan_scope: 'latest_user' | 'all_user' | 'context'
  action: KeywordAction
  reply: string
  injection: string
  injection_position: 'prepend' | 'append'
  stop_on_match: boolean
  group_ids: number[]
  user_ids: number[]
  exclude_user_ids: number[]
  models: string[]
  exclude_models: string[]
  protocols: KeywordProtocol[]
}
export interface KeywordPolicy { version: number; revision: string; enabled: boolean; rules: KeywordRule[]; legacy: boolean }
export interface KeywordHit { rule_id: string; rule_name: string; action: KeywordAction; keywords: string[]; injection?: string; injection_position?: string }
export interface KeywordTestResult { enabled: boolean; action: 'forward' | KeywordAction; reply: string; hits: KeywordHit[]; effective_request: unknown }
export interface KeywordTest { policy: KeywordPolicy; text: string; protocol: KeywordProtocol; model: string; user_id: number; group_id: number }

export async function getKeywordRules(): Promise<KeywordPolicy> { const { data } = await apiClient.get<KeywordPolicy>('/admin/keyword-rules'); return data }
export async function saveKeywordRules(policy: KeywordPolicy): Promise<KeywordPolicy> { const { data } = await apiClient.put<KeywordPolicy>('/admin/keyword-rules', policy); return data }
export async function testKeywordRules(payload: KeywordTest): Promise<KeywordTestResult> { const { data } = await apiClient.post<KeywordTestResult>('/admin/keyword-rules/test', payload); return data }
