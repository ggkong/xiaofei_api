import { afterEach, describe, expect, it, vi } from 'vitest'
import { generateKeywordAI, keywordAIModels } from '../admin/keywordAI'

const brief = { goal: 'Help', content: '', action: 'reply' as const, matchMode: 'exact' as const, count: 1, language: 'en' }
afterEach(() => vi.unstubAllGlobals())
describe('keyword generation through the normal gateway', () => {
  it('uses the selected key on the fixed gateway and keeps it out of the model prompt', async () => {
    const output = { summary: 'Explanation', rules: [{ name: 'Help', keywords: ['Help'], reason: 'Specific full question.', reply: 'Instructions', examples: [{ text: 'Help', should_match: true, explanation: 'Exact' }, { text: 'Help!', should_match: false, explanation: 'Punctuation differs' }] }] }
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ model: 'provider-model', choices: [{ finish_reason: 'stop', message: { content: JSON.stringify(output) } }] })))
    vi.stubGlobal('fetch', fetch)
    const signal = new AbortController().signal
    const result = await generateKeywordAI('test-key-only', 'alias', brief, signal)
    expect(result.model).toBe('provider-model')
    expect(fetch).toHaveBeenCalledWith(expect.stringMatching(/\/v1\/chat\/completions$/), expect.objectContaining({ signal, credentials: 'omit', redirect: 'error', headers: expect.objectContaining({ Authorization: 'Bearer test-key-only' }) }))
    const body = JSON.parse(fetch.mock.calls[0][1].body)
    expect(body).toMatchObject({ model: 'alias', stream: false, max_tokens: 4096 })
    expect(JSON.stringify(body)).not.toContain('test-key-only')
  })
  it('handles quota failures and incomplete outputs without accepting rules', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(new Response('private provider error', { status: 429 })).mockResolvedValueOnce(new Response(JSON.stringify({ choices: [{ finish_reason: 'length', message: { content: '{}' } }] })))
    vi.stubGlobal('fetch', fetch)
    await expect(generateKeywordAI('key', 'alias', brief, new AbortController().signal)).rejects.toThrow('aiQuotaError')
    await expect(generateKeywordAI('key', 'alias', brief, new AbortController().signal)).rejects.toThrow('aiTruncated')
  })
  it('bounds response size and returns only usable model identifiers', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify({ data: [{ id: 'alias' }, { id: 'alias' }, {}, { id: 123 }] }))).mockResolvedValueOnce(new Response('x'.repeat(256 * 1024 + 1)))
    vi.stubGlobal('fetch', fetch)
    expect(await keywordAIModels('key', new AbortController().signal)).toEqual(['alias'])
    await expect(generateKeywordAI('key', 'alias', brief, new AbortController().signal)).rejects.toThrow('aiInvalidOutput')
  })
})
