import { flushPromises, mount } from '@vue/test-utils'
import { reactive } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import VersionBadge from '../VersionBadge.vue'
const { state, performUpdate, rollback, getRollbackVersions } = vi.hoisted(() => ({ state: {} as Record<string, unknown>, performUpdate: vi.fn(), rollback: vi.fn(), getRollbackVersions: vi.fn() }))
vi.mock('@/stores', () => ({ useAppStore: () => state, useAuthStore: () => ({ isAdmin: true }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copied: false, copyToClipboard: vi.fn() }) }))
vi.mock('@/api/admin/system', () => ({ performUpdate, rollback, getRollbackVersions, restartService: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
beforeEach(() => { Object.assign(state, reactive({ versionLoading: false, currentVersion: '0.2.21-custom.1', latestVersion: '0.2.22', hasUpdate: true, releaseInfo: { html_url: 'https://github.com/Wei-Shaw/sub2api/releases/tag/v0.2.22' }, buildType: 'release', customBuild: true, fetchVersion: vi.fn(), clearVersionCache: vi.fn() })); vi.clearAllMocks() })
describe('custom release protection', () => {
  it('shows protection and upstream notes while hiding replacement and rollback actions', async () => {
    const wrapper = mount(VersionBadge)
    await wrapper.get('button').trigger('click'); await flushPromises()
    expect(wrapper.get('[data-testid="custom-build-protection"]').text()).toContain('version.customProtected')
    expect(wrapper.text()).not.toContain('version.updateNow')
    expect(wrapper.text()).not.toContain('version.rollbackSelectVersion')
    expect(wrapper.findAll('a').map(a => a.attributes('href'))).toContain('https://github.com/ggkong/xiaofei_api')
    expect(performUpdate).not.toHaveBeenCalled(); expect(rollback).not.toHaveBeenCalled(); expect(getRollbackVersions).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('keeps the custom explanation visible when its upstream base is current', async () => {
    state.hasUpdate = false
    const wrapper = mount(VersionBadge)
    await wrapper.get('button').trigger('click'); await flushPromises()
    expect(wrapper.find('[data-testid="custom-build-protection"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('version.updateNow')
    wrapper.unmount()
  })
})
