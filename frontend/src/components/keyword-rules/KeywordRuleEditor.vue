<template>
  <section class="min-w-0" :aria-label="t('keywordRules.editor')">
    <div class="flex flex-wrap items-start justify-between gap-3 border-b border-gray-100 p-5 dark:border-gray-800">
      <div><p class="text-xs font-medium text-primary-600">{{ t('keywordRules.editor') }}</p><h2 class="mt-1 text-lg font-semibold text-gray-950 dark:text-white">{{ model.name || t('keywordRules.untitled') }}</h2></div>
      <label class="flex items-center gap-2 text-sm text-gray-600 dark:text-gray-300"><input v-model="model.enabled" type="checkbox" role="switch" class="h-4 w-4 accent-primary-600" />{{ t('keywordRules.ruleEnabled') }}</label>
    </div>
    <div class="space-y-6 p-5">
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('keywordRules.name') }}<input v-model="model.name" class="input mt-2 w-full" maxlength="100" :placeholder="t('keywordRules.namePlaceholder')" /></label>
        <label class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('keywordRules.category') }}<input v-model="model.category" class="input mt-2 w-full" maxlength="60" :placeholder="t('keywordRules.categoryPlaceholder')" /></label>
      </div>
      <div>
        <div class="mb-2 flex flex-wrap items-center justify-between gap-2"><label for="rule-keywords" class="text-sm font-semibold text-gray-700 dark:text-gray-200">{{ t('keywordRules.keywords') }} <span class="ml-1 text-xs font-normal text-gray-400">{{ model.keywords.length.toLocaleString() }}</span></label><button type="button" class="text-xs font-medium text-primary-600 hover:underline" @click="$emit('import')">{{ t('keywordRules.bulkAdd') }}</button></div>
        <textarea id="rule-keywords" v-model="keywordText" class="input min-h-36 w-full resize-y font-mono text-sm leading-6" :placeholder="t('keywordRules.keywordsPlaceholder')" />
        <details v-if="multilineKeywords.length" class="mt-2 text-xs text-gray-500"><summary class="cursor-pointer">{{ t('keywordRules.multilineKeywords', { count: multilineKeywords.length }) }}</summary><div v-for="(word, index) in multilineKeywords" :key="index" class="mt-2 flex items-start gap-2"><pre class="min-w-0 flex-1 whitespace-pre-wrap break-words rounded-lg bg-gray-50 p-3 dark:bg-gray-900">{{ word }}</pre><button class="text-red-600" @click="removeMultiline(word)">{{ t('common.delete') }}</button></div></details>
        <p class="mt-2 text-xs text-gray-400">{{ t('keywordRules.keywordCapacity') }}</p>
      </div>
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('keywordRules.matchMode') }}<select v-model="model.match_mode" class="input mt-2 w-full"><option v-for="mode in modes" :key="mode" :value="mode">{{ t(`keywordRules.modes.${mode}`) }}</option></select></label>
        <label class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('keywordRules.keywordLogic') }}<select v-model="model.keyword_logic" class="input mt-2 w-full"><option value="any">{{ t('keywordRules.any') }}</option><option value="all">{{ t('keywordRules.every') }}</option></select></label>
      </div>
      <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl bg-gray-50 p-3 text-xs dark:bg-gray-900"><p class="max-w-lg leading-5 text-gray-500">{{ t(`keywordRules.modeHints.${model.match_mode}`) }}</p><label class="flex shrink-0 items-center gap-2 font-medium text-gray-600 dark:text-gray-300"><input v-model="model.case_sensitive" type="checkbox" class="accent-primary-600" />{{ t('keywordRules.caseSensitive') }}</label></div>
      <div>
        <h3 class="mb-3 text-sm font-semibold text-gray-700 dark:text-gray-200">{{ t('keywordRules.action') }}</h3>
        <div class="grid grid-cols-3 gap-2"><button v-for="action in actions" :key="action" type="button" class="rounded-xl border px-2 py-3 text-sm font-medium transition-colors" :class="model.action === action ? 'border-primary-400 bg-primary-50 text-primary-700 dark:border-primary-700 dark:bg-primary-950 dark:text-primary-300' : 'border-gray-200 text-gray-500 hover:border-primary-300 dark:border-gray-700'" :aria-pressed="model.action === action" @click="chooseAction(action)">{{ t(`keywordRules.actions.${action}`) }}</button></div>
        <label v-if="model.action !== 'inject'" class="mt-4 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t(model.action === 'reply' ? 'keywordRules.reply' : 'keywordRules.blockMessage') }}<textarea v-model="model.reply" class="input mt-2 min-h-28 w-full resize-y text-sm leading-6" maxlength="8000" :placeholder="t('keywordRules.replyPlaceholder')" /></label>
        <template v-else>
          <label class="mt-4 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('keywordRules.injection') }}<textarea v-model="model.injection" class="input mt-2 min-h-36 w-full resize-y text-sm leading-6" maxlength="16000" :placeholder="t('keywordRules.injectionPlaceholder')" /></label>
          <div class="mt-3 flex flex-wrap gap-4 text-xs text-gray-600 dark:text-gray-300"><label class="flex items-center gap-2"><input v-model="model.injection_position" type="radio" value="prepend" class="accent-primary-600" />{{ t('keywordRules.prepend') }}</label><label class="flex items-center gap-2"><input v-model="model.injection_position" type="radio" value="append" class="accent-primary-600" />{{ t('keywordRules.append') }}</label></div>
          <label class="mt-3 flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300"><input v-model="model.stop_on_match" type="checkbox" class="accent-primary-600" />{{ t('keywordRules.stopAfterInjection') }}</label>
        </template>
        <p class="mt-2 text-xs leading-5 text-gray-400">{{ t(`keywordRules.actionHints.${model.action}`) }}</p>
      </div>
      <details class="rounded-xl border border-gray-200 dark:border-gray-800">
        <summary class="cursor-pointer px-4 py-3 text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('keywordRules.scopeAndExclusions') }}</summary>
        <div class="space-y-4 border-t border-gray-100 p-4 dark:border-gray-800">
          <label class="block text-xs font-medium text-gray-500">{{ t('keywordRules.scanScope') }}<select v-model="model.scan_scope" class="input mt-2 w-full"><option v-for="scope in ['latest_user', 'all_user', 'context']" :key="scope" :value="scope">{{ t(`keywordRules.scopes.${scope}`) }}</option></select><span class="mt-2 block font-normal leading-5">{{ t('keywordRules.historyHint') }}</span></label>
          <label class="block text-xs font-medium text-gray-500">{{ t('keywordRules.excludeKeywords') }}<textarea v-model="excludeText" class="input mt-2 min-h-20 w-full resize-y font-mono text-xs" :placeholder="t('keywordRules.excludePlaceholder')" /></label>
          <label class="block text-xs font-medium text-gray-500">{{ t('keywordRules.groups') }}<select v-model="model.group_ids" multiple class="input mt-2 min-h-24 w-full"><option v-for="group in scopeGroups" :key="group.id" :value="group.id">{{ group.name }}</option></select><span class="mt-1 block font-normal">{{ t('keywordRules.emptyScope') }}</span></label>
          <div class="grid gap-3 sm:grid-cols-2"><label class="text-xs font-medium text-gray-500">{{ t('keywordRules.models') }}<textarea v-model="modelText" class="input mt-2 min-h-20 w-full resize-y font-mono text-xs" :placeholder="t('keywordRules.modelsPlaceholder')" /></label><label class="text-xs font-medium text-gray-500">{{ t('keywordRules.excludeModels') }}<textarea v-model="excludeModelText" class="input mt-2 min-h-20 w-full resize-y font-mono text-xs" /></label></div>
          <div class="grid gap-3 sm:grid-cols-2"><label class="text-xs font-medium text-gray-500">{{ t('keywordRules.users') }}<input v-model="userIDText" class="input mt-2 w-full" :placeholder="t('keywordRules.idsPlaceholder')" /></label><label class="text-xs font-medium text-gray-500">{{ t('keywordRules.excludeUsers') }}<input v-model="excludeUserIDText" class="input mt-2 w-full" :placeholder="t('keywordRules.idsPlaceholder')" /></label></div>
          <div><p class="text-xs font-medium text-gray-500">{{ t('keywordRules.protocols') }}</p><div class="mt-2 flex flex-wrap gap-3"><label v-for="protocol in protocols" :key="protocol" class="flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300"><input v-model="model.protocols" type="checkbox" :value="protocol" class="accent-primary-600" />{{ t(`keywordRules.protocolNames.${protocol}`) }}</label></div><p class="mt-2 text-xs text-gray-400">{{ t('keywordRules.emptyScope') }}</p></div>
        </div>
      </details>
      <div class="flex flex-wrap items-end justify-between gap-4 border-t border-gray-100 pt-4 dark:border-gray-800"><label class="text-xs font-medium text-gray-500">{{ t('keywordRules.priority') }}<input v-model.number="model.priority" class="input mt-2 block w-28" type="number" min="0" max="100000" /></label><button type="button" class="text-xs text-red-600 hover:underline" @click="$emit('delete')">{{ t('keywordRules.deleteRule') }}</button><p class="w-full text-xs leading-5 text-gray-400">{{ t('keywordRules.priorityHint') }}</p></div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { KeywordRule, KeywordMatchMode, KeywordAction, KeywordProtocol } from '@/api/admin/keywordRules'
import { splitKeywords } from '@/utils/keywordRules'
const model = defineModel<KeywordRule>({ required: true })
const props = defineProps<{ groups: { id: number; name: string }[] }>()
defineEmits<{ import: []; delete: [] }>()
const { t } = useI18n()
const modes: KeywordMatchMode[] = ['exact', 'contains', 'word', 'prefix', 'suffix', 'regex']
const actions: KeywordAction[] = ['reply', 'inject', 'block']
const protocols: KeywordProtocol[] = ['openai_chat_completions', 'openai_responses', 'anthropic_messages', 'gemini']
const scopeGroups = computed(() => [...props.groups, ...model.value.group_ids.filter(id => !props.groups.some(group => group.id === id)).map(id => ({ id, name: `#${id}` }))])
const multilineKeywords = computed(() => model.value.keywords.filter(word => /[\r\n]/.test(word)))
function lines(field: 'keywords' | 'exclude_keywords' | 'models' | 'exclude_models') {
  const text = ref(model.value[field].filter(value => !/[\r\n]/.test(value)).join('\n'))
  watch(() => [model.value.id, model.value[field]] as const, () => {
    const expected = model.value[field].filter(value => !/[\r\n]/.test(value))
    if (JSON.stringify(splitKeywords(text.value, field === 'keywords' && model.value.match_mode === 'regex')) !== JSON.stringify(expected)) text.value = expected.join('\n')
  })
  watch(text, value => {
    const values = [...splitKeywords(value, field === 'keywords' && model.value.match_mode === 'regex'), ...model.value[field].filter(word => /[\r\n]/.test(word))]
    if (JSON.stringify(values) !== JSON.stringify(model.value[field])) model.value = { ...model.value, [field]: values }
  })
  return text
}
const keywordText = lines('keywords'), excludeText = lines('exclude_keywords'), modelText = lines('models'), excludeModelText = lines('exclude_models')
function removeMultiline(word: string) { model.value = { ...model.value, keywords: model.value.keywords.filter(value => value !== word) } }
function chooseAction(action: KeywordAction) { model.value.action = action; if (action === 'block' && !model.value.reply) model.value.reply = t('keywordRules.defaultBlock') }
function ids(field: 'user_ids' | 'exclude_user_ids') {
 const parse = (value: string) => value.split(/[,，\s]+/).filter(Boolean).map(Number)
 const text = ref(model.value[field].join(', '))
 watch(() => [model.value.id, model.value[field]] as const, () => { if (JSON.stringify(parse(text.value)) !== JSON.stringify(model.value[field])) text.value = model.value[field].join(', ') })
 watch(text, value => { const parsed = parse(value); if (JSON.stringify(parsed) !== JSON.stringify(model.value[field])) model.value = { ...model.value, [field]: parsed } })
 return text
}
const userIDText = ids('user_ids'), excludeUserIDText = ids('exclude_user_ids')
</script>
