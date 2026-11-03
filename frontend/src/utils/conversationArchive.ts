export interface ArchiveMessage {
  role: string
  text: string
}

function object(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : null
}

function textContent(value: unknown, depth = 0): string {
  if (depth > 12) return ''
  if (typeof value === 'string') return value
  if (Array.isArray(value)) return value.map(part => textContent(part, depth + 1)).filter(Boolean).join('\n')
  const part = object(value)
  if (!part) return ''
  if (typeof part.text === 'string') return part.text
  const text = object(part.text)
  return typeof text?.value === 'string' ? text.value : ''
}

function messages(value: unknown, fallbackRole: string): ArchiveMessage[] {
  if (!Array.isArray(value)) return []
  return value.flatMap(item => {
    const message = object(item)
    if (!message) return []
    const text = textContent(message.content)
    return text ? [{ role: typeof message.role === 'string' ? message.role : fallbackRole, text }] : []
  })
}

// Request bodies are context snapshots. Do not merge these into a conversation
// timeline: the same previous messages may appear in every subsequent request.
export function archiveRequestMessages(value: unknown): ArchiveMessage[] {
  const body = object(value)
  if (!body) return []
  const result: ArchiveMessage[] = []
  for (const instructions of [body.instructions, body.system]) {
    const text = textContent(instructions)
    if (text) result.push({ role: 'system', text })
  }
  if (Array.isArray(body.messages)) return [...result, ...messages(body.messages, 'user')]
  if (typeof body.input === 'string') return [...result, { role: 'user', text: body.input }]
  return [...result, ...messages(body.input, 'user')]
}

export function archiveResponseMessages(value: unknown): ArchiveMessage[] {
  const body = object(value)
  if (!body) return []
  // Keep distinct Chat Completions choices separate instead of presenting their
  // concatenation as one assistant response.
  if (Array.isArray(body.events)) {
    const choices = new Map<number, string>()
    for (const raw of body.events) {
      const event = object(raw)
      if (!Array.isArray(event?.choices)) continue
      for (const rawChoice of event.choices) {
        const choice = object(rawChoice)
        const delta = object(choice?.delta)
        if (typeof delta?.content !== 'string') continue
        const index = typeof choice?.index === 'number' ? choice.index : 0
        choices.set(index, (choices.get(index) ?? '') + delta.content)
      }
    }
    if (choices.size) return [...choices.entries()].sort(([a], [b]) => a - b).filter(([, text]) => text).map(([, text]) => ({ role: 'assistant', text }))
  }
  if (Array.isArray(body.choices)) {
    return body.choices.flatMap(raw => {
      const choice = object(raw)
      return messages(choice?.message ? [choice.message] : [], 'assistant')
    })
  }
  if (Array.isArray(body.output)) return messages(body.output, 'assistant')
  const text = typeof body.text === 'string' ? body.text : textContent(body.content)
  return text ? [{ role: 'assistant', text }] : []
}

export function archiveDateRange(from: string, until: string): Record<string, string> | null {
  const result: Record<string, string> = {}
  const start = from ? new Date(from) : null
  const end = until ? new Date(until) : null
  if ((start && Number.isNaN(start.getTime())) || (end && Number.isNaN(end.getTime())) || (start && end && start >= end)) return null
  if (start) result.created_from = start.toISOString()
  if (end) result.created_until = end.toISOString()
  return result
}
