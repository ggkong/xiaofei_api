import { describe, expect, it } from 'vitest'
import { archiveDateRange, archiveRequestMessages, archiveResponseMessages } from '../conversationArchive'

describe('conversation archive display', () => {
  it('preserves request context order and supports Chat, Responses and Anthropic text', () => {
    expect(archiveRequestMessages({ messages: [{ role: 'system', content: 'policy' }, { role: 'user', content: [{ type: 'text', text: 'hello' }, { type: 'image_url', image_url: '[media omitted]' }] }, { role: 'tool', content: 'result' }] })).toEqual([
      { role: 'system', text: 'policy' }, { role: 'user', text: 'hello' }, { role: 'tool', text: 'result' }
    ])
    expect(archiveRequestMessages({ instructions: 'policy', input: 'question' })).toEqual([{ role: 'system', text: 'policy' }, { role: 'user', text: 'question' }])
    expect(archiveRequestMessages({ system: [{ type: 'text', text: 'policy' }], messages: [{ role: 'user', content: 'question' }] })).toHaveLength(2)
    expect(archiveRequestMessages({ omitted: true })).toEqual([])
  })

  it('keeps multiple streamed completion choices separate', () => {
    const events = [
      { choices: [{ index: 1, delta: { content: 'second' } }, { index: 0, delta: { content: 'first' } }] },
      { choices: [{ index: 0, delta: { content: ' reply' } }] }
    ]
    expect(archiveResponseMessages({ events, text: 'incorrect concatenation' })).toEqual([{ role: 'assistant', text: 'first reply' }, { role: 'assistant', text: 'second' }])
  })

  it('renders native response text without executing or changing HTML', () => {
    const text = '<script>alert(1)</script>'
    expect(archiveResponseMessages({ output: [{ type: 'message', role: 'assistant', content: [{ type: 'output_text', text }] }] })).toEqual([{ role: 'assistant', text }])
    expect(archiveResponseMessages({ choices: [{ message: { role: 'assistant', content: text } }] })).toEqual([{ role: 'assistant', text }])
    expect(archiveResponseMessages({ content: [{ type: 'text', text }] })).toEqual([{ role: 'assistant', text }])
    expect(archiveResponseMessages({ text, events: [] })).toEqual([{ role: 'assistant', text }])
    expect(archiveResponseMessages({ output: [{ type: 'function_call', arguments: '{}' }] })).toEqual([])
  })

  it('validates time ranges and sends explicit UTC timestamps', () => {
    expect(archiveDateRange('2026-11-04T00:00:00Z', '2026-11-04T01:00:00Z')).toEqual({ created_from: '2026-11-04T00:00:00.000Z', created_until: '2026-11-04T01:00:00.000Z' })
    expect(archiveDateRange('2026-11-04T00:00:00+08:00', '')).toEqual({ created_from: '2026-11-03T16:00:00.000Z' })
    expect(archiveDateRange('', '')).toEqual({})
    expect(archiveDateRange('invalid', '')).toBeNull()
    expect(archiveDateRange('2026-11-04T01:00:00Z', '2026-11-04T00:00:00Z')).toBeNull()
    expect(archiveDateRange('2026-11-04T00:00:00Z', '2026-11-04T00:00:00Z')).toBeNull()
  })
})
