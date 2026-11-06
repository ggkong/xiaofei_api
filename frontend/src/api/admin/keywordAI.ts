import { buildGatewayUrl } from '@/api/url'
import { KeywordAIError, keywordAIMessages, parseKeywordAI, type KeywordAIBrief } from '@/utils/keywordAI'

// Use the normal gateway: key permissions, IP restrictions, quota, model admission,
// audit, archive and billing apply just as for the administrator's other calls.
async function gatewayJSON(path: string, key: string, signal: AbortSignal, body?: unknown): Promise<Record<string, unknown>> {
  const response = await fetch(buildGatewayUrl(path), {
    method: body === undefined ? 'GET' : 'POST',
    headers: { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' },
    credentials: 'omit', redirect: 'error', signal,
    ...(body === undefined ? {} : { body: JSON.stringify(body) })
  })
  if (!response.ok) {
    if (response.body) await response.body.cancel()
    const code = response.status
    throw new KeywordAIError(code === 401 ? 'aiKeyUnavailable' : code === 402 || code === 429 ? 'aiQuotaError' : code === 403 ? 'aiPermissionError' : code === 404 ? 'aiModelError' : 'aiRequestError')
  }
  const reader = response.body?.getReader()
  if (!reader) throw new KeywordAIError('aiRequestError')
  const chunks: Uint8Array[] = []; let size = 0
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      size += value.length
      if (size > 256 * 1024) throw new KeywordAIError('aiInvalidOutput')
      chunks.push(value)
    }
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock() }
  const buffer = new Uint8Array(size); let offset = 0
  for (const chunk of chunks) { buffer.set(chunk, offset); offset += chunk.length }
  try {
    const data: unknown = JSON.parse(new TextDecoder().decode(buffer))
    if (!data || typeof data !== 'object' || Array.isArray(data)) throw new Error()
    return data as Record<string, unknown>
  } catch { throw new KeywordAIError('aiInvalidOutput') }
}

export async function keywordAIModels(key: string, signal: AbortSignal): Promise<string[]> {
  const document = await gatewayJSON('/v1/models', key, signal)
  const items = document.data
  if (!Array.isArray(items)) throw new KeywordAIError('aiModelError')
  return [...new Set(items.flatMap(item => item && typeof item === 'object' && typeof item.id === 'string' ? [item.id as string] : []))].sort()
}

export async function generateKeywordAI(key: string, model: string, brief: KeywordAIBrief, signal: AbortSignal) {
  const document = await gatewayJSON('/v1/chat/completions', key, signal, { model, messages: keywordAIMessages(brief), max_tokens: 4096, stream: false })
  const choices = document.choices
  if (!Array.isArray(choices) || !choices[0] || typeof choices[0] !== 'object') throw new KeywordAIError('aiInvalidOutput')
  if (choices[0].finish_reason === 'length') throw new KeywordAIError('aiTruncated')
  const content: unknown = choices[0].message?.content
  if (typeof content !== 'string') throw new KeywordAIError('aiInvalidOutput')
  return { ...parseKeywordAI(content, brief), model: typeof document.model === 'string' ? document.model : model }
}
