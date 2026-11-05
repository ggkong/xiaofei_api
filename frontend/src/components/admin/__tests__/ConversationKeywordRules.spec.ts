import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ConversationKeywordRules from '../ConversationKeywordRules.vue'

const { get, put } = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, put } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('conversation keyword rules', () => {
  beforeEach(() => { get.mockReset(); put.mockReset(); put.mockResolvedValue({ data: {} }) })

  it('loads existing words and enables local blocking for all members and models', async () => {
    get.mockResolvedValue({ data: { blocked_keywords: ['原有词'] } })
    const wrapper = mount(ConversationKeywordRules)
    await flushPromises()
    expect(wrapper.get('textarea').element.value).toBe('原有词')
    await wrapper.get('#keyword-master').setValue(true)
    await wrapper.get('textarea').setValue(' 禁止词\nSECRET\n\n')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(put).toHaveBeenNthCalledWith(1, '/admin/risk-control/config', expect.objectContaining({
      enabled: true, mode: 'pre_block', keyword_blocking_mode: 'keyword_only',
      blocked_keywords: ['禁止词', 'SECRET'], keyword_match_mode: 'exact', keyword_auto_reply_enabled: false, all_groups: true, model_filter: { type: 'all', models: [] }, auto_ban_enabled: false
    }))
    expect(put).toHaveBeenNthCalledWith(2, '/admin/settings', { risk_control_enabled: true })
    expect(put).toHaveBeenNthCalledWith(3, '/admin/risk-control/config', { enabled: true })
    expect(wrapper.get('[role="status"]').text()).toBe('conversationArchive.rulesSaved')
  })

  it('clearing the words disables blocking', async () => {
    get.mockResolvedValue({ data: { blocked_keywords: ['old'] } })
    const wrapper = mount(ConversationKeywordRules)
    await flushPromises()
    await wrapper.get('textarea').setValue('')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(1)
    expect(put).toHaveBeenCalledWith('/admin/risk-control/config', expect.objectContaining({ enabled: false, blocked_keywords: [] }))
  })

  it('does not overwrite rules when the existing configuration cannot be loaded', async () => {
    get.mockRejectedValue(new Error('unavailable'))
    const wrapper = mount(ConversationKeywordRules)
    await flushPromises()
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit')
    expect(put).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').exists()).toBe(true)
  })

  it('pauses rules while preserving their words, matching mode and reply', async () => {
    get.mockResolvedValue({ data: { enabled: false, blocked_keywords: ['Exact WORD'], keyword_match_mode: 'literal', keyword_auto_reply_enabled: true, keyword_reply: '保留的回复' } })
    const wrapper = mount(ConversationKeywordRules)
    await flushPromises()
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(1)
    expect(put).toHaveBeenCalledWith('/admin/risk-control/config', expect.objectContaining({ enabled: false, blocked_keywords: ['Exact WORD'], keyword_match_mode: 'literal', keyword_reply: '保留的回复' }))
  })

  it('requires reply text and saves automatic replies only with precise rules', async () => {
    get.mockResolvedValue({ data: { enabled: true, blocked_keywords: ['EXACT'], keyword_match_mode: 'contains' } })
    const wrapper = mount(ConversationKeywordRules)
    await flushPromises()
    await wrapper.findAll('input[type="checkbox"]')[1].setValue(true)
    expect(wrapper.get('select').element.value).toBe('exact')
    await wrapper.get('form').trigger('submit')
    expect(put).not.toHaveBeenCalled()
    expect(wrapper.get('[role="alert"]').text()).toBe('conversationArchive.replyRequired')
    await wrapper.findAll('textarea')[1].setValue('指定回复')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(put).toHaveBeenNthCalledWith(1, '/admin/risk-control/config', expect.objectContaining({ enabled: true, keyword_auto_reply_enabled: true, keyword_match_mode: 'exact', keyword_reply: '指定回复' }))
  })
})
