// Generated from frontend/src/utils/conversationArchive.ts; preserve the shared protocol renderer.
function contextMessage(value, fallbackRole) {
    const item = object(value);
    if (!item)
        return [];
    const role = typeof item.role === 'string' ? item.role : fallbackRole;
    const result = [];
    const content = item.content;
    const parts = Array.isArray(content) ? content : [content];
    const text = textContent(content);
    const mediaCount = parts.filter(part => {
        const value = object(part);
        return value && ['image', 'image_url', 'input_image', 'input_audio', 'audio', 'file', 'input_file'].includes(String(value.type));
    }).length;
    if (text || mediaCount)
        result.push({ role, text, mediaCount });
    const tools = [...parts.filter(part => ['tool_use', 'tool_result'].includes(String(object(part)?.type))), ...(Array.isArray(item.tool_calls) ? item.tool_calls : [])];
    if (item.type === 'function_call' || item.type === 'function_call_output')
        tools.push(item);
    if (item.function_call)
        tools.push(item.function_call);
    if (role === 'tool' && !result.length && item.output !== undefined)
        tools.push(item);
    for (const tool of tools) {
        const data = object(tool);
        const fn = object(data?.function);
        result.push({ role: data?.type === 'tool_result' || data?.type === 'function_call_output' ? 'tool' : role, kind: 'tool', name: String(fn?.name ?? data?.name ?? data?.tool_use_id ?? data?.call_id ?? ''), text: JSON.stringify(tool, null, 2) });
    }
    return result;
}
export function archiveContextMessages(value) {
    const body = object(value);
    if (!body)
        return [];
    const result = [];
    for (const value of [body.system, body.instructions]) {
        const text = textContent(value);
        if (text)
            result.push({ role: 'system', text });
    }
    const input = body.messages ?? body.input;
    if (typeof input === 'string')
        result.push({ role: 'user', text: input });
    if (Array.isArray(input))
        for (const item of input)
            result.push(...contextMessage(item, 'user'));
    return result;
}
export function archiveResponseContext(value) {
    const body = object(value);
    if (!body)
        return [];
    if (Array.isArray(body.events)) {
        // A terminal structured item contains complete tool arguments; selecting it
        // avoids duplicating the same response's deltas and completed output.
        for (const raw of [...body.events].reverse()) {
            const event = object(raw);
            if (['response.completed', 'response.done'].includes(String(event?.type)) && object(event?.response)) {
                const terminal = archiveResponseContext(event.response);
                if (terminal.length)
                    return terminal;
            }
        }
        const text = archiveResponseMessages(body);
        const tools = [];
        const chatTools = new Map();
        for (const raw of body.events) {
            const event = object(raw);
            if (event?.type === 'response.output_item.done')
                tools.push(...contextMessage(event.item, 'assistant').filter(item => item.kind === 'tool'));
            if (event?.type === 'content_block_start') {
                const block = object(event.content_block);
                if (block?.type === 'tool_use')
                    tools.push({ role: 'assistant', kind: 'tool', name: String(block.name ?? ''), text: JSON.stringify(block, null, 2) });
            }
            if (event?.type === 'content_block_delta' && typeof object(event.delta)?.partial_json === 'string' && tools.length)
                tools[tools.length - 1].text += String(object(event.delta).partial_json);
            if (!Array.isArray(event?.choices))
                continue;
            for (const rawChoice of event.choices) {
                const choice = object(rawChoice);
                const delta = object(choice?.delta);
                if (!Array.isArray(delta?.tool_calls))
                    continue;
                for (const rawTool of delta.tool_calls) {
                    const tool = object(rawTool);
                    const fn = object(tool?.function);
                    const key = `${choice?.index ?? 0}/${tool?.index ?? 0}`;
                    const existing = chatTools.get(key) ?? { name: '', arguments: '' };
                    if (typeof fn?.name === 'string')
                        existing.name = fn.name;
                    if (typeof fn?.arguments === 'string')
                        existing.arguments += fn.arguments;
                    chatTools.set(key, existing);
                }
            }
        }
        for (const tool of chatTools.values())
            tools.push({ role: 'assistant', kind: 'tool', name: tool.name, text: tool.arguments });
        return [...text, ...tools];
    }
    if (Array.isArray(body.choices))
        return body.choices.flatMap(raw => contextMessage(object(raw)?.message, 'assistant'));
    if (Array.isArray(body.output))
        return body.output.flatMap(raw => contextMessage(raw, 'assistant'));
    if (Array.isArray(body.content))
        return contextMessage(body, 'assistant');
    if (Array.isArray(body.candidates))
        return body.candidates.flatMap(raw => {
            const parts = object(object(raw)?.content)?.parts;
            return Array.isArray(parts) ? parts.flatMap(part => {
                const value = object(part);
                if (typeof value?.text === 'string')
                    return [{ role: 'assistant', text: value.text }];
                return value?.functionCall ? [{ role: 'assistant', kind: 'tool', text: JSON.stringify(value.functionCall, null, 2) }] : [];
            }) : [];
        });
    return archiveResponseMessages(value);
}
function object(value) {
    return value !== null && typeof value === 'object' && !Array.isArray(value) ? value : null;
}
function textContent(value, depth = 0) {
    if (depth > 12)
        return '';
    if (typeof value === 'string')
        return value;
    if (Array.isArray(value))
        return value.map(part => textContent(part, depth + 1)).filter(Boolean).join('\n');
    const part = object(value);
    if (!part)
        return '';
    if (typeof part.text === 'string')
        return part.text;
    const text = object(part.text);
    return typeof text?.value === 'string' ? text.value : '';
}
function messages(value, fallbackRole) {
    if (!Array.isArray(value))
        return [];
    return value.flatMap(item => {
        const message = object(item);
        if (!message)
            return [];
        const text = textContent(message.content);
        return text ? [{ role: typeof message.role === 'string' ? message.role : fallbackRole, text }] : [];
    });
}
// Request bodies are context snapshots. Do not merge these into a conversation
// timeline: the same previous messages may appear in every subsequent request.
export function archiveRequestMessages(value) {
    const body = object(value);
    if (!body)
        return [];
    const result = [];
    for (const instructions of [body.instructions, body.system]) {
        const text = textContent(instructions);
        if (text)
            result.push({ role: 'system', text });
    }
    if (Array.isArray(body.messages))
        return [...result, ...messages(body.messages, 'user')];
    if (typeof body.input === 'string')
        return [...result, { role: 'user', text: body.input }];
    return [...result, ...messages(body.input, 'user')];
}
export function archiveResponseMessages(value) {
    const body = object(value);
    if (!body)
        return [];
    // Keep distinct Chat Completions choices separate instead of presenting their
    // concatenation as one assistant response.
    if (Array.isArray(body.events)) {
        const choices = new Map();
        for (const raw of body.events) {
            const event = object(raw);
            if (!Array.isArray(event?.choices))
                continue;
            for (const rawChoice of event.choices) {
                const choice = object(rawChoice);
                const delta = object(choice?.delta);
                if (typeof delta?.content !== 'string')
                    continue;
                const index = typeof choice?.index === 'number' ? choice.index : 0;
                choices.set(index, (choices.get(index) ?? '') + delta.content);
            }
        }
        if (choices.size)
            return [...choices.entries()].sort(([a], [b]) => a - b).filter(([, text]) => text).map(([, text]) => ({ role: 'assistant', text }));
    }
    if (Array.isArray(body.choices)) {
        return body.choices.flatMap(raw => {
            const choice = object(raw);
            return messages(choice?.message ? [choice.message] : [], 'assistant');
        });
    }
    if (Array.isArray(body.output))
        return messages(body.output, 'assistant');
    const text = typeof body.text === 'string' ? body.text : textContent(body.content);
    return text ? [{ role: 'assistant', text }] : [];
}
export function archiveDateRange(from, until) {
    const result = {};
    const start = from ? new Date(from) : null;
    const end = until ? new Date(until) : null;
    if ((start && Number.isNaN(start.getTime())) || (end && Number.isNaN(end.getTime())) || (start && end && start >= end))
        return null;
    if (start)
        result.created_from = start.toISOString();
    if (end)
        result.created_until = end.toISOString();
    return result;
}
