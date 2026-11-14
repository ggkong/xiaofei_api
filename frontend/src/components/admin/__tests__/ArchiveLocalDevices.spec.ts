import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ArchiveLocalDevices from '../ArchiveLocalDevices.vue'
const { get, post, put } = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get, post, put } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const device = { id: 'a'.repeat(64), name: 'My Windows', enabled: true, offload_enabled: false, keep_days: 7, confirmed_records: 0, last_seen_at: null }
afterEach(() => { vi.restoreAllMocks(); window.history.replaceState({}, '', '/') })
describe('local archive devices', () => {
  it('prefills only a public key and pairs explicitly with migration initially locked', async () => {
    get.mockResolvedValue({ data: [device] }); post.mockResolvedValue({ data: device })
    window.history.replaceState({}, '', '/admin/conversation-archives?archive_key=ssh-ed25519%20PUBLIC')
    const wrapper = mount(ArchiveLocalDevices); await flushPromises()
    expect(wrapper.findAll('input')[1].element.value).toBe('ssh-ed25519 PUBLIC')
    expect(window.location.search).toBe('')
    expect(post).not.toHaveBeenCalled()
    expect(wrapper.findAll('input[type="checkbox"]')[1].attributes('disabled')).toBeDefined()
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(post).toHaveBeenCalledWith('/admin/conversation-archives/devices', { name: 'Windows', public_key: 'ssh-ed25519 PUBLIC' })
    expect(put).not.toHaveBeenCalled()
    wrapper.unmount()
  })
  it('requires an explicit migration confirmation and sends each independent setting', async () => {
    get.mockResolvedValue({ data: [{ ...device, confirmed_records: 20 }] }); put.mockResolvedValue({ data: {} })
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mount(ArchiveLocalDevices); await flushPromises()
    await wrapper.findAll('input[type="checkbox"]')[1].setValue(true)
    const save = wrapper.findAll('button').find(b => b.text() === 'archiveLocal.save')!
    await save.trigger('click'); expect(put).not.toHaveBeenCalled()
    confirm.mockReturnValue(true); await save.trigger('click'); await flushPromises()
    expect(put).toHaveBeenCalledWith(`/admin/conversation-archives/devices/${device.id}`, { enabled: true, offload_enabled: true, keep_days: 7 })
    wrapper.unmount()
  })
  it('shows revoked or failed access without retaining a misleading success', async () => {
    get.mockResolvedValue({ data: [] }); post.mockRejectedValue(new Error('403'))
    const wrapper = mount(ArchiveLocalDevices); await flushPromises(); await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('archiveLocal.error')
    wrapper.unmount()
  })
})
