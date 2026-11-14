<template>
  <section class="card space-y-5 p-5" aria-labelledby="archive-local-title">
    <header class="flex flex-wrap items-start justify-between gap-3"><div><h2 id="archive-local-title" class="font-semibold text-gray-950 dark:text-white">{{ t('archiveLocal.title') }}</h2><p class="mt-2 max-w-3xl text-xs leading-6 text-gray-500">{{ t('archiveLocal.description') }}</p></div><div class="flex gap-2"><button class="btn btn-secondary" :disabled="busy" @click="refresh">{{ t('archiveLocal.refresh') }}</button><button class="btn btn-secondary" @click="emit('close')">{{ t('archiveLocal.close') }}</button></div></header>
    <div class="rounded-xl border border-primary-100 bg-primary-50/40 p-4 dark:border-primary-900 dark:bg-primary-950/20"><a class="btn btn-primary" href="https://github.com/ggkong/xiaofei_api/releases/tag/archive-desktop-v1.0.0" target="_blank" rel="noopener noreferrer">{{ t('archiveLocal.download') }}</a><p class="mt-3 text-xs leading-6 text-gray-500">{{ t('archiveLocal.setup') }}</p></div>
    <form class="grid gap-3 sm:grid-cols-[1fr_2fr_auto]" @submit.prevent="pair"><label class="text-xs text-gray-500">{{ t('archiveLocal.name') }}<input v-model.trim="name" class="input mt-2 w-full" required maxlength="100" /></label><label class="text-xs text-gray-500">{{ t('archiveLocal.publicKey') }}<input v-model.trim="publicKey" class="input mt-2 w-full font-mono text-xs" required maxlength="1024" placeholder="ssh-ed25519 AAAA…" /></label><button class="btn btn-primary self-end" :disabled="busy">{{ t('archiveLocal.pair') }}</button></form>
    <p class="text-xs text-gray-400">{{ t('archiveLocal.keyHint') }} {{ t('archiveLocal.pairHint') }}</p>
    <p v-if="message" class="rounded-lg bg-primary-50 p-3 text-xs text-primary-700 dark:bg-primary-950" role="status">{{ message }}</p><p v-if="error" class="rounded-lg bg-red-50 p-3 text-xs text-red-700 dark:bg-red-950" role="alert">{{ error }}</p>
    <p v-if="!devices.length" class="py-3 text-xs text-gray-400">{{ t('archiveLocal.empty') }}</p>
    <div v-for="device in devices" :key="device.id" class="space-y-4 rounded-xl border border-gray-200 p-4 dark:border-gray-800"><div class="flex flex-wrap items-start justify-between gap-3"><div><h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ device.name }}</h3><p class="mt-1 text-xs text-gray-500">{{ t('archiveLocal.confirmed', { count: device.confirmed_records }) }} · {{ device.last_seen_at ? t('archiveLocal.lastSync', { time: new Date(device.last_seen_at).toLocaleString() }) : t('archiveLocal.never') }}</p></div><label class="flex items-center gap-2 text-xs text-gray-500"><input v-model="device.enabled" type="checkbox" />{{ t('archiveLocal.enabled') }}</label></div>
      <div class="grid gap-4 rounded-lg bg-gray-50 p-3 dark:bg-gray-900 sm:grid-cols-[1fr_auto]"><div><label class="flex items-center gap-2 text-xs font-medium text-gray-700 dark:text-gray-300"><input v-model="device.offload_enabled" type="checkbox" :disabled="!device.confirmed_records" />{{ t('archiveLocal.offload') }}</label><p class="mt-2 text-xs leading-6 text-gray-500">{{ t('archiveLocal.offloadHint') }}</p></div><label class="text-xs text-gray-500">{{ t('archiveLocal.keepDays') }}<input v-model.number="device.keep_days" class="input mt-2 block w-28" type="number" min="1" max="3650" /></label></div><button class="btn btn-secondary" :disabled="busy" @click="save(device)">{{ t('archiveLocal.save') }}</button></div>
    <p class="text-xs leading-6 text-gray-400">{{ t('archiveLocal.restoreHint') }}</p>
  </section>
</template>
<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiClient } from '@/api/client'
import type { ArchiveDevice } from '@/api/admin/conversationArchive'
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const devices = ref<ArchiveDevice[]>([])
const name = ref('Windows')
const publicKey = ref('')
const busy = ref(false)
const error = ref('')
const message = ref('')
const originalOffload = new Map<string, boolean>()
let active = true
async function load() { const { data } = await apiClient.get<ArchiveDevice[]>('/admin/conversation-archives/devices'); if (!active) return; devices.value = data; for (const d of data) originalOffload.set(d.id, d.offload_enabled) }
async function refresh() { if (busy.value) return; busy.value = true; error.value = ''; try { await load() } catch { if (active) error.value = t('archiveLocal.error') } finally { busy.value = false } }
async function pair() { if (busy.value) return; busy.value = true; error.value = ''; message.value = ''; try { await apiClient.post('/admin/conversation-archives/devices', { name: name.value, public_key: publicKey.value }); if (!active) return; publicKey.value = ''; message.value = t('archiveLocal.pairHint'); await load() } catch { if (active) error.value = t('archiveLocal.error') } finally { busy.value = false } }
async function save(device: ArchiveDevice) { if (busy.value) return; if (device.offload_enabled && !originalOffload.get(device.id) && !window.confirm(t('archiveLocal.confirm'))) return; busy.value = true; error.value = ''; message.value = ''; try { await apiClient.put(`/admin/conversation-archives/devices/${device.id}`, { enabled: device.enabled, offload_enabled: device.offload_enabled, keep_days: device.keep_days }); if (!active) return; message.value = t('archiveLocal.saved'); await load() } catch { if (active) error.value = t('archiveLocal.error') } finally { busy.value = false } }
onMounted(() => { const url = new URL(window.location.href); const key = url.searchParams.get('archive_key'); if (key && key.length <= 1024 && key.startsWith('ssh-ed25519 ')) { publicKey.value = key; url.searchParams.delete('archive_key'); window.history.replaceState(window.history.state, '', url) } void load().catch(() => { if (active) error.value = t('archiveLocal.error') }) })
onBeforeUnmount(() => { active = false })
</script>
