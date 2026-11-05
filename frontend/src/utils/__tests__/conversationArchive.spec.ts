import { describe, expect, it } from 'vitest'
import { archiveDateRange, archiveRequestMessages, archiveResponseMessages, archiveContextMessages, archiveResponseContext } from '../conversationArchive'

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

  it('retains system, tool and media context in order', () => {
    const context = archiveContextMessages({ instructions: 'policy', input: [
      { role: 'user', content: [{ type: 'input_text', text: 'question' }, { type: 'input_image', image_url: '[media omitted]' }] },
      { type: 'function_call', name: 'read_file', arguments: '{"path":"app.ts"}', call_id: 'call1' },
      { type: 'function_call_output', call_id: 'call1', output: 'file contents' }
    ] })
    expect(context.map(item => item.role)).toEqual(['system', 'user', 'user', 'tool'])
    expect(context[1].mediaCount).toBe(1)
    expect(context[2].kind).toBe('tool')
    expect(context[3].text).toContain('file contents')
  })

  it('uses terminal output without duplicating streaming deltas', () => {
    const result = archiveResponseContext({ text: 'delta text', events: [
      { type: 'response.output_text.delta', delta: 'delta text' },
      { type: 'response.completed', response: { output: [{ type: 'message', role: 'assistant', content: [{ type: 'output_text', text: 'complete text' }] }, { type: 'function_call', name: 'search', arguments: '{"q":"test"}' }] } }
    ] })
    expect(result).toHaveLength(2)
    expect(result[0].text).toBe('complete text')
    expect(result[1].kind).toBe('tool')
  })

  it('reassembles streamed Chat tool arguments', () => {
    const result = archiveResponseContext({ events: [
      { choices: [{ index: 0, delta: { tool_calls: [{ index: 0, function: { name: 'lookup', arguments: '{"q":' } }] } }] },
      { choices: [{ index: 0, delta: { tool_calls: [{ index: 0, function: { arguments: '"hello"}' } }] } }] }
    ] })
    expect(result).toEqual([{ role: 'assistant', kind: 'tool', name: 'lookup', text: '{"q":"hello"}' }])
  })
})
