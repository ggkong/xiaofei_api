<template>
  <AppLayout>
    <div class="mx-auto max-w-7xl space-y-5">
      <header class="flex flex-wrap items-start justify-between gap-4">
        <div><div class="mb-2 flex items-center gap-2 text-xs font-medium uppercase tracking-widest text-primary-600 dark:text-primary-400"><span class="h-1.5 w-1.5 rounded-full bg-primary-500" />{{ t('keywordRules.eyebrow') }}</div><h1 class="text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">{{ t('keywordRules.title') }}</h1><p class="mt-2 max-w-2xl text-sm leading-6 text-gray-500">{{ t('keywordRules.description') }}</p></div>
        <div class="flex flex-wrap items-center gap-2"><button class="btn btn-secondary" :disabled="loading || saving" @click="reload">{{ t('common.refresh') }}</button><button class="btn btn-secondary" :disabled="loading" @click="exportRules">{{ t('keywordRules.export') }}</button><button class="btn btn-primary" :disabled="loading || saving || !policy || !dirty" @click="save">{{ saving ? t('common.submitting') : t('keywordRules.save') }}</button></div>
      </header>
      <p v-if="error" class="rounded-xl bg-red-50 p-4 text-sm text-red-700 dark:bg-red-950 dark:text-red-200" role="alert">{{ error }}</p>
      <p v-if="notice" class="rounded-xl bg-emerald-50 p-3 text-sm text-emerald-700 dark:bg-emerald-950 dark:text-emerald-200" role="status">{{ notice }}</p>
      <p v-if="loading" class="card p-12 text-center text-sm text-gray-400">{{ t('common.loading') }}</p>
      <template v-if="policy">
        <section class="flex flex-wrap items-center justify-between gap-4 rounded-2xl border border-primary-100 bg-white/90 p-5 dark:border-primary-900 dark:bg-gray-900">
          <div class="flex items-center gap-3"><span class="flex h-11 w-11 items-center justify-center rounded-xl bg-primary-50 text-lg font-semibold text-primary-600 dark:bg-primary-950">K</span><div><p class="font-semibold text-gray-900 dark:text-white">{{ t('keywordRules.masterSwitch') }}<span class="ml-2 rounded-full px-2 py-1 text-[10px] font-medium" :class="saved?.enabled ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300' : 'bg-gray-100 text-gray-500 dark:bg-gray-800'">{{ t(saved?.enabled ? 'keywordRules.enabled' : 'keywordRules.paused') }}</span></p><p class="mt-1 text-xs leading-5 text-gray-500">{{ dirty ? t('keywordRules.unsaved') : t('keywordRules.masterHint') }}</p></div></div>
          <label class="flex items-center gap-3 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('keywordRules.enableFeature') }}<input v-model="policy.enabled" type="checkbox" role="switch" class="h-5 w-5 accent-primary-600" :disabled="saving" /></label>
        </section>
        <p v-if="policy.legacy && policy.rules.length" class="rounded-xl bg-amber-50 p-3 text-xs leading-6 text-amber-800 dark:bg-amber-950 dark:text-amber-200">{{ t('keywordRules.legacyHint') }}</p>
        <div class="grid grid-cols-2 gap-3 lg:grid-cols-4"><div v-for="stat in stats" :key="stat.label" class="rounded-xl border border-gray-200 bg-white px-4 py-4 dark:border-gray-800 dark:bg-gray-900"><p class="text-xs text-gray-500">{{ t(`keywordRules.${stat.label}`) }}</p><p class="mt-2 text-2xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ stat.value.toLocaleString() }}</p></div></div>
        <section class="card overflow-hidden">
          <div class="space-y-3 border-b border-gray-100 p-4 dark:border-gray-800">
            <div class="flex flex-wrap items-center gap-3"><input v-model="query" class="input min-w-40 flex-1" maxlength="200" :aria-label="t('keywordRules.search')" :placeholder="t('keywordRules.searchPlaceholder')" /><select v-model="searchScope" class="input w-36" :aria-label="t('keywordRules.searchScope')"><option v-for="scope in ['all', 'name', 'keywords', 'reply', 'injection']" :key="scope" :value="scope">{{ t(`keywordRules.searchScopes.${scope}`) }}</option></select><button class="btn btn-secondary" @click="showImport = true">{{ t('keywordRules.import') }}</button><button class="btn btn-primary" @click="addRule">+ {{ t('keywordRules.newRule') }}</button></div>
            <div class="flex flex-wrap items-center gap-2"><select v-model="statusFilter" class="input min-w-28 text-xs" :aria-label="t('keywordRules.filterStatus')"><option value="">{{ t('keywordRules.allStates') }}</option><option value="enabled">{{ t('keywordRules.enabled') }}</option><option value="paused">{{ t('keywordRules.paused') }}</option></select><select v-model="actionFilter" class="input min-w-28 text-xs" :aria-label="t('keywordRules.action')"><option value="">{{ t('keywordRules.allActions') }}</option><option v-for="action in ['reply', 'inject', 'block']" :key="action" :value="action">{{ t(`keywordRules.actions.${action}`) }}</option></select><select v-model="modeFilter" class="input min-w-28 text-xs" :aria-label="t('keywordRules.matchMode')"><option value="">{{ t('keywordRules.allModes') }}</option><option v-for="mode in ['exact', 'contains', 'word', 'prefix', 'suffix', 'regex']" :key="mode" :value="mode">{{ t(`keywordRules.modes.${mode}`) }}</option></select><select v-model="categoryFilter" class="input min-w-28 text-xs" :aria-label="t('keywordRules.category')"><option value="">{{ t('keywordRules.allCategories') }}</option><option v-for="category in categories" :key="category" :value="category">{{ category }}</option></select><p class="ml-auto text-xs text-gray-400">{{ t('keywordRules.filteredCount', { count: filtered.length }) }}</p></div>
          </div>
          <div class="grid min-h-[520px] lg:grid-cols-[310px_minmax(0,1fr)]">
            <aside class="border-b border-gray-100 bg-gray-50/50 dark:border-gray-800 dark:bg-gray-900/30 lg:border-b-0 lg:border-r">
              <div class="max-h-[850px] divide-y divide-gray-100 overflow-y-auto dark:divide-gray-800"><button v-for="rule in pageRules" :key="rule.id" class="block w-full space-y-3 border-l-2 p-4 text-left transition-colors" :class="selectedID === rule.id ? 'border-primary-500 bg-primary-50/70 dark:bg-primary-950/30' : 'border-transparent hover:bg-white dark:hover:bg-gray-800'" :aria-pressed="selectedID === rule.id" @click="selectRule(rule.id)"><div class="flex items-center justify-between gap-2"><p class="min-w-0 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ rule.name || t('keywordRules.untitled') }}</p><span class="shrink-0 rounded-full px-2 py-0.5 text-[10px]" :class="rule.enabled ? 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300' : 'bg-gray-100 text-gray-400 dark:bg-gray-800'">{{ t(rule.enabled ? 'keywordRules.enabled' : 'keywordRules.paused') }}</span></div><div class="flex flex-wrap gap-1.5"><span class="rounded-md bg-white px-2 py-1 text-[10px] font-medium text-primary-700 dark:bg-gray-800 dark:text-primary-300">{{ t(`keywordRules.actions.${rule.action}`) }}</span><span class="rounded-md bg-white px-2 py-1 text-[10px] text-gray-500 dark:bg-gray-800">{{ t(`keywordRules.modes.${rule.match_mode}`) }}</span><span v-if="rule.category" class="max-w-24 truncate rounded-md bg-white px-2 py-1 text-[10px] text-gray-500 dark:bg-gray-800">{{ rule.category }}</span></div><p class="line-clamp-2 break-words text-xs leading-5 text-gray-500">{{ rule.keywords.slice(0, 4).join(' · ') || t('keywordRules.addKeywords') }}</p><p class="text-[10px] text-gray-400">{{ t('keywordRules.keywordCount', { count: rule.keywords.length }) }} · {{ t('keywordRules.priority') }} {{ rule.priority }}</p></button><p v-if="!filtered.length" class="p-8 text-center text-sm leading-6 text-gray-400">{{ policy.rules.length ? t('keywordRules.noMatches') : t('keywordRules.empty') }}</p></div>
              <div v-if="filtered.length > 20" class="flex items-center justify-between border-t border-gray-100 p-3 text-xs dark:border-gray-800"><button :disabled="page === 0" class="text-primary-600 disabled:text-gray-300" @click="page--">{{ t('conversationArchive.previous') }}</button><span class="text-gray-400">{{ page + 1 }} / {{ Math.ceil(filtered.length / 20) }}</span><button :disabled="(page + 1) * 20 >= filtered.length" class="text-primary-600 disabled:text-gray-300" @click="page++">{{ t('conversationArchive.next') }}</button></div>
            </aside>
            <div ref="editorPanel" class="min-w-0 scroll-mt-8"><template v-if="selectedRule"><div class="flex justify-end gap-4 border-b border-gray-100 px-5 py-2 text-xs dark:border-gray-800"><button class="text-gray-500 hover:text-primary-600" @click="duplicateRule">{{ t('keywordRules.duplicate') }}</button><button class="text-primary-600 hover:underline" @click="jumpToTest">{{ t('keywordRules.testThisRule') }}</button></div><KeywordRuleEditor v-model="selectedRule" :groups="groups" @import="showImport = true" @delete="deleteRule" /></template><div v-else class="flex h-full min-h-96 flex-col items-center justify-center gap-4 px-5 text-center"><span class="flex h-14 w-14 items-center justify-center rounded-2xl bg-primary-50 text-2xl text-primary-500 dark:bg-primary-950">+</span><p class="max-w-md text-sm leading-7 text-gray-500">{{ t('keywordRules.emptyHint') }}</p><button class="btn btn-primary" @click="addRule">{{ t('keywordRules.newRule') }}</button></div></div>
          </div>
          <div class="flex flex-wrap items-center justify-between gap-3 border-t border-gray-100 px-5 py-3 dark:border-gray-800"><p class="text-xs text-gray-400">{{ dirty ? t('keywordRules.unsaved') : t('keywordRules.orderHint') }}</p><button class="btn btn-primary" :disabled="saving || !dirty" @click="save">{{ saving ? t('common.submitting') : t('keywordRules.save') }}</button></div>
        </section>
        <div ref="testPanel" class="scroll-mt-8"><KeywordRuleTester :policy="policy" :selected-rule-id="selectedID" /></div>
        <p class="text-center text-xs text-gray-400"><a href="/admin/conversation-archives" class="text-primary-600 hover:underline">{{ t('keywordRules.viewConversations') }}</a> · {{ t('keywordRules.recordHint') }}</p>
      </template>
      <KeywordImportDialog v-if="showImport" :target-name="selectedRule?.name" :preserve-whitespace="selectedRule?.match_mode === 'regex'" @close="showImport = false" @apply="applyImport" />
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import KeywordRuleEditor from '@/components/keyword-rules/KeywordRuleEditor.vue'
import KeywordImportDialog from '@/components/keyword-rules/KeywordImportDialog.vue'
import KeywordRuleTester from '@/components/keyword-rules/KeywordRuleTester.vue'
import { apiClient } from '@/api/client'
import { getKeywordRules, saveKeywordRules, type KeywordPolicy, type KeywordRule } from '@/api/admin/keywordRules'
import { copyPolicy, createKeywordRule, deduplicateKeywords, ruleID, validateKeywordPolicy, type KeywordImport } from '@/utils/keywordRules'
const { t } = useI18n()
const policy = ref<KeywordPolicy | null>(null), saved = ref<KeywordPolicy | null>(null), selectedID = ref(''), groups = ref<{ id: number; name: string }[]>([])
const loading = ref(true), saving = ref(false), error = ref(''), notice = ref(''), showImport = ref(false)
const query = ref(''), searchScope = ref('all'), actionFilter = ref(''), modeFilter = ref(''), statusFilter = ref(''), categoryFilter = ref(''), page = ref(0)
const editorPanel = ref<HTMLElement | null>(null), testPanel = ref<HTMLElement | null>(null)
let loadVersion = 0
const dirty = computed(() => !!policy.value && JSON.stringify(policy.value) !== JSON.stringify(saved.value))
const selectedRule = computed<KeywordRule | null>({ get: () => policy.value?.rules.find(rule => rule.id === selectedID.value) ?? null, set: value => { if (value && policy.value) { const index = policy.value.rules.findIndex(rule => rule.id === value.id); if (index >= 0) policy.value.rules[index] = value } } })
const categories = computed(() => [...new Set(policy.value?.rules.map(rule => rule.category).filter(Boolean) ?? [])].sort())
const stats = computed(() => [{ label: 'totalRules', value: policy.value?.rules.length ?? 0 }, { label: 'totalKeywords', value: policy.value?.rules.reduce((sum, rule) => sum + rule.keywords.length, 0) ?? 0 }, { label: 'replyRules', value: policy.value?.rules.filter(rule => rule.action === 'reply').length ?? 0 }, { label: 'injectionRules', value: policy.value?.rules.filter(rule => rule.action === 'inject').length ?? 0 }])
const filtered = computed(() => (policy.value?.rules ?? []).filter(rule => {
  if ((actionFilter.value && rule.action !== actionFilter.value) || (modeFilter.value && rule.match_mode !== modeFilter.value) || (categoryFilter.value && rule.category !== categoryFilter.value) || (statusFilter.value && rule.enabled !== (statusFilter.value === 'enabled'))) return false
  const fields: Record<string, string> = { name: rule.name, keywords: rule.keywords.join('\n'), reply: rule.reply, injection: rule.injection }
  const text = (searchScope.value === 'all' ? Object.values(fields).join('\n') + '\n' + rule.category : fields[searchScope.value] ?? '').toLowerCase()
  return query.value.toLowerCase().trim().split(/\s+/).filter(Boolean).every(term => text.includes(term))
}).sort((a, b) => a.priority - b.priority))
const pageRules = computed(() => filtered.value.slice(page.value * 20, page.value * 20 + 20))
watch([query, searchScope, actionFilter, modeFilter, statusFilter, categoryFilter], () => { page.value = 0 })
async function load() {
  const version = ++loadVersion; loading.value = true; error.value = ''
  try {
    const data = await getKeywordRules()
    if (version !== loadVersion) return
    if (data.legacy) for (const rule of data.rules) if (rule.id === 'legacy-keywords' && rule.name === 'Imported keyword rule') rule.name = t('keywordRules.legacyRuleName')
    policy.value = data; saved.value = copyPolicy(data)
    selectedID.value = data.rules[0]?.id ?? ''
  } catch { if (version === loadVersion) error.value = t('keywordRules.loadError') }
  finally { if (version === loadVersion) loading.value = false }
}
function reload() { if (!dirty.value || window.confirm(t('keywordRules.discardConfirm'))) { notice.value = ''; void load() } }
async function save() {
  if (!policy.value || saving.value) return
  const invalid = validateKeywordPolicy(policy.value)
  if (invalid) { error.value = t(`keywordRules.${invalid}`); return }
  const submitted = copyPolicy(policy.value), serialized = JSON.stringify(submitted)
  saving.value = true; error.value = ''; notice.value = ''
  try {
    const data = await saveKeywordRules(submitted)
    saved.value = copyPolicy(data)
    if (JSON.stringify(policy.value) === serialized) policy.value = data
    else { policy.value.revision = data.revision; policy.value.legacy = false }
    notice.value = t('keywordRules.saved')
  } catch (failure) {
    const err = failure as { response?: { status?: number; data?: { message?: string } } }
    error.value = err.response?.status === 409 ? t('keywordRules.conflict') : err.response?.data?.message || t('keywordRules.saveError')
  } finally { saving.value = false }
}
function addRule() {
  if (!policy.value) return
  if (policy.value.rules.length >= 500) { error.value = t('keywordRules.capacityError'); return }
  const rule = createKeywordRule(t('keywordRules.defaultName', { count: policy.value.rules.length + 1 }))
  rule.priority = Math.min(100000, Math.max(0, ...policy.value.rules.map(item => item.priority)) + 10)
  policy.value.rules.push(rule); selectedID.value = rule.id; notice.value = ''; void scrollEditor()
}
function selectRule(id: string) { selectedID.value = id; void scrollEditor() }
async function scrollEditor() { await nextTick(); if (window.innerWidth < 1024) editorPanel.value?.scrollIntoView?.({ behavior: 'smooth', block: 'start' }) }
function duplicateRule() { if (!selectedRule.value || !policy.value) return; const copy: KeywordRule = JSON.parse(JSON.stringify(selectedRule.value)); copy.id = ruleID(); copy.name = t('keywordRules.copyName', { name: copy.name }).slice(0, 100); policy.value.rules.push(copy); selectedID.value = copy.id }
function deleteRule() { if (!selectedRule.value || !policy.value || !window.confirm(t('keywordRules.deleteConfirm', { name: selectedRule.value.name }))) return; policy.value.rules = policy.value.rules.filter(rule => rule.id !== selectedID.value); selectedID.value = policy.value.rules[0]?.id ?? '' }
function jumpToTest() { testPanel.value?.scrollIntoView?.({ behavior: 'smooth', block: 'start' }) }
function applyImport(value: KeywordImport & { replace: boolean }) {
  if (!policy.value) return
  if (value.rules.length) {
    const ids = new Set(value.replace ? [] : policy.value.rules.map(rule => rule.id))
    const rules = value.rules.map(rule => { while (ids.has(rule.id)) rule.id = ruleID(); ids.add(rule.id); return rule })
    if (!value.replace && rules.length + policy.value.rules.length > 500 || rules.length > 500) { error.value = t('keywordRules.capacityError'); return }
    policy.value.rules = value.replace ? rules : [...policy.value.rules, ...rules]; selectedID.value = rules[0].id
  } else if (value.keywords.length) {
    if (!selectedRule.value) { addRule(); const created = policy.value.rules.find(rule => rule.id === selectedID.value); if (created) created.name = t('keywordRules.newImportRule') }
    if (selectedRule.value) selectedRule.value.keywords = deduplicateKeywords([...selectedRule.value.keywords, ...value.keywords], selectedRule.value.case_sensitive || selectedRule.value.match_mode === 'regex', selectedRule.value.match_mode === 'regex')
  }
  showImport.value = false; notice.value = t('keywordRules.imported'); void scrollEditor()
}
function exportRules() {
  if (!policy.value) return
  const blob = new Blob([JSON.stringify({ version: 1, enabled: policy.value.enabled, rules: policy.value.rules }, null, 2)], { type: 'application/json;charset=utf-8' })
  const url = URL.createObjectURL(blob), link = document.createElement('a')
  link.href = url; link.download = `keyword-rules-${new Date().toISOString().slice(0, 10)}.json`; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000)
}
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value) { event.preventDefault(); event.returnValue = '' } }
onBeforeRouteLeave(() => !dirty.value || window.confirm(t('keywordRules.discardConfirm')))
onMounted(() => { void load(); void apiClient.get<{ id: number; name: string }[]>('/admin/groups/all').then(({ data }) => { groups.value = data }).catch(() => {}); window.addEventListener('beforeunload', beforeUnload) })
onBeforeUnmount(() => { loadVersion++; window.removeEventListener('beforeunload', beforeUnload) })
</script>
