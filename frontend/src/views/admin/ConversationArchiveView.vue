<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-5">
      <header class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <div class="mb-2 flex items-center gap-2 text-xs font-medium uppercase tracking-widest text-primary-600 dark:text-primary-400">
            <span class="h-1.5 w-1.5 rounded-full bg-primary-500" />{{ t('conversationArchive.eyebrow') }}
          </div>
          <h1 class="text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">{{ t('conversationArchive.title') }}</h1>
          <p class="mt-2 text-sm text-gray-500">{{ t('conversationArchive.description') }}</p>
        </div>
        <div class="flex items-center gap-2">
          <span v-if="captureEnabled === true" class="mr-2 hidden items-center gap-1.5 rounded-full bg-emerald-50 px-3 py-1.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300 sm:inline-flex"><span class="h-1.5 w-1.5 rounded-full bg-emerald-500" />{{ t('conversationArchive.captureActive') }}</span>
          <button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('conversationArchive.refresh') }}</button>
          <button class="btn btn-primary" :aria-expanded="showRules" @click="showRules = !showRules">{{ t('conversationArchive.keywords') }}</button>
        </div>
      </header>

      <ConversationKeywordRules v-if="showRules" @close="showRules = false" />
      <div v-if="captureEnabled === false" class="rounded-xl bg-amber-50 p-4 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-100">{{ t('conversationArchive.disabled') }}</div>

      <div class="grid grid-cols-3 gap-3">
        <div v-for="stat in [{ label: 'pageRecords', value: items.length }, { label: 'pageMembers', value: memberCount }, { label: 'pageBlocked', value: blockedCount }]" :key="stat.label" class="rounded-xl border border-gray-200 bg-white px-4 py-4 dark:border-gray-800 dark:bg-gray-900">
          <p class="text-xs text-gray-500">{{ t(`conversationArchive.${stat.label}`) }}</p>
          <p class="mt-2 text-2xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ stat.value }}<span class="ml-2 text-xs font-normal text-gray-400">{{ t('conversationArchive.currentPage') }}</span></p>
        </div>
      </div>

      <section class="card overflow-hidden">
        <form class="space-y-3 border-b border-gray-100 p-4 dark:border-gray-800" @submit.prevent="search">
          <div class="flex flex-wrap items-end gap-3">
            <label class="min-w-48 flex-1 text-xs font-medium text-gray-500">{{ t('conversationArchive.searchLabel') }}<input v-model.trim="filters.q" class="input mt-1.5 w-full" maxlength="200" :placeholder="t('conversationArchive.searchPlaceholder')" /></label>
            <label class="text-xs font-medium text-gray-500">{{ t('conversationArchive.model') }}<input v-model.trim="filters.model" class="input mt-1.5 w-40" maxlength="255" :placeholder="t('conversationArchive.allModels')" /></label>
            <label class="text-xs font-medium text-gray-500">{{ t('conversationArchive.status') }}
              <select v-model="filters.status" class="input mt-1.5 w-32">
                <option value="">{{ t('conversationArchive.all') }}</option><option value="blocked">{{ t('conversationArchive.blocked') }}</option>
                <option value="complete">{{ t('conversationArchive.complete') }}</option><option value="interrupted">{{ t('conversationArchive.interrupted') }}</option><option value="error">{{ t('conversationArchive.failed') }}</option>
              </select>
            </label>
            <button class="btn btn-primary" :disabled="loading" type="submit">{{ t('conversationArchive.search') }}</button>
          </div>
          <details class="text-xs text-gray-500">
            <summary class="w-fit cursor-pointer py-1">{{ t('conversationArchive.moreFilters') }}</summary>
            <div class="mt-3 flex flex-wrap gap-3">
              <label>{{ t('conversationArchive.user') }}<input v-model="filters.user_id" class="input mt-1 block w-28" type="number" min="1" /></label>
              <label>{{ t('conversationArchive.key') }}<input v-model="filters.api_key_id" class="input mt-1 block w-28" type="number" min="1" /></label>
              <label>{{ t('conversationArchive.session') }}<input v-model.trim="filters.session_id" class="input mt-1 block" maxlength="255" /></label>
              <label>{{ t('conversationArchive.from') }}<input v-model="filters.created_from" class="input mt-1 block" type="datetime-local" /></label>
              <label>{{ t('conversationArchive.until') }}<input v-model="filters.created_until" class="input mt-1 block" type="datetime-local" /></label>
            </div>
          </details>
        </form>
        <p v-if="error" class="mx-4 mt-4 rounded-lg bg-red-50 p-3 text-sm text-red-600 dark:bg-red-950" role="alert">{{ error }}</p>
        <div class="overflow-x-auto">
          <table class="w-full min-w-[760px] table-fixed text-left text-sm">
            <thead class="bg-gray-50/80 text-xs font-medium text-gray-500 dark:bg-gray-800/50"><tr>
              <th class="w-44 px-5 py-3">{{ t('conversationArchive.member') }}</th>
              <th class="w-40 px-3 py-3">{{ t('conversationArchive.model') }}</th>
              <th class="px-3 py-3">{{ t('conversationArchive.question') }}</th>
              <th class="w-28 px-3 py-3">{{ t('conversationArchive.status') }}</th>
              <th class="w-20 px-3 py-3"><span class="sr-only">{{ t('conversationArchive.actions') }}</span></th>
            </tr></thead>
            <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
              <tr v-for="item in items" :key="item.id" class="group align-top transition-colors hover:bg-primary-50/30 dark:hover:bg-gray-800/40">
                <td class="px-5 py-4"><div class="flex items-center gap-2.5"><span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-primary-50 text-xs font-semibold text-primary-700 dark:bg-primary-950 dark:text-primary-300">{{ initials(item) }}</span><div class="min-w-0"><p class="truncate font-medium text-gray-900 dark:text-gray-100" :title="item.user_email">{{ memberName(item) }}</p><p class="mt-1 text-[11px] text-gray-400">{{ new Date(item.created_at).toLocaleString() }}</p></div></div></td>
                <td class="px-3 py-4"><span class="inline-block max-w-full truncate rounded-md border border-gray-200 bg-gray-50 px-2 py-1 font-mono text-xs text-gray-700 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300" :title="item.model">{{ item.model || t('conversationArchive.unknownModel') }}</span></td>
                <td class="px-3 py-4"><button class="block w-full text-left" :disabled="detailLoading || deleting" @click="openDetail(item.id)"><p class="line-clamp-2 whitespace-pre-wrap break-words font-medium leading-relaxed text-gray-900 group-hover:text-primary-700 dark:text-gray-100 dark:group-hover:text-primary-300">{{ item.prompt_preview || (item.prompt_omitted ? t('conversationArchive.promptOmitted') : t('conversationArchive.toolTurn')) }}</p><p v-if="item.response_preview" class="mt-1.5 line-clamp-1 text-xs leading-relaxed text-gray-500">{{ item.response_preview }}</p></button></td>
                <td class="px-3 py-4"><span class="inline-flex rounded-full px-2 py-1 text-[11px] font-medium" :class="item.blocked ? 'bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300' : item.status === 'complete' ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300' : 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'">{{ item.blocked ? t('conversationArchive.blocked') : statusLabel(item.status) }}</span></td>
                <td class="px-3 py-4"><button class="text-xs font-medium text-primary-600 hover:underline dark:text-primary-400" :disabled="detailLoading || deleting" @click="openDetail(item.id)">{{ t('conversationArchive.view') }}</button></td>
              </tr>
              <tr v-if="!items.length"><td colspan="5" class="px-5 py-16 text-center"><p class="text-sm font-medium text-gray-500">{{ loading ? t('conversationArchive.loading') : t('conversationArchive.empty') }}</p><p v-if="!loading" class="mt-2 text-xs text-gray-400">{{ t('conversationArchive.emptyHint') }}</p></td></tr>
            </tbody>
          </table>
        </div>
        <div class="flex items-center justify-between border-t border-gray-100 px-5 py-3 dark:border-gray-800"><span class="text-xs text-gray-400">{{ t('conversationArchive.pageHint') }}</span><div class="flex gap-2"><button class="btn btn-secondary" :disabled="loading || !cursors.length" @click="previous">{{ t('conversationArchive.previous') }}</button><button class="btn btn-secondary" :disabled="loading || !nextCursor" @click="next">{{ t('conversationArchive.next') }}</button></div></div>
      </section>

      <section v-if="detail" ref="detailPanel" class="card scroll-mt-6 overflow-hidden" aria-live="polite">
        <div class="flex items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-gray-800"><div><h2 class="font-semibold text-gray-900 dark:text-white">{{ memberName(detail) }} <span class="ml-2 text-xs font-normal text-gray-500">{{ detail.model }}</span></h2><p class="mt-1 text-xs text-gray-400">{{ new Date(detail.created_at).toLocaleString() }}</p></div><button class="btn btn-secondary" @click="closeDetail">{{ t('conversationArchive.close') }}</button></div>
        <div class="space-y-5 p-5">
          <p v-if="detail.truncated" class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-950 dark:text-amber-300">{{ t('conversationArchive.limitNotice') }}</p>
          <div class="grid gap-5 lg:grid-cols-2">
            <div class="rounded-xl border border-gray-200 p-4 dark:border-gray-800"><h3 class="mb-3 text-xs font-semibold text-gray-500">{{ t('conversationArchive.question') }}</h3><p v-for="(message, index) in inputMessages" :key="index" class="max-h-96 overflow-auto whitespace-pre-wrap break-words text-sm leading-7 text-gray-900 dark:text-gray-100">{{ message.text }}</p><p v-if="!inputMessages.length" class="text-sm text-gray-400">{{ detail.prompt_preview || t('conversationArchive.promptOmitted') }}</p></div>
            <div class="rounded-xl border border-primary-100 bg-primary-50/40 p-4 dark:border-primary-900 dark:bg-primary-950/20"><h3 class="mb-3 text-xs font-semibold text-primary-600 dark:text-primary-400">{{ t('conversationArchive.output') }}</h3><p v-for="(message, index) in outputMessages" :key="index" class="max-h-96 overflow-auto whitespace-pre-wrap break-words text-sm leading-7 text-gray-900 dark:text-gray-100">{{ message.text }}</p><p v-if="!outputMessages.length" class="text-sm text-gray-400">{{ detail.blocked ? t('conversationArchive.blockMessage') : t('conversationArchive.toolReply') }}</p></div>
          </div>
          <div class="flex items-start justify-between gap-4"><details class="min-w-0 flex-1 text-xs text-gray-500"><summary class="cursor-pointer">{{ t('conversationArchive.raw') }}</summary><pre class="mt-3 max-h-96 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-gray-50 p-4 dark:bg-gray-900">{{ pretty(detail.request_body) }}</pre><pre class="mt-3 max-h-96 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-gray-50 p-4 dark:bg-gray-900">{{ pretty(detail.response_body) }}</pre></details><button class="shrink-0 text-xs text-red-600 hover:underline" :disabled="deleting" @click="removeDetail">{{ t('conversationArchive.delete') }}</button></div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConversationKeywordRules from '@/components/admin/ConversationKeywordRules.vue'
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
  user_name?: string
  user_email?: string
  prompt_preview?: string
  response_preview?: string
  blocked?: boolean
  prompt_omitted?: boolean
  request_body?: unknown
  response_body?: unknown
}
const { t } = useI18n()
const showRules = ref(false)
const filters = ref({ q: '', user_id: '', api_key_id: '', model: '', session_id: '', status: '', created_from: '', created_until: '' })
const activeFilters = ref<Record<string, string | number>>({})
const items = ref<ArchiveRecord[]>([])
const detail = ref<ArchiveRecord | null>(null)
const detailPanel = ref<HTMLElement | null>(null)
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
const inputMessages = computed(() => archiveRequestMessages(detail.value?.request_body).filter(message => message.role === 'user').slice(-1))
const memberCount = computed(() => new Set(items.value.map(item => item.user_id)).size)
const blockedCount = computed(() => items.value.filter(item => item.blocked).length)
const memberName = (item: ArchiveRecord) => item.user_name?.trim() || item.user_email || `${t('conversationArchive.member')} #${item.user_id ?? '—'}`
const initials = (item: ArchiveRecord) => [...memberName(item)].slice(0, 2).join('').toUpperCase()
const outputMessages = computed(() => archiveResponseMessages(detail.value?.response_body))
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
  for (const key of ['q', 'user_id', 'api_key_id', 'model', 'session_id', 'status'] as const) if (filters.value[key] !== '') params[key] = filters.value[key]
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
    if (version === detailVersion) {
      detail.value = data
      await nextTick()
      if (version === detailVersion) detailPanel.value?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
    }
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
