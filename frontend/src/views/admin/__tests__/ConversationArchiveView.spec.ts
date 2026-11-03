import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ConversationArchiveView from '../ConversationArchiveView.vue'

const { get, remove } = vi.hoisted(() => ({ get: vi.fn(), remove: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, delete: remove } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))

describe('administrator conversation archive', () => {
  it('renders captured model output as inert text and shows disabled capture', async () => {
    get.mockReset()
    const record = { id: 1, user_id: 42, api_key_id: 7, session_id: '', model: 'test', created_at: '2026-11-03T00:00:00Z', status: 'complete', truncated: false }
    get.mockResolvedValueOnce({ data: { items: [record], next_before_id: 0, capture_enabled: false } })
    get.mockResolvedValueOnce({ data: { ...record, request_body: { messages: [] }, response_body: { content: '<img src=x onerror=alert(1)>' } } })
    const wrapper = mount(ConversationArchiveView)
    await flushPromises()
    expect(wrapper.text()).toContain('conversationArchive.disabled')
    const button = wrapper.findAll('button').find(item => item.text() === 'conversationArchive.view')
    await button!.trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('<img src=x onerror=alert(1)>')
    expect(wrapper.find('img').exists()).toBe(false)
    expect(get).toHaveBeenLastCalledWith('/admin/conversation-archives/1')
  })

  it('clears records when the backend rejects archive access', async () => {
    get.mockReset()
    get.mockRejectedValueOnce(new Error('403'))
    const wrapper = mount(ConversationArchiveView)
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('conversationArchive.loadError')
    expect(wrapper.findAll('tbody tr')).toHaveLength(1)
    expect(wrapper.text()).toContain('conversationArchive.empty')
  })

  it('keeps pagination on the applied filters until the administrator searches', async () => {
    get.mockReset()
    get.mockResolvedValue({ data: { items: [], next_before_id: 100, capture_enabled: true } })
    const wrapper = mount(ConversationArchiveView)
    await flushPromises()
    await wrapper.findAll('input[maxlength="255"]')[0].setValue('draft-model')
    await wrapper.findAll('button').find(button => button.text() === 'conversationArchive.next')!.trigger('click')
    await flushPromises()
    expect(get).toHaveBeenLastCalledWith('/admin/conversation-archives', { params: { limit: 30, before_id: 100 } })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(get).toHaveBeenLastCalledWith('/admin/conversation-archives', { params: { limit: 30, model: 'draft-model' } })
  })

  it('rejects reversed date ranges without issuing a query', async () => {
    get.mockReset()
    get.mockResolvedValueOnce({ data: { items: [], next_before_id: 0, capture_enabled: true } })
    const wrapper = mount(ConversationArchiveView)
    await flushPromises()
    const dates = wrapper.findAll('input[type="datetime-local"]')
    await dates[0].setValue('2026-11-05T00:00')
    await dates[1].setValue('2026-11-04T00:00')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(get).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[role="alert"]').text()).toBe('conversationArchive.invalidRange')
  })

  it('does not restore an old detail response after a new search', async () => {
    get.mockReset()
    const record = { id: 1, user_id: 42, api_key_id: 7, session_id: '', model: 'test', created_at: '2026-11-03T00:00:00Z', status: 'complete', truncated: false }
    let resolveDetail!: (value: unknown) => void
    get.mockResolvedValueOnce({ data: { items: [record], next_before_id: 0, capture_enabled: true } })
    get.mockReturnValueOnce(new Promise(resolve => { resolveDetail = resolve }))
    get.mockResolvedValueOnce({ data: { items: [record], next_before_id: 0, capture_enabled: true } })
    const wrapper = mount(ConversationArchiveView)
    await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'conversationArchive.view')!.trigger('click')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    resolveDetail({ data: { ...record, response_body: { text: 'stale-private-content' } } })
    await flushPromises()
    expect(wrapper.text()).not.toContain('stale-private-content')
    expect(wrapper.find('section[aria-live]').exists()).toBe(false)
    expect(wrapper.findAll('button').find(button => button.text() === 'conversationArchive.view')!.attributes('disabled')).toBeUndefined()
  })
})
