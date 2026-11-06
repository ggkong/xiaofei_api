<template>
  <BaseDialog :show="true" :title="t('keywordRules.aiTitle')" width="extra-wide" @close="emit('close')">
    <div class="space-y-5">
      <div class="rounded-xl border border-primary-100 bg-primary-50/50 p-4 dark:border-primary-900 dark:bg-primary-950/20">
        <p class="text-sm leading-6 text-gray-700 dark:text-gray-200">{{ t('keywordRules.aiIntro') }}</p>
        <p class="mt-2 text-xs leading-5 text-gray-500">{{ t('keywordRules.aiCostHint') }}</p>
      </div>
      <p v-if="loadingKeys" class="text-sm text-gray-500">{{ t('common.loading') }}</p>
      <div v-else-if="!keys.length" class="rounded-xl bg-amber-50 p-4 text-sm leading-6 text-amber-800 dark:bg-amber-950 dark:text-amber-200">
        {{ t('keywordRules.aiNoKeys') }} <a href="/keys" target="_blank" rel="noopener" class="font-medium underline">{{ t('keywordRules.aiManageKeys') }}</a>
        <button class="ml-3 underline" @click="loadKeys">{{ t('common.refresh') }}</button>
      </div>
      <form class="space-y-4" @submit.prevent="generate">
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="text-xs font-medium text-gray-500">{{ t('keywordRules.aiKey') }}<select v-model="keyID" class="input mt-2 w-full" :disabled="loadingKeys"><option :value="0">{{ t('keywordRules.aiChooseKey') }}</option><option v-for="key in keys" :key="key.id" :value="key.id">{{ key.name }}{{ key.group?.name ? ` · ${key.group.name}` : '' }}</option></select></label>
          <label class="text-xs font-medium text-gray-500">{{ t('keywordRules.aiModel') }}<input v-model.trim="model" class="input mt-2 w-full" list="keyword-ai-models" maxlength="255" :placeholder="loadingModels ? t('common.loading') : t('keywordRules.aiModelPlaceholder')" /><datalist id="keyword-ai-models"><option v-for="name in models" :key="name" :value="name" /></datalist></label>
        </div>
        <p v-if="modelError" class="text-xs text-amber-700 dark:text-amber-300">{{ modelError }}</p>
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="text-xs font-medium text-gray-500">{{ t('keywordRules.aiTask') }}<select v-model="task" class="input mt-2 w-full"><option value="new">{{ t('keywordRules.aiNewRules') }}</option><option v-if="target" value="expand">{{ t('keywordRules.aiExpandRule', { name: target.name }) }}</option></select></label>
          <label v-if="task === 'new'" class="text-xs font-medium text-gray-500">{{ t('keywordRules.action') }}<select v-model="action" class="input mt-2 w-full"><option v-for="value in actions" :key="value" :value="value">{{ t(`keywordRules.actions.${value}`) }}</option></select></label>
          <p v-else class="self-end rounded-lg bg-gray-50 p-3 text-xs leading-5 text-gray-500 dark:bg-gray-900">{{ t('keywordRules.aiExpandHint') }}</p>
        </div>
        <label class="block text-xs font-medium text-gray-500">{{ t('keywordRules.aiGoal') }}<textarea v-model="goal" class="input mt-2 min-h-28 w-full resize-y text-sm leading-6" maxlength="3000" :placeholder="t('keywordRules.aiGoalPlaceholder')" /></label>
        <label v-if="task === 'new'" class="block text-xs font-medium text-gray-500">{{ t(action === 'inject' ? 'keywordRules.aiDesiredInjection' : 'keywordRules.aiDesiredReply') }}<textarea v-model="content" class="input mt-2 min-h-20 w-full resize-y text-sm leading-6" :maxlength="action === 'inject' ? 16000 : 8000" :placeholder="t('keywordRules.aiContentPlaceholder')" /></label>
        <div class="flex flex-wrap items-end gap-4">
          <label v-if="task === 'new'" class="min-w-40 flex-1 text-xs font-medium text-gray-500">{{ t('keywordRules.matchMode') }}<select v-model="matchMode" class="input mt-2 w-full"><option v-for="value in modes" :key="value" :value="value">{{ t(`keywordRules.modes.${value}`) }}</option></select></label>
          <label v-if="task === 'new'" class="text-xs font-medium text-gray-500">{{ t('keywordRules.aiCount') }}<input v-model.number="count" class="input mt-2 block w-24" type="number" min="1" max="5" /></label>
          <button type="submit" class="btn btn-primary" :disabled="loading || !selectedKey || !goal.trim() || !model.trim() || !validCount">{{ loading ? t('keywordRules.aiGenerating') : t('keywordRules.aiGenerate') }}</button>
          <button v-if="loading" type="button" class="btn btn-secondary" @click="cancel">{{ t('keywordRules.aiCancelGeneration') }}</button>
        </div>
        <p class="text-xs leading-5 text-gray-400">{{ t('keywordRules.aiDataHint') }}</p>
      </form>
      <p v-if="error" class="rounded-xl bg-red-50 p-3 text-sm leading-6 text-red-700 dark:bg-red-950 dark:text-red-200" role="alert">{{ error }}</p>
      <section v-if="result" class="space-y-4 border-t border-gray-100 pt-5 dark:border-gray-800" aria-live="polite">
        <div><h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('keywordRules.aiSuggestions') }}</h4><p class="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-gray-600 dark:text-gray-300">{{ result.summary }}</p><p class="mt-2 text-xs text-gray-400">{{ t('keywordRules.aiModelUsed', { model: result.model }) }} · {{ t('keywordRules.aiExampleHint') }}</p></div>
        <article v-for="(suggestion, index) in result.suggestions" :key="suggestion.rule.id" class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-gray-700">
          <label class="flex items-start gap-3"><input v-model="selected[index]" type="checkbox" class="mt-1 accent-primary-600" :disabled="!verified[index] || loading" /><span class="min-w-0"><span class="font-semibold text-gray-900 dark:text-white">{{ suggestion.rule.name }}</span><span class="ml-2 text-xs text-gray-500">{{ t(`keywordRules.modes.${suggestion.rule.match_mode}`) }} · {{ t(`keywordRules.actions.${suggestion.rule.action}`) }}</span></span></label>
          <div class="flex flex-wrap gap-2"><span v-for="word in suggestion.keywords" :key="word" class="max-w-full whitespace-pre-wrap break-all rounded-md bg-primary-50 px-2 py-1 font-mono text-xs text-primary-700 dark:bg-primary-950 dark:text-primary-300">{{ word }}</span></div>
          <p class="whitespace-pre-wrap break-words text-sm leading-6 text-gray-600 dark:text-gray-300">{{ suggestion.reason }}</p>
          <div class="rounded-lg bg-gray-50 p-3 dark:bg-gray-900"><p class="text-xs font-medium text-gray-400">{{ t(suggestion.rule.action === 'inject' ? 'keywordRules.injection' : 'keywordRules.reply') }}</p><p class="mt-1 whitespace-pre-wrap break-words text-sm leading-6 text-gray-700 dark:text-gray-200">{{ suggestion.rule.action === 'inject' ? suggestion.rule.injection : suggestion.rule.reply }}</p></div>
          <div v-for="(example, exampleIndex) in suggestion.examples" :key="exampleIndex" class="rounded-lg border border-gray-100 p-3 text-xs leading-5 dark:border-gray-800">
            <div class="flex flex-wrap gap-x-4 gap-y-1"><span class="text-gray-500">{{ t(example.should_match ? 'keywordRules.aiExpectedHit' : 'keywordRules.aiExpectedMiss') }}</span><span class="font-medium" :class="checks[index]?.[exampleIndex] === example.should_match ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-600'">{{ checks[index]?.[exampleIndex] === null || checks[index]?.[exampleIndex] === undefined ? t(loading ? 'keywordRules.aiChecking' : 'keywordRules.aiCheckFailed') : t(checks[index][exampleIndex] ? 'keywordRules.aiActualHit' : 'keywordRules.aiActualMiss') }}</span></div>
            <p class="mt-1 whitespace-pre-wrap break-words font-mono text-gray-800 dark:text-gray-100">{{ example.text }}</p><p class="mt-1 break-words text-gray-500">{{ example.explanation }}</p>
          </div>
          <p v-if="!loading && !verified[index]" class="text-xs leading-5 text-amber-700 dark:text-amber-300">{{ t('keywordRules.aiExampleMismatch') }}</p>
        </article>
      </section>
    </div>
    <template #footer><p class="mr-auto text-xs leading-5 text-gray-500">{{ t(task === 'new' ? 'keywordRules.aiDraftHint' : 'keywordRules.aiExpandDraftHint') }}</p><button class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="loading || !selectionCount" @click="apply">{{ t('keywordRules.aiApply', { count: selectionCount }) }}</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { keysAPI } from '@/api/keys'
import { generateKeywordAI, keywordAIModels } from '@/api/admin/keywordAI'
import { testKeywordRules, type KeywordAction, type KeywordMatchMode, type KeywordPolicy, type KeywordRule } from '@/api/admin/keywordRules'
import { KeywordAIError, keywordAIExampleRule, type KeywordAIBrief, type KeywordAIResult, type KeywordAISelection } from '@/utils/keywordAI'
import type { ApiKey } from '@/types'

const props = defineProps<{ target: KeywordRule | null }>()
const emit = defineEmits<{ close: []; apply: [selection: KeywordAISelection] }>()
const { t, locale } = useI18n()
const keys = ref<ApiKey[]>([]), keyID = ref(0), loadingKeys = ref(false)
const models = ref<string[]>([]), model = ref(''), loadingModels = ref(false), modelError = ref('')
const task = ref<'new' | 'expand'>('new'), action = ref<KeywordAction>('reply'), matchMode = ref<KeywordMatchMode>('exact'), count = ref(3)
const goal = ref(''), content = ref(''), loading = ref(false), error = ref('')
const result = ref<(KeywordAIResult & { model: string }) | null>(null), selected = ref<boolean[]>([]), verified = ref<boolean[]>([]), checks = ref<(boolean | null)[][]>([])
const actions: KeywordAction[] = ['reply', 'inject', 'block'], modes: KeywordMatchMode[] = ['exact', 'contains', 'word', 'prefix', 'suffix', 'regex']
const selectedKey = computed(() => keys.value.find(key => key.id === keyID.value))
const selectionCount = computed(() => selected.value.filter((value, index) => value && verified.value[index]).length)
const validCount = computed(() => task.value === 'expand' || Number.isInteger(count.value) && count.value >= 1 && count.value <= 5)
let generation = 0, modelsVersion = 0, keysVersion = 0, controller: AbortController | undefined, modelsController: AbortController | undefined, keysController: AbortController | undefined
let targetSnapshot = ''
function cancel() { generation++; controller?.abort(); loading.value = false }
function invalidate() { cancel(); result.value = null; selected.value = []; verified.value = []; checks.value = []; error.value = '' }
watch([task, action, matchMode, count, goal, content, keyID, model, () => JSON.stringify(props.target)], invalidate)
watch(keyID, () => { void loadModels() })
async function loadKeys() {
  const version = ++keysVersion; loadingKeys.value = true; keysController?.abort(); keysController = new AbortController()
  try {
    const data = await keysAPI.list(1, 100, { status: 'active' }, { signal: keysController.signal })
    if (version !== keysVersion) return
    keys.value = data.items.filter(key => key.status === 'active' && key.group_id && (!key.expires_at || Date.parse(key.expires_at) > Date.now()) && (!key.quota || key.quota_used < key.quota))
    if (!keys.value.some(key => key.id === keyID.value)) keyID.value = keys.value[0]?.id ?? 0
  } catch { if (version === keysVersion) error.value = t('keywordRules.aiKeysError') }
  finally { if (version === keysVersion) loadingKeys.value = false }
}
async function loadModels() {
  const version = ++modelsVersion; modelsController?.abort(); models.value = []; model.value = ''; modelError.value = ''
  if (!selectedKey.value) { loadingModels.value = false; return }
  modelsController = new AbortController(); const current = modelsController
  const timer = setTimeout(() => current.abort(), 15000); loadingModels.value = true
  try {
    const values = await keywordAIModels(selectedKey.value.key, current.signal)
    if (version !== modelsVersion) return
    models.value = values
    if (!model.value) model.value = values.find(name => !/image|video|audio|tts|whisper|embedding|realtime|rerank/i.test(name)) ?? ''
  } catch { if (version === modelsVersion) modelError.value = t('keywordRules.aiModelsHint') }
  finally { clearTimeout(timer); if (version === modelsVersion) loadingModels.value = false }
}
async function generate() {
  if (loading.value || !selectedKey.value || !goal.value.trim() || !model.value || !validCount.value) return
  invalidate(); const version = ++generation; controller = new AbortController(); const current = controller
  const timer = setTimeout(() => current.abort(), 120000); loading.value = true
  const target = task.value === 'expand' && props.target ? JSON.parse(JSON.stringify(props.target)) as KeywordRule : undefined
  targetSnapshot = target ? JSON.stringify(target) : ''
  const brief: KeywordAIBrief = { goal: goal.value, content: content.value, action: action.value, matchMode: matchMode.value, count: count.value, language: locale.value, target }
  try {
    const data = await generateKeywordAI(selectedKey.value.key, model.value, brief, current.signal)
    if (version !== generation) return
    result.value = data; checks.value = data.suggestions.map(() => [null, null]); verified.value = data.suggestions.map(() => false); selected.value = data.suggestions.map(() => false)
    await Promise.all(data.suggestions.map(async (suggestion, index) => {
      const rule = keywordAIExampleRule(suggestion.rule)
      const policy: KeywordPolicy = { version: 1, enabled: false, legacy: false, revision: '', rules: [rule] }
      const outcomes = await Promise.allSettled(suggestion.examples.map(example => testKeywordRules({ policy, text: example.text, protocol: 'openai_chat_completions', model: '', user_id: 0, group_id: 0 })))
      if (version !== generation) return
      checks.value[index] = outcomes.map(outcome => outcome.status === 'fulfilled' ? outcome.value.hits.length > 0 : null)
      verified.value[index] = outcomes.every((outcome, exampleIndex) => outcome.status === 'fulfilled' && (outcome.value.hits.length > 0) === suggestion.examples[exampleIndex].should_match)
      selected.value[index] = verified.value[index]
    }))
  } catch (failure) {
    if (version === generation) error.value = t(`keywordRules.${failure instanceof KeywordAIError ? failure.key : current.signal.aborted ? 'aiTimeout' : 'aiRequestError'}`)
  } finally { clearTimeout(timer); if (version === generation) loading.value = false }
}
function apply() {
  if (!result.value || loading.value || !selectionCount.value) return
  const suggestions = result.value.suggestions.filter((_, index) => selected.value[index] && verified.value[index])
  if (task.value === 'expand' && props.target) emit('apply', { rules: [], targetId: props.target.id, targetSnapshot, keywords: suggestions.flatMap(value => value.keywords), description: suggestions[0]?.reason })
  else emit('apply', { rules: suggestions.map(value => value.rule) })
}
onMounted(() => { void loadKeys() })
onBeforeUnmount(() => { cancel(); modelsVersion++; keysVersion++; modelsController?.abort(); keysController?.abort() })
</script>
