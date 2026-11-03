<template>
  <AppLayout>
    <div class="space-y-5">
      <div>
        <h1 class="text-xl font-semibold">{{ t('conversationArchive.title') }}</h1>
        <p class="mt-2 text-sm text-gray-500">{{ t('conversationArchive.description') }}</p>
      </div>
      <div v-if="captureEnabled === false" class="rounded-lg bg-amber-50 p-4 text-amber-900 dark:bg-amber-950 dark:text-amber-100">
        {{ t('conversationArchive.disabled') }}
      </div>
      <form class="card flex flex-wrap items-end gap-3 p-4" @submit.prevent="search">
        <label class="text-sm">{{ t('conversationArchive.user') }}<input v-model="filters.user_id" class="input mt-1" type="number" min="1" /></label>
        <label class="text-sm">{{ t('conversationArchive.key') }}<input v-model="filters.api_key_id" class="input mt-1" type="number" min="1" /></label>
        <label class="text-sm">{{ t('conversationArchive.model') }}<input v-model.trim="filters.model" class="input mt-1" maxlength="255" /></label>
        <label class="text-sm">{{ t('conversationArchive.session') }}<input v-model.trim="filters.session_id" class="input mt-1" maxlength="255" /></label>
        <label class="text-sm">{{ t('conversationArchive.from') }}<input v-model="filters.created_from" class="input mt-1" type="datetime-local" /></label>
        <label class="text-sm">{{ t('conversationArchive.until') }}<input v-model="filters.created_until" class="input mt-1" type="datetime-local" /></label>
        <label class="text-sm">{{ t('conversationArchive.status') }}
          <select v-model="filters.status" class="input mt-1">
            <option value="">{{ t('conversationArchive.all') }}</option>
            <option value="complete">{{ t('conversationArchive.complete') }}</option>
            <option value="interrupted">{{ t('conversationArchive.interrupted') }}</option>
            <option value="error">{{ t('conversationArchive.failed') }}</option>
          </select>
        </label>
        <button class="btn btn-primary" :disabled="loading" type="submit">{{ t('conversationArchive.search') }}</button>
      </form>
      <p v-if="error" class="text-sm text-red-600" role="alert">{{ error }}</p>
      <div class="card overflow-x-auto">
        <table class="w-full text-left text-sm">
          <thead class="bg-gray-50 dark:bg-gray-800"><tr>
            <th class="p-3">ID / {{ t('conversationArchive.time') }}</th>
            <th class="p-3">{{ t('conversationArchive.user') }} / {{ t('conversationArchive.key') }}</th>
            <th class="p-3">{{ t('conversationArchive.model') }}</th>
            <th class="p-3">{{ t('conversationArchive.session') }}</th>
            <th class="p-3">{{ t('conversationArchive.status') }}</th>
            <th class="p-3">{{ t('conversationArchive.actions') }}</th>
          </tr></thead>
          <tbody>
            <tr v-for="item in items" :key="item.id" class="border-t border-gray-100 dark:border-gray-700">
              <td class="p-3">#{{ item.id }}<br />{{ new Date(item.created_at).toLocaleString() }}</td>
              <td class="p-3">{{ item.user_id ?? '—' }} / {{ item.api_key_id ?? '—' }}</td>
              <td class="p-3">{{ item.model || '—' }}</td>
              <td class="max-w-48 break-all p-3">{{ item.session_id || t('conversationArchive.noSession') }}</td>
              <td class="p-3">{{ statusLabel(item.status) }}<span v-if="item.truncated"> · {{ t('conversationArchive.truncated') }}</span></td>
              <td class="p-3"><button class="text-primary-600 hover:underline" :disabled="detailLoading || deleting" @click="openDetail(item.id)">{{ t('conversationArchive.view') }}</button></td>
            </tr>
            <tr v-if="!items.length"><td colspan="6" class="p-8 text-center text-gray-500">{{ loading ? t('conversationArchive.loading') : t('conversationArchive.empty') }}</td></tr>
          </tbody>
        </table>
      </div>
      <div class="flex gap-3">
        <button class="btn btn-secondary" :disabled="loading || !cursors.length" @click="previous">{{ t('conversationArchive.previous') }}</button>
        <button class="btn btn-secondary" :disabled="loading || !nextCursor" @click="next">{{ t('conversationArchive.next') }}</button>
      </div>
      <section v-if="detail" class="card space-y-4 p-5" aria-live="polite">
        <div class="flex items-center justify-between gap-3">
          <h2 class="font-semibold">{{ t('conversationArchive.detail') }} #{{ detail.id }}</h2>
          <button class="btn btn-secondary" @click="closeDetail">{{ t('conversationArchive.close') }}</button>
        </div>
        <p class="break-all text-sm text-gray-500">{{ detail.endpoint }} · {{ detail.request_id }} · HTTP {{ detail.http_status }}</p>
        <p v-if="detail.truncated" class="text-sm text-amber-600">{{ t('conversationArchive.limitNotice') }}</p>
        <h3 class="font-medium">{{ t('conversationArchive.input') }}</h3>
        <article v-for="(message, index) in inputMessages" :key="`input-${index}`" class="rounded-lg border border-gray-200 p-4 dark:border-gray-700">
          <h4 class="mb-2 text-xs font-semibold text-gray-500">{{ roleLabel(message.role) }}</h4>
          <p class="max-h-96 overflow-auto whitespace-pre-wrap break-words text-sm">{{ message.text }}</p>
        </article>
        <p v-if="!inputMessages.length" class="text-sm text-gray-500">{{ t('conversationArchive.noText') }}</p>
        <h3 class="font-medium">{{ t('conversationArchive.output') }}</h3>
        <article v-for="(message, index) in outputMessages" :key="`output-${index}`" class="rounded-lg border border-primary-200 bg-primary-50 p-4 dark:border-primary-800 dark:bg-gray-900">
          <h4 class="mb-2 text-xs font-semibold text-gray-500">{{ roleLabel(message.role) }}</h4>
          <p class="max-h-96 overflow-auto whitespace-pre-wrap break-words text-sm">{{ message.text }}</p>
        </article>
        <p v-if="!outputMessages.length" class="text-sm text-gray-500">{{ t('conversationArchive.noText') }}</p>
        <details class="space-y-3">
          <summary class="cursor-pointer text-sm font-medium">{{ t('conversationArchive.raw') }}</summary>
          <h3 class="font-medium">{{ t('conversationArchive.input') }}</h3>
          <pre class="max-h-96 overflow-auto whitespace-pre-wrap break-all rounded bg-gray-50 p-4 text-sm dark:bg-gray-900">{{ pretty(detail.request_body) }}</pre>
          <h3 class="font-medium">{{ t('conversationArchive.output') }}</h3>
          <pre class="max-h-96 overflow-auto whitespace-pre-wrap break-all rounded bg-gray-50 p-4 text-sm dark:bg-gray-900">{{ pretty(detail.response_body) }}</pre>
        </details>
        <button class="btn btn-secondary text-red-600" :disabled="deleting" @click="removeDetail">{{ t('conversationArchive.delete') }}</button>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { apiClient } from '@/api/client'
import { archiveDateRange, archiveRequestMessages, archiveResponseMessages } from '@/utils/conversationArchive'

interface ArchiveRecord {
  id: number
  request_id: string
  user_id: number | null
  api_key_id: number | null
  session_id: string
  endpoint: string
  model: string
  http_status: number
  status: 'complete' | 'interrupted' | 'error'
  truncated: boolean
  created_at: string
  request_body?: unknown
  response_body?: unknown
}
const { t } = useI18n()
const filters = ref({ user_id: '', api_key_id: '', model: '', session_id: '', status: '', created_from: '', created_until: '' })
const activeFilters = ref<Record<string, string | number>>({})
const items = ref<ArchiveRecord[]>([])
const detail = ref<ArchiveRecord | null>(null)
const captureEnabled = ref<boolean | null>(null)
const loading = ref(false)
const detailLoading = ref(false)
const deleting = ref(false)
const error = ref('')
const beforeID = ref(0)
const nextCursor = ref(0)
const cursors = ref<number[]>([])
let loadVersion = 0
let detailVersion = 0
const pretty = (value: unknown) => JSON.stringify(value, null, 2)
const inputMessages = computed(() => archiveRequestMessages(detail.value?.request_body))
const outputMessages = computed(() => archiveResponseMessages(detail.value?.response_body))
const roleLabel = (role: string) => t(`conversationArchive.roles.${['system', 'developer', 'user', 'assistant', 'tool'].includes(role) ? role : 'message'}`)
const statusLabel = (status: ArchiveRecord['status']) => t(`conversationArchive.${status === 'error' ? 'failed' : status}`)
async function load() {
  const version = ++loadVersion
  loading.value = true
  error.value = ''
  try {
    const params: Record<string, string | number> = { limit: 30, ...activeFilters.value }
    if (beforeID.value) params.before_id = beforeID.value
    const { data } = await apiClient.get<{ items: ArchiveRecord[]; next_before_id: number; capture_enabled: boolean }>('/admin/conversation-archives', { params })
    if (version !== loadVersion) return
    items.value = data.items
    nextCursor.value = data.next_before_id
    captureEnabled.value = data.capture_enabled
  } catch {
    if (version === loadVersion) { items.value = []; nextCursor.value = 0; closeDetail(); error.value = t('conversationArchive.loadError') }
  } finally {
    if (version === loadVersion) loading.value = false
  }
}
function closeDetail() { detail.value = null; detailVersion++; detailLoading.value = false }
function search() {
  const range = archiveDateRange(filters.value.created_from, filters.value.created_until)
  if (!range) { error.value = t('conversationArchive.invalidRange'); return }
  const params: Record<string, string | number> = { ...range }
  for (const key of ['user_id', 'api_key_id', 'model', 'session_id', 'status'] as const) if (filters.value[key] !== '') params[key] = filters.value[key]
  activeFilters.value = params
  beforeID.value = 0; cursors.value = []; closeDetail(); void load()
}
function next() { if (loading.value || !nextCursor.value) return; cursors.value.push(beforeID.value); beforeID.value = nextCursor.value; closeDetail(); void load() }
function previous() { if (loading.value || !cursors.value.length) return; beforeID.value = cursors.value.pop() ?? 0; closeDetail(); void load() }
async function openDetail(id: number) {
  const version = ++detailVersion
  detailLoading.value = true
  detail.value = null
  error.value = ''
  try {
    const { data } = await apiClient.get<ArchiveRecord>(`/admin/conversation-archives/${id}`)
    if (version === detailVersion) detail.value = data
  } catch { if (version === detailVersion) error.value = t('conversationArchive.loadError') }
  finally { if (version === detailVersion) detailLoading.value = false }
}
async function removeDetail() {
  if (!detail.value || deleting.value || !window.confirm(t('conversationArchive.deleteConfirm'))) return
  const id = detail.value.id
  deleting.value = true
  try {
    await apiClient.delete(`/admin/conversation-archives/${id}`)
    if (detail.value?.id === id) closeDetail()
    await load()
  } catch { error.value = t('conversationArchive.loadError') }
  finally { deleting.value = false }
}
onMounted(load)
onBeforeUnmount(() => { loadVersion++; closeDetail() })
</script>
