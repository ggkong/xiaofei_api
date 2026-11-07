import { archiveContextMessages, archiveResponseContext } from './context.js'
const $ = id => document.getElementById(id)
let items = [], current = null, currentRaw = '', tab = 'exchange', before = 0, next = 0, cursors = [], query = '', group = '', detailVersion = 0, listVersion = 0
const date = value => new Date(value).toLocaleString('zh-CN', { hour12: false })
const name = item => item.user_name || item.user_email || `成员 #${item.user_id ?? '—'}`
const model = item => item.auto_reply ? '关键词自动回复 · 未调用模型' : item.upstream_model || item.response_model || item.model || '模型未记录'
function node(tag, cls, text) { const el = document.createElement(tag); if (cls) el.className = cls; if (text !== undefined) el.textContent = text; return el }
function error(text = '') { $('error').textContent = text; $('error').hidden = !text }
async function get(path) { const r = await fetch(path, { cache: 'no-store' }); if (!r.ok) throw new Error('无法读取本地记录。请检查同步状态和归档磁盘。'); return r }
async function load() {
  const version = ++listVersion
  try {
    const params = new URLSearchParams({ before: String(before), q: query, group })
    const page = await (await get(`/api/records?${params}`)).json()
    if (version !== listVersion) return
    items = page.items; next = page.next_id; $('count').textContent = page.count.toLocaleString(); $('records').replaceChildren()
    for (const item of items) {
      const button = node('button', `record${current?.id === item.id ? ' selected' : ''}`)
      const top = node('div', 'record-top'); top.append(node('span', 'record-name', name(item)), node('span', 'record-time', new Date(item.created_at).toLocaleDateString('zh-CN')))
      button.append(top, node('p', 'record-prompt', item.prompt_preview || '工具调用 / 非文本内容'), node('span', 'record-model', model(item)))
      button.addEventListener('click', () => open(item.id)); $('records').append(button)
    }
    if (!items.length) $('records').append(node('p', 'empty', '还没有匹配的本地记录'))
    $('prev').disabled = !cursors.length; $('next').disabled = !next; $('reset').hidden = !group
  } catch (e) { if (version === listVersion) error(e.message) }
}
async function open(id) {
  const version = ++detailVersion
  try {
    const raw = await (await get(`/api/records/${id}`)).text(); if (version !== detailVersion) return
    currentRaw = raw; current = JSON.parse(raw); tab = 'exchange'
    $('empty').hidden = true; $('detail').hidden = false; $('member').textContent = name(current); $('model').textContent = model(current); $('detail-time').textContent = `${date(current.created_at)} · #${current.id} · ${current.endpoint}`
    $('download').href = `/api/records/${id}?download=1`; $('limit').hidden = !current.truncated; $('related').disabled = !current.conversation_key
    render(); error(); await load()
    if (innerWidth < 650) $('detail').scrollIntoView({ behavior: 'smooth', block: 'start' })
  } catch (e) { if (version === detailVersion) error(e.message) }
}
function render() {
  document.querySelectorAll('[data-tab]').forEach(b => { b.classList.toggle('active', b.dataset.tab === tab); b.setAttribute('aria-pressed', String(b.dataset.tab === tab)) })
  $('messages').hidden = !['exchange', 'context'].includes(tab); $('models').hidden = tab !== 'models'; $('raw').hidden = tab !== 'raw'
  if (tab === 'raw') { $('raw').textContent = currentRaw; return }
  if (tab === 'models') {
    $('models').replaceChildren()
    for (const [label, value] of [['请求模型', current.model], ['实际转发模型', current.auto_reply ? '未调用模型' : current.upstream_model], ['服务商声明模型', current.auto_reply ? '未调用模型' : current.response_model], ['客户端响应模型', current.wire_model], ['请求标识', current.request_id], ['会话标识', current.session_id || current.conversation_key]]) {
      const card = node('div', 'model-card'); card.append(node('p', '', label), node('strong', '', value || '未记录')); $('models').append(card)
    }
    return
  }
  const context = archiveContextMessages(current.request_body)
  const lastUser = context.filter(m => m.role === 'user').slice(-1)
  const messages = [...(tab === 'context' ? context : lastUser), ...archiveResponseContext(current.response_body)]
  $('messages').replaceChildren()
  if (!messages.length) { $('messages').append(node('p', '', '没有可展示的文本，请在原始记录中查看结构化内容。')); return }
  for (const message of messages) {
    const bubble = node('div', `bubble ${['assistant', 'system'].includes(message.role) ? message.role : ''}`)
    bubble.append(node('div', 'role', `${message.role}${message.name ? ' · ' + message.name : ''}`))
    if (message.kind === 'tool') { const details = node('details'); details.append(node('summary', '', '工具调用 / 结果'), node('pre', '', message.text)); bubble.append(details) }
    else bubble.append(node('p', '', message.text || `${message.mediaCount || 0} 项媒体内容（查看原始记录）`))
    $('messages').append(bubble)
  }
}
async function status() {
  try { const data = await (await get('/api/status')).json(); $('directory').textContent = data.directory; const s = data.sync; $('sync-status').textContent = s.running ? `正在同步 · 已检查 ${s.progress.checked} 条 · 新增 ${s.progress.downloaded} 条` : s.error ? `上次同步未完成：${s.error}（可重新同步，已保存内容会保留）` : s.success_at ? `最近成功同步 ${date(s.success_at)} · 本地正文已校验` : '等待首次同步：先在服务器管理页完成配对'; $('sync').disabled = s.running } catch { /* status retries independently */ }
}
$('search-form').addEventListener('submit', e => { e.preventDefault(); query = $('search').value.trim(); before = 0; cursors = []; void load() })
$('next').addEventListener('click', () => { if (!next) return; cursors.push(before); before = next; void load() })
$('prev').addEventListener('click', () => { if (!cursors.length) return; before = cursors.pop(); void load() })
$('related').addEventListener('click', () => { group = current.conversation_key || ''; before = 0; cursors = []; void load() })
$('reset').addEventListener('click', () => { group = ''; before = 0; cursors = []; void load() })
document.querySelectorAll('[data-tab]').forEach(b => b.addEventListener('click', () => { tab = b.dataset.tab; render() }))
$('sync').addEventListener('click', async () => { $('sync').disabled = true; try { const response = await fetch('/api/sync', { method: 'POST' }); if (!response.ok) throw new Error('无法启动同步'); setTimeout(() => { void status(); void load() }, 2000) } catch (e) { error(e.message); $('sync').disabled = false } })
void load(); void status(); setInterval(() => { void status() }, 5000)
