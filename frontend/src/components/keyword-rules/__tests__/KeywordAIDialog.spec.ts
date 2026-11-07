import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ref } from 'vue'
import KeywordAIDialog from '../KeywordAIDialog.vue'
import { createKeywordRule } from '@/utils/keywordRules'

const { list, generate, models, testRules } = vi.hoisted(() => ({ list: vi.fn(), generate: vi.fn(), models: vi.fn(), testRules: vi.fn() }))
vi.mock('@/api/keys', () => ({ keysAPI: { list } }))
vi.mock('@/api/admin/keywordAI', () => ({ generateKeywordAI: generate, keywordAIModels: models }))
vi.mock('@/api/admin/keywordRules', () => ({ testKeywordRules: testRules }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key, locale: ref('zh') }) }))
vi.mock('@/components/common/BaseDialog.vue', () => ({ default: { template: '<div><slot /><slot name="footer" /></div>' } }))
const suggestion = { rule: { ...createKeywordRule('余额查询'), enabled: false, keywords: ['查询余额'], reply: '查看用量页' }, keywords: ['查询余额'], reason: '具体问题，避免泛词。', examples: [{ text: '查询余额', should_match: true, explanation: '完整一致' }, { text: '查询余额!', should_match: false, explanation: '标点不同' }] }
const wrappers: ReturnType<typeof mount>[] = []
beforeEach(() => {
  vi.resetAllMocks()
  list.mockResolvedValue({ items: [{ id: 7, key: 'test-key', name: 'My key', status: 'active', group_id: 1, quota: 0, quota_used: 0 }] })
  models.mockResolvedValue(['alias'])
  generate.mockResolvedValue({ summary: 'Use precise phrases.', model: 'provider-model', suggestions: [structuredClone(suggestion)] })
  testRules.mockImplementation(async ({ text }: { text: string }) => ({ hits: text === '查询余额' ? [{ rule_id: suggestion.rule.id }] : [] }))
})
afterEach(() => { wrappers.forEach(wrapper => wrapper.unmount()); wrappers.length = 0 })
async function render(target = null as ReturnType<typeof createKeywordRule> | null) {
  const wrapper = mount(KeywordAIDialog, { props: { target } }); wrappers.push(wrapper)
  await flushPromises()
  await wrapper.get('textarea[maxlength="3000"]').setValue('精确识别余额查询')
  return wrapper
}
async function run(wrapper: Awaited<ReturnType<typeof render>>) {
  await wrapper.get('form').trigger('submit'); await flushPromises()
}
describe('AI-assisted keyword review', () => {
  it('requires a deliberate model call and checks examples before adding a paused draft', async () => {
    const wrapper = await render()
    expect(generate).not.toHaveBeenCalled()
    await run(wrapper)
    expect(generate).toHaveBeenCalledTimes(1)
    expect(testRules).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('keywordRules.aiExpectedHit')
    expect(wrapper.text()).toContain('keywordRules.aiActualMiss')
    expect(wrapper.emitted('apply')).toBeUndefined()
    await wrapper.findAll('button').find(button => button.text() === 'keywordRules.aiApply')!.trigger('click')
    expect(wrapper.emitted('apply')?.[0][0]).toMatchObject({ rules: [expect.objectContaining({ enabled: false, keywords: ['查询余额'] })] })
  })
  it('keeps mismatched or failed examples out of the draft', async () => {
    testRules.mockResolvedValue({ hits: [] })
    const wrapper = await render(); await run(wrapper)
    expect(wrapper.text()).toContain('keywordRules.aiExampleMismatch')
    expect(wrapper.get('input[type="checkbox"]').attributes('disabled')).toBeDefined()
    const apply = wrapper.findAll('button').find(button => button.text() === 'keywordRules.aiApply')!
    expect(apply.attributes('disabled')).toBeDefined()
    await apply.trigger('click')
    expect(wrapper.emitted('apply')).toBeUndefined()
  })
  it('invalidates suggestions when the goal changes and ignores cancelled results', async () => {
    const wrapper = await render(); await run(wrapper)
    await wrapper.get('textarea[maxlength="3000"]').setValue('新的需求')
    expect(wrapper.findAll('article')).toHaveLength(0)
    let resolve: (value: unknown) => void = () => {}
    generate.mockImplementationOnce(() => new Promise(accept => { resolve = accept }))
    await wrapper.get('form').trigger('submit'); await flushPromises()
    await wrapper.findAll('button').find(button => button.text() === 'keywordRules.aiCancelGeneration')!.trigger('click')
    resolve({ summary: 'Stale', model: 'alias', suggestions: [suggestion] }); await flushPromises()
    expect(wrapper.findAll('article')).toHaveLength(0)
  })
  it('retains the target snapshot when expanding, without changing the original rule', async () => {
    const target = { ...createKeywordRule('原规则'), keywords: ['原词'], reply: '保持回复', group_ids: [3] }
    const wrapper = await render(target)
    await wrapper.findAll('select')[1].setValue('expand')
    await run(wrapper)
    expect(generate.mock.calls[0][2].target).toEqual(target)
    await wrapper.findAll('button').find(button => button.text() === 'keywordRules.aiApply')!.trigger('click')
    expect(wrapper.emitted('apply')?.[0][0]).toMatchObject({ rules: [], targetId: target.id, targetSnapshot: JSON.stringify(target), keywords: ['查询余额'] })
    expect(target.keywords).toEqual(['原词'])
  })
  it('offers a setup path when there are no eligible keys and never calls a model', async () => {
    list.mockResolvedValue({ items: [] })
    const wrapper = await render()
    expect(wrapper.get('a[href="/keys"]').text()).toBe('keywordRules.aiManageKeys')
    await run(wrapper)
    expect(generate).not.toHaveBeenCalled()
  })
})
