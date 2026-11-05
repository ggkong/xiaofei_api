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
        <div class="flex flex-wrap items-center gap-2">
          <span v-if="captureEnabled === true && !capturePaused" class="mr-2 hidden items-center gap-1.5 rounded-full bg-emerald-50 px-3 py-1.5 text-xs font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300 sm:inline-flex"><span class="h-1.5 w-1.5 rounded-full bg-emerald-500" />{{ retentionDays === 0 ? t('conversationArchive.permanentActive') : t('conversationArchive.captureActive') }}</span>
          <button class="btn btn-secondary" :aria-expanded="showStorage" @click="showStorage = !showStorage">{{ t('conversationArchive.storageTitle') }}</button>
          <button class="btn btn-secondary" :disabled="loading" @click="load">{{ t('conversationArchive.refresh') }}</button>
          <a href="/admin/keyword-rules" class="btn btn-primary">{{ t('conversationArchive.keywords') }}</a>
        </div>
      </header>

      <ConversationArchiveStorage v-if="showStorage" @close="showStorage = false" @saved="load" />
      <div v-if="captureEnabled === false" class="rounded-xl bg-amber-50 p-4 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-100">{{ t('conversationArchive.disabled') }}</div>
      <div v-if="capturePaused" class="rounded-xl bg-amber-50 p-4 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-100">{{ t('conversationArchive.capacityPaused') }}</div>

      <div class="grid grid-cols-3 gap-3">
        <div v-for="stat in [{ label: 'pageRecords', value: items.reduce((total, item) => total + (item.call_count ?? 1), 0) }, { label: 'pageMembers', value: memberCount }, { label: 'pageBlocked', value: blockedCount }]" :key="stat.label" class="rounded-xl border border-gray-200 bg-white px-4 py-4 dark:border-gray-800 dark:bg-gray-900">
          <p class="text-xs text-gray-500">{{ t(`conversationArchive.${stat.label}`) }}</p>
          <p class="mt-2 text-2xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ stat.value }}<span class="ml-2 text-xs font-normal text-gray-400">{{ t('conversationArchive.currentPage') }}</span></p>
        </div>
      </div>

      <section class="card overflow-hidden">
        <form class="space-y-3 border-b border-gray-100 p-4 dark:border-gray-800" @submit.prevent="search">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="inline-flex rounded-lg bg-gray-100 p-1 text-xs dark:bg-gray-800"><button v-for="mode in ['groups', 'calls'] as const" :key="mode" type="button" class="rounded-md px-3 py-2 font-medium transition-colors" :class="viewMode === mode ? 'bg-white text-primary-700 shadow-sm dark:bg-gray-700 dark:text-primary-300' : 'text-gray-500'" :aria-pressed="viewMode === mode" @click="changeMode(mode)">{{ t(`conversationArchive.${mode === 'groups' ? 'mergedView' : 'allCalls'}`) }}</button></div>
            <p class="text-xs text-gray-400">{{ t('conversationArchive.mergeHint') }}</p>
          </div>
          <div class="flex flex-wrap items-end gap-3">
            <label class="min-w-48 flex-1 text-xs font-medium text-gray-500">{{ t('conversationArchive.searchLabel') }}<input v-model.trim="filters.q" class="input mt-1.5 w-full" maxlength="200" :placeholder="t('conversationArchive.searchPlaceholder')" /></label>
            <label class="text-xs font-medium text-gray-500">{{ t('conversationArchive.model') }}<input v-model.trim="filters.model" class="input mt-1.5 block w-40" maxlength="255" :placeholder="t('conversationArchive.allModels')" /></label>
            <label class="text-xs font-medium text-gray-500">{{ t('conversationArchive.status') }}
              <select v-model="filters.status" class="input mt-1.5 block w-32">
                <option value="">{{ t('conversationArchive.all') }}</option><option value="blocked">{{ t('conversationArchive.blocked') }}</option><option value="auto_reply">{{ t('conversationArchive.automaticReply') }}</option>
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
        <div class="divide-y divide-gray-100 dark:divide-gray-800 md:hidden">
          <button v-for="item in items" :key="item.id" class="block w-full space-y-3 p-4 text-left transition-colors hover:bg-primary-50/30" :disabled="detailLoading || deleting" @click="openGroup(item)">
            <div class="flex min-w-0 items-center justify-between gap-3"><span class="flex min-w-0 items-center gap-2"><span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary-50 text-xs font-semibold text-primary-700 dark:bg-primary-950 dark:text-primary-300">{{ initials(item) }}</span><span class="truncate text-sm font-semibold text-gray-900 dark:text-gray-100">{{ memberName(item) }}</span></span><span class="max-w-36 truncate rounded-md bg-gray-100 px-2 py-1 font-mono text-[11px] text-gray-600 dark:bg-gray-800 dark:text-gray-300">{{ displayModel(item) }}</span></div>
            <p class="line-clamp-3 whitespace-pre-wrap break-words text-sm font-medium leading-6 text-gray-900 dark:text-gray-100">{{ item.prompt_preview || (item.prompt_omitted ? t('conversationArchive.promptOmitted') : t('conversationArchive.toolTurn')) }}</p>
            <span v-if="(item.call_count ?? 1) > 1" class="inline-flex rounded-md bg-primary-50 px-2 py-1 text-[11px] font-medium text-primary-700 dark:bg-primary-950 dark:text-primary-300">{{ t('conversationArchive.groupCalls', { count: item.call_count }) }}</span><p v-if="item.response_preview" class="line-clamp-2 text-xs leading-5 text-gray-500">{{ item.response_preview }}</p>
            <div class="flex items-center justify-between text-[11px] text-gray-400"><span>{{ formatTime(item.created_at) }}</span><span :class="item.blocked ? 'text-red-600' : 'text-primary-600'">{{ item.auto_reply ? t('conversationArchive.automaticReply') : item.blocked ? t('conversationArchive.blocked') : t('conversationArchive.view') }}</span></div>
          </button>
          <p v-if="!items.length" class="p-8 text-center text-sm text-gray-400">{{ loading ? t('conversationArchive.loading') : t('conversationArchive.empty') }}</p>
        </div>
        <div class="overflow-x-auto">
          <table class="hidden w-full min-w-[760px] table-fixed text-left text-sm md:table">
            <thead class="bg-gray-50/80 text-xs font-medium text-gray-500 dark:bg-gray-800/50"><tr>
              <th class="w-44 px-5 py-3">{{ t('conversationArchive.member') }}</th>
              <th class="w-40 px-3 py-3">{{ t('conversationArchive.model') }}</th>
              <th class="px-3 py-3">{{ t('conversationArchive.question') }}</th>
              <th class="w-28 px-3 py-3">{{ t('conversationArchive.status') }}</th>
              <th class="w-20 px-3 py-3">{{ t('conversationArchive.actions') }}</th>
            </tr></thead>
            <tbody class="divide-y divide-gray-100 dark:divide-gray-800">
              <tr v-for="item in items" :key="item.id" class="group align-top transition-colors hover:bg-primary-50/30 dark:hover:bg-gray-800/40">
                <td class="px-5 py-4"><div class="flex items-center gap-2.5"><span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-primary-50 text-xs font-semibold text-primary-700 dark:bg-primary-950 dark:text-primary-300">{{ initials(item) }}</span><div class="min-w-0"><p class="truncate font-medium text-gray-900 dark:text-gray-100" :title="item.user_email">{{ memberName(item) }}</p><p class="mt-1 text-[11px] text-gray-400">{{ formatTime(item.created_at) }}</p></div></div></td>
                <td class="px-3 py-4"><span class="inline-block max-w-full truncate rounded-md border border-gray-200 bg-gray-50 px-2 py-1 font-mono text-xs text-gray-700 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-300" :title="item.models?.join(', ') || item.model">{{ displayModel(item) }}</span><p v-if="item.models && item.models.length > 1" class="mt-1 text-[11px] text-gray-400" :title="item.models.join(', ')">{{ t('conversationArchive.modelVariants', { count: item.models.length }) }}</p></td>
                <td class="px-3 py-4"><button class="block w-full text-left" :disabled="detailLoading || deleting" @click="openGroup(item)"><p class="line-clamp-2 whitespace-pre-wrap break-words font-medium leading-relaxed text-gray-900 group-hover:text-primary-700 dark:text-gray-100 dark:group-hover:text-primary-300">{{ item.prompt_preview || (item.prompt_omitted ? t('conversationArchive.promptOmitted') : t('conversationArchive.toolTurn')) }}</p><span v-if="(item.call_count ?? 1) > 1" class="mt-1.5 inline-flex rounded-md bg-primary-50 px-2 py-0.5 text-[11px] font-medium text-primary-700 dark:bg-primary-950 dark:text-primary-300">{{ t('conversationArchive.groupCalls', { count: item.call_count }) }}</span><p v-if="item.response_preview" class="mt-1.5 line-clamp-1 text-xs leading-relaxed text-gray-500">{{ item.response_preview }}</p></button></td>
                <td class="px-3 py-4"><span class="inline-flex rounded-full px-2 py-1 text-[11px] font-medium" :class="item.blocked ? 'bg-red-50 text-red-700 dark:bg-red-950 dark:text-red-300' : item.status === 'complete' ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300' : 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400'">{{ item.auto_reply ? t('conversationArchive.automaticReply') : item.blocked ? t('conversationArchive.blocked') : statusLabel(item.status) }}</span></td>
                <td class="px-3 py-4"><button class="text-xs font-medium text-primary-600 hover:underline dark:text-primary-400" :disabled="detailLoading || deleting" @click="openGroup(item)">{{ t('conversationArchive.view') }}</button></td>
              </tr>
              <tr v-if="!items.length"><td colspan="5" class="px-5 py-16 text-center"><p class="text-sm font-medium text-gray-500">{{ loading ? t('conversationArchive.loading') : t('conversationArchive.empty') }}</p><p v-if="!loading" class="mt-2 text-xs text-gray-400">{{ t('conversationArchive.emptyHint') }}</p></td></tr>
            </tbody>
          </table>
        </div>
        <div class="flex items-center justify-between border-t border-gray-100 px-5 py-3 dark:border-gray-800"><span class="text-xs text-gray-400">{{ t('conversationArchive.pageHint') }}</span><div class="flex gap-2"><button class="btn btn-secondary" :disabled="loading || !cursors.length" @click="previous">{{ t('conversationArchive.previous') }}</button><button class="btn btn-secondary" :disabled="loading || !nextCursor" @click="next">{{ t('conversationArchive.next') }}</button></div></div>
      </section>

      <section v-if="detail" ref="detailPanel" class="card scroll-mt-6 overflow-hidden" aria-live="polite">
        <div class="flex items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-gray-800"><div><h2 class="font-semibold text-gray-900 dark:text-white">{{ memberName(detail) }} <span class="ml-2 text-xs font-normal text-gray-500">{{ displayModel(detail) }}</span></h2><p class="mt-1 text-xs text-gray-400">{{ formatTime(detail.created_at) }}<span v-if="groupSummary && (groupSummary.call_count ?? 1) > 1"> · {{ t(`conversationArchive.${groupSummary.group_source === 'session' ? 'sessionGroup' : groupSummary.group_source === 'question' ? 'questionGroup' : 'unavailableGroup'}`) }} · {{ t('conversationArchive.groupCalls', { count: groupSummary.call_count }) }}</span></p></div><button class="btn btn-secondary shrink-0 whitespace-nowrap" @click="closeDetail">{{ t('conversationArchive.close') }}</button></div>
        <div v-if="groupCalls.length" class="border-b border-gray-100 bg-gray-50/60 px-5 py-4 dark:border-gray-800 dark:bg-gray-900/40">
          <div class="mb-3 flex items-center justify-between gap-3"><p class="text-xs text-gray-500">{{ t('conversationArchive.chooseCall') }}</p><button v-if="callsCursor" class="text-xs text-primary-600 hover:underline" :disabled="callsLoading" @click="loadGroupCalls(true)">{{ t('conversationArchive.moreCalls') }}</button></div>
          <div class="flex max-h-48 gap-2 overflow-auto pb-1"><button v-for="call in groupCalls" :key="call.id" class="w-48 shrink-0 rounded-xl border px-3 py-2.5 text-left text-xs transition-colors" :class="detail.id === call.id ? 'border-primary-400 bg-primary-50 dark:border-primary-600 dark:bg-primary-950' : 'border-gray-200 bg-white hover:border-primary-300 dark:border-gray-700 dark:bg-gray-800'" :disabled="detailLoading" @click="openDetail(call.id, false)"><p class="truncate font-medium text-gray-900 dark:text-gray-100">{{ displayModel(call) }}</p><p class="mt-1 text-[11px] text-gray-400">{{ formatTime(call.created_at) }}</p><p class="mt-2 line-clamp-2 leading-5 text-gray-500">{{ call.response_preview || call.prompt_preview || t('conversationArchive.noText') }}</p></button></div>
        </div>
        <div class="flex gap-5 border-b border-gray-100 px-5 dark:border-gray-800" role="tablist" :aria-label="t('conversationArchive.detail')"><button v-for="tab in ['exchange', 'context', 'models'] as const" :key="tab" class="border-b-2 py-3 text-sm font-medium" :class="detailTab === tab ? 'border-primary-600 text-primary-600 dark:text-primary-400' : 'border-transparent text-gray-500'" role="tab" :aria-selected="detailTab === tab" @click="detailTab = tab">{{ t(`conversationArchive.${tab}`) }}</button></div>
        <div class="space-y-5 p-5" role="tabpanel">
          <div v-if="detail.keyword_rules?.length" class="flex flex-wrap items-center gap-2 rounded-xl bg-primary-50 px-3 py-3 text-xs dark:bg-primary-950"><span class="font-medium text-primary-700 dark:text-primary-300">{{ t('keywordRules.matchedRules') }}</span><span v-for="hit in detail.keyword_rules" :key="hit.rule_id" class="rounded-md bg-white px-2 py-1 text-gray-600 dark:bg-gray-900 dark:text-gray-300">{{ hit.rule_name }} · {{ t(`keywordRules.actions.${hit.action}`) }}</span></div>
          <p v-if="detail.truncated" class="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-950 dark:text-amber-300">{{ t('conversationArchive.limitNotice') }}</p>
          <template v-if="detailTab === 'exchange'">
            <div class="grid gap-5 lg:grid-cols-2">
              <div class="rounded-xl border border-gray-200 p-4 dark:border-gray-800"><h3 class="mb-3 text-xs font-semibold text-gray-500">{{ t('conversationArchive.question') }}</h3><p v-for="(message, index) in inputMessages" :key="index" class="max-h-96 overflow-auto whitespace-pre-wrap break-words text-sm leading-7 text-gray-900 dark:text-gray-100">{{ message.text }}</p><p v-if="!inputMessages.length" class="text-sm text-gray-400">{{ detail.prompt_preview || t('conversationArchive.promptOmitted') }}</p></div>
              <div class="rounded-xl border border-primary-100 bg-primary-50/40 p-4 dark:border-primary-900 dark:bg-primary-950/20"><h3 class="mb-3 text-xs font-semibold text-primary-600 dark:text-primary-400">{{ detail.auto_reply ? t('conversationArchive.automaticReply') : t('conversationArchive.output') }}</h3><template v-for="(message, index) in outputMessages" :key="index"><details v-if="message.kind === 'tool'" class="text-xs text-gray-500"><summary class="cursor-pointer py-2">{{ message.name || t('conversationArchive.toolCall') }}</summary><pre class="max-h-80 overflow-auto whitespace-pre-wrap break-words leading-6">{{ message.text }}</pre></details><p v-else class="max-h-96 overflow-auto whitespace-pre-wrap break-words text-sm leading-7 text-gray-900 dark:text-gray-100">{{ message.text }}</p></template><p v-if="!outputMessages.length" class="text-sm text-gray-400">{{ detail.blocked ? t('conversationArchive.blockMessage') : detail.status === 'error' ? t('conversationArchive.errorReply') : t('conversationArchive.toolReply') }}</p></div>
            </div>
          </template>
          <template v-else-if="detailTab === 'context'"><p class="text-xs leading-5 text-gray-500">{{ t('conversationArchive.contextHint') }}</p><ConversationContext :key="detail.id" :messages="contextMessages" /></template>
          <div v-else class="space-y-4">
            <p v-if="detail.auto_reply" class="rounded-xl bg-primary-50 p-4 text-sm text-primary-700 dark:bg-primary-950 dark:text-primary-300">{{ t('conversationArchive.localReplyModelHint') }}</p>
            <dl class="grid gap-4 sm:grid-cols-2"><div v-for="field in modelFields" :key="field.label" class="rounded-xl border border-gray-200 p-4 dark:border-gray-800"><dt class="text-xs text-gray-500">{{ t(`conversationArchive.${field.label}`) }}</dt><dd class="mt-2 break-all font-mono text-sm text-gray-900 dark:text-gray-100">{{ field.value || t('conversationArchive.notRecorded') }}</dd></div></dl>
            <p class="text-xs leading-5 text-gray-500">{{ t('conversationArchive.modelProvenanceHint') }}</p><p v-if="detail.model_conflict" class="text-xs text-amber-700 dark:text-amber-300">{{ t('conversationArchive.modelConflict') }}</p>
          </div>
          <div class="flex items-start justify-between gap-4 border-t border-gray-100 pt-4 dark:border-gray-800"><details class="min-w-0 flex-1 text-xs text-gray-500"><summary class="cursor-pointer">{{ t('conversationArchive.raw') }}</summary><pre class="mt-3 max-h-96 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-gray-50 p-4 dark:bg-gray-900">{{ pretty(detail.request_body) }}</pre><pre class="mt-3 max-h-96 overflow-auto whitespace-pre-wrap break-all rounded-lg bg-gray-50 p-4 dark:bg-gray-900">{{ pretty(detail.response_body) }}</pre></details><button class="shrink-0 text-xs text-red-600 hover:underline" :disabled="deleting" @click="removeDetail">{{ t('conversationArchive.delete') }}</button></div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import ConversationArchiveStorage from '@/components/admin/ConversationArchiveStorage.vue'
import ConversationContext from '@/components/admin/ConversationContext.vue'
import type { ArchivePage, ArchiveRecord } from '@/api/admin/conversationArchive'
import { apiClient } from '@/api/client'
import { archiveDateRange, archiveRequestMessages, archiveResponseContext, archiveContextMessages } from '@/utils/conversationArchive'

const { t, locale } = useI18n()
const showStorage = ref(false)
const retentionDays = ref<number | null>(null)
const viewMode = ref<'groups' | 'calls'>('groups')
const detailTab = ref<'exchange' | 'context' | 'models'>('exchange')
const groupSummary = ref<ArchiveRecord | null>(null)
const groupCalls = ref<ArchiveRecord[]>([])
const callsCursor = ref(0)
const callsLoading = ref(false)
let groupVersion = 0
const filters = ref({ q: '', user_id: '', api_key_id: '', model: '', session_id: '', status: '', created_from: '', created_until: '' })
const activeFilters = ref<Record<string, string | number>>({})
const items = ref<ArchiveRecord[]>([])
const detail = ref<ArchiveRecord | null>(null)
const detailPanel = ref<HTMLElement | null>(null)
const captureEnabled = ref<boolean | null>(null)
const capturePaused = ref(false)
const loading = ref(false)
const detailLoading = ref(false)
const deleting = ref(false)
const error = ref('')
const beforeID = ref(0)
const nextCursor = ref(0)
const cursors = ref<number[]>([])
let loadVersion = 0
let detailVersion = 0
const formatTime = (value: string) => new Date(value).toLocaleString(locale?.value === 'zh' ? 'zh-CN' : undefined, { hour12: false })
const pretty = (value: unknown) => JSON.stringify(value, null, 2)
const inputMessages = computed(() => archiveRequestMessages(detail.value?.request_body).filter(message => message.role === 'user').slice(-1))
const memberCount = computed(() => new Set(items.value.map(item => item.user_id)).size)
const blockedCount = computed(() => items.value.reduce((total, item) => total + (item.blocked_count ?? (item.blocked ? 1 : 0)) + (item.auto_reply_count ?? (item.auto_reply ? 1 : 0)), 0))
const memberName = (item: ArchiveRecord) => item.user_name?.trim() || item.user_email || `${t('conversationArchive.member')} #${item.user_id ?? '—'}`
const initials = (item: ArchiveRecord) => [...memberName(item)].slice(0, 2).join('').toUpperCase()
const outputMessages = computed(() => archiveResponseContext(detail.value?.response_body))
const contextMessages = computed(() => [...archiveContextMessages(detail.value?.request_body), ...outputMessages.value])
const displayModel = (item: ArchiveRecord) => item.auto_reply ? t('conversationArchive.noModelCalled') : item.upstream_model || item.response_model || item.model || t('conversationArchive.unknownModel')
const modelFields = computed(() => [
  { label: 'requestedModel', value: detail.value?.model },
  { label: 'forwardedModel', value: detail.value?.auto_reply ? t('conversationArchive.noModelCalled') : detail.value?.upstream_model },
  { label: 'reportedModel', value: detail.value?.auto_reply ? t('conversationArchive.noModelCalled') : detail.value?.response_model },
  { label: 'clientModel', value: detail.value?.wire_model }
])
const statusLabel = (status: ArchiveRecord['status']) => t(`conversationArchive.${status === 'error' ? 'failed' : status}`)
async function load() {
  const version = ++loadVersion
  loading.value = true
  error.value = ''
  try {
    const params: Record<string, string | number> = { limit: 30, ...activeFilters.value }
    if (beforeID.value) params.before_id = beforeID.value
    const { data } = await apiClient.get<ArchivePage>(viewMode.value === 'groups' ? '/admin/conversation-archives/groups' : '/admin/conversation-archives', { params })
    if (version !== loadVersion) return
    items.value = data.items
    nextCursor.value = data.next_before_id
    captureEnabled.value = data.capture_enabled
    capturePaused.value = !!data.capture_paused
    retentionDays.value = data.retention_days ?? null
  } catch {
    if (version === loadVersion) { items.value = []; nextCursor.value = 0; closeDetail(); error.value = t('conversationArchive.loadError') }
  } finally {
    if (version === loadVersion) loading.value = false
  }
}
function closeDetail() { detail.value = null; detailVersion++; detailLoading.value = false; groupVersion++; groupCalls.value = []; groupSummary.value = null; callsCursor.value = 0; callsLoading.value = false }
function changeMode(mode: 'groups' | 'calls') { if (mode === viewMode.value) return; viewMode.value = mode; beforeID.value = 0; cursors.value = []; closeDetail(); void load() }
async function openGroup(item: ArchiveRecord) {
  groupVersion++; groupSummary.value = item; groupCalls.value = []; callsCursor.value = 0
  const work: Promise<void>[] = [openDetail(item.id)]
  if ((item.call_count ?? 1) > 1 && item.conversation_key) work.push(loadGroupCalls(false))
  await Promise.all(work)
}
async function loadGroupCalls(append: boolean) {
  if (!groupSummary.value?.conversation_key) return
  const version = groupVersion
  callsLoading.value = true
  try {
    const params: Record<string, string | number> = { conversation_key: groupSummary.value.conversation_key, limit: 30 }
    if (append && callsCursor.value) params.before_id = callsCursor.value
    const { data } = await apiClient.get<ArchivePage>('/admin/conversation-archives', { params })
    if (version !== groupVersion) return
    groupCalls.value = append ? [...groupCalls.value, ...data.items] : data.items
    callsCursor.value = data.next_before_id
  } catch { if (version === groupVersion) error.value = t('conversationArchive.loadError') }
  finally { if (version === groupVersion) callsLoading.value = false }
}
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
async function openDetail(id: number, scroll = true) {
  const version = ++detailVersion
  detailLoading.value = true
  detail.value = null
  error.value = ''
  try {
    const { data } = await apiClient.get<ArchiveRecord>(`/admin/conversation-archives/${id}`)
    if (version === detailVersion) {
      detail.value = data
      detailTab.value = 'exchange'
      await nextTick()
      if (scroll && version === detailVersion) detailPanel.value?.scrollIntoView?.({ behavior: 'smooth', block: 'start' })
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
