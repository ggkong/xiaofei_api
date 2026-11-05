import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import KeywordRulesView from '../KeywordRulesView.vue'
import { createKeywordRule } from '@/utils/keywordRules'

const { get, put, post } = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn(), post: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, put, post } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))
const original = { ...createKeywordRule('服务规则'), id: 'one', keywords: ['帮助', '说明'], reply: '查看帮助中心', priority: 10 }
const second = { ...createKeywordRule('开发规则'), id: 'two', keywords: ['代码'], action: 'inject', injection: '提供代码', category: '开发', priority: 20 }
const base = { version: 1, revision: 'rev-one', enabled: false, legacy: false, rules: [original, second] }
async function render() {
  const router = createRouter({ history: createMemoryHistory(), routes: [{ path: '/', component: KeywordRulesView }] })
  await router.push('/'); await router.isReady()
  const wrapper = mount({ template: '<router-view />' }, { global: { plugins: [router] } })
  await flushPromises(); return wrapper
}
beforeEach(() => {
  get.mockReset(); put.mockReset(); post.mockReset()
  get.mockImplementation(async (path: string) => ({ data: path === '/admin/keyword-rules' ? JSON.parse(JSON.stringify(base)) : [] }))
  put.mockImplementation(async (_path: string, body: unknown) => ({ data: { ...body as object, revision: 'rev-two' } }))
})
describe('independent keyword rules console', () => {
  it('searches by content and filters by action without requiring risk-control settings', async () => {
    const wrapper = await render()
    expect(wrapper.get('aside').text()).toContain('服务规则')
    expect(wrapper.get('aside').text()).toContain('开发规则')
    await wrapper.get('input[aria-label="keywordRules.search"]').setValue('提供代码')
    expect(wrapper.get('aside').text()).not.toContain('服务规则')
    expect(wrapper.get('aside').text()).toContain('开发规则')
    await wrapper.get('input[aria-label="keywordRules.search"]').setValue('')
    await wrapper.get('select[aria-label="keywordRules.action"]').setValue('reply')
    expect(wrapper.get('aside').text()).toContain('服务规则')
    expect(wrapper.get('aside').text()).not.toContain('开发规则')
    expect(get.mock.calls.every(([path]) => !String(path).includes('risk-control'))).toBe(true)
  })
  it('saves separate replies and injections in one revision-protected update', async () => {
    const wrapper = await render()
    await wrapper.get('[aria-label="keywordRules.editor"] textarea[maxlength="8000"]').setValue('新的指定回复')
    await wrapper.findAll('button').find(button => button.text() === 'keywordRules.save')!.trigger('click')
    await flushPromises()
    expect(put).toHaveBeenCalledTimes(1)
    expect(put).toHaveBeenCalledWith('/admin/keyword-rules', expect.objectContaining({ revision: 'rev-one', rules: [expect.objectContaining({ reply: '新的指定回复' }), expect.objectContaining({ action: 'inject', injection: '提供代码' })] }))
    expect(wrapper.get('[role="status"]').text()).toBe('keywordRules.saved')
  })
  it('preserves a paused library and provides an explicit feature switch', async () => {
    const wrapper = await render()
    await wrapper.findAll('input[role="switch"]')[0].setValue(true)
    await wrapper.findAll('button').find(button => button.text() === 'keywordRules.save')!.trigger('click')
    await flushPromises()
    expect(put).toHaveBeenCalledWith('/admin/keyword-rules', expect.objectContaining({ enabled: true, rules: expect.arrayContaining([expect.objectContaining({ id: 'one' }), expect.objectContaining({ id: 'two' })]) }))
  })
  it('reports concurrent edits without discarding the local draft', async () => {
    put.mockRejectedValueOnce({ response: { status: 409 } })
    const wrapper = await render()
    const input = wrapper.get('input[maxlength="100"]')
    await input.setValue('保留我的草稿')
    await wrapper.findAll('button').find(button => button.text() === 'keywordRules.save')!.trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('keywordRules.conflict')
    expect(input.element.value).toBe('保留我的草稿')
  })
})
