<template>
  <section class="card overflow-hidden" aria-labelledby="keyword-test-title">
    <div class="border-b border-gray-100 px-5 py-4 dark:border-gray-800"><h2 id="keyword-test-title" class="font-semibold text-gray-900 dark:text-white">{{ t('keywordRules.testTitle') }}</h2><p class="mt-1 text-xs leading-5 text-gray-500">{{ t('keywordRules.testHint') }}</p></div>
    <form class="space-y-4 p-5" @submit.prevent="run">
      <div class="grid gap-4 lg:grid-cols-2">
        <div class="space-y-3"><label class="block text-xs font-medium text-gray-500">{{ t('keywordRules.testText') }}<textarea v-model="text" class="input mt-2 min-h-36 w-full resize-y text-sm leading-6" maxlength="65536" :placeholder="t('keywordRules.testPlaceholder')" /></label><label v-if="selectedRuleId" class="flex items-center gap-2 text-xs text-gray-500"><input v-model="onlySelected" type="checkbox" class="accent-primary-600" />{{ t('keywordRules.testSelectedOnly') }}</label></div>
        <div class="grid grid-cols-2 content-start gap-3"><label class="text-xs font-medium text-gray-500">{{ t('keywordRules.testProtocol') }}<select v-model="protocol" class="input mt-2 w-full"><option v-for="value in protocols" :key="value" :value="value">{{ t(`keywordRules.protocolNames.${value}`) }}</option></select></label><label class="text-xs font-medium text-gray-500">{{ t('keywordRules.testModel') }}<input v-model.trim="model" class="input mt-2 w-full" maxlength="255" /></label><label class="text-xs font-medium text-gray-500">{{ t('keywordRules.testGroup') }}<input v-model.number="groupID" class="input mt-2 w-full" type="number" min="0" /></label><label class="text-xs font-medium text-gray-500">{{ t('keywordRules.testUser') }}<input v-model.number="userID" class="input mt-2 w-full" type="number" min="0" /></label><label class="col-span-2 flex items-center gap-2 text-xs text-gray-500"><input v-model="includePaused" type="checkbox" class="accent-primary-600" />{{ t('keywordRules.testIncludePaused') }}</label></div>
      </div>
      <div class="flex items-center justify-between gap-3"><p class="text-xs text-gray-400">{{ t('keywordRules.testDraftHint') }}</p><button type="submit" class="btn btn-primary shrink-0" :disabled="loading || !text.trim() || !policy.rules.length">{{ loading ? t('common.loading') : t('keywordRules.runTest') }}</button></div>
      <p v-if="error" class="text-sm text-red-600" role="alert">{{ error }}</p>
      <div v-if="result" class="space-y-3 border-t border-gray-100 pt-4 dark:border-gray-800" aria-live="polite">
        <p class="text-sm font-semibold" :class="result.hits.length ? 'text-primary-700 dark:text-primary-300' : 'text-gray-500'">{{ result.hits.length ? t(`keywordRules.testResults.${result.action === 'forward' ? 'inject' : result.action}`) : t('keywordRules.testNoHit') }}</p>
        <div v-for="hit in result.hits" :key="hit.rule_id" class="rounded-xl bg-gray-50 p-3 dark:bg-gray-900"><p class="text-xs font-medium text-gray-700 dark:text-gray-200">{{ hit.rule_name }} <span class="ml-2 text-gray-400">{{ t(`keywordRules.actions.${hit.action}`) }}</span></p><p class="mt-2 break-words font-mono text-xs text-gray-500">{{ hit.keywords.join(' · ') }}</p></div>
        <p v-if="result.reply" class="whitespace-pre-wrap break-words rounded-xl border border-primary-100 bg-primary-50/40 p-4 text-sm leading-6 text-gray-800 dark:border-primary-900 dark:bg-primary-950 dark:text-gray-100">{{ result.reply }}</p>
        <details v-if="result.effective_request" class="text-xs text-gray-500"><summary class="cursor-pointer">{{ t('keywordRules.effectiveRequest') }}</summary><pre class="mt-3 max-h-96 overflow-auto whitespace-pre-wrap break-words rounded-xl bg-gray-50 p-4 leading-6 dark:bg-gray-900">{{ JSON.stringify(result.effective_request, null, 2) }}</pre></details>
      </div>
    </form>
  </section>
</template>
<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { testKeywordRules, type KeywordPolicy, type KeywordProtocol, type KeywordTestResult } from '@/api/admin/keywordRules'
import { copyPolicy } from '@/utils/keywordRules'
const props = defineProps<{ policy: KeywordPolicy; selectedRuleId: string }>()
const { t } = useI18n()
const text = ref(''), model = ref(''), groupID = ref(0), userID = ref(0), protocol = ref<KeywordProtocol>('openai_chat_completions')
const onlySelected = ref(false), includePaused = ref(false), loading = ref(false), error = ref(''), result = ref<KeywordTestResult | null>(null)
const protocols: KeywordProtocol[] = ['openai_chat_completions', 'openai_responses', 'anthropic_messages', 'gemini']
let version = 0
watch(() => JSON.stringify([props.policy, props.selectedRuleId, text.value, model.value, groupID.value, userID.value, protocol.value, onlySelected.value, includePaused.value]), () => { version++; result.value = null; loading.value = false })
async function run() {
  if (!text.value.trim() || loading.value) return
  const current = ++version
  loading.value = true; error.value = ''; result.value = null
  const policy = copyPolicy(props.policy)
  if (onlySelected.value) policy.rules = policy.rules.filter(rule => rule.id === props.selectedRuleId)
  if (includePaused.value) policy.rules.forEach(rule => { rule.enabled = true })
  try {
    const data = await testKeywordRules({ policy, text: text.value, protocol: protocol.value, model: model.value, group_id: Number(groupID.value) || 0, user_id: Number(userID.value) || 0 })
    if (current === version) result.value = data
  } catch { if (current === version) error.value = t('keywordRules.testError') }
  finally { if (current === version) loading.value = false }
}
onBeforeUnmount(() => { version++ })
</script>
