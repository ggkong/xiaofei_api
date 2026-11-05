<template>
  <BaseDialog :show="true" :title="t('keywordRules.importTitle')" width="wide" @close="$emit('close')">
    <div class="space-y-4">
      <p class="text-sm leading-6 text-gray-500">{{ t('keywordRules.importHint') }}</p>
      <div class="grid gap-3 sm:grid-cols-2"><label class="text-xs font-medium text-gray-500">{{ t('keywordRules.importMode') }}<select v-model="mode" class="input mt-2 w-full" @change="changeMode"><option value="keywords">{{ t('keywordRules.importKeywords') }}</option><option value="rules">{{ t('keywordRules.importRules') }}</option></select></label><label class="text-xs font-medium text-gray-500">{{ t('keywordRules.fileFormat') }}<select v-model="format" class="input mt-2 w-full"><option v-if="mode === 'keywords'" value="txt">TXT</option><option value="csv">CSV</option><option value="json">JSON</option></select></label></div>
      <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl bg-gray-50 p-3 text-xs text-gray-500 dark:bg-gray-900"><span>{{ mode === 'keywords' ? t('keywordRules.importTarget', { name: targetName || t('keywordRules.newImportRule') }) : t('keywordRules.importCSVExample') }}</span><label class="cursor-pointer font-medium text-primary-600 hover:underline">{{ t('keywordRules.chooseFile') }}<input class="sr-only" type="file" accept=".txt,.csv,.json" @change="readFile" /></label></div>
      <textarea v-model="source" class="input min-h-52 w-full resize-y font-mono text-xs leading-6" :placeholder="placeholder" />
      <p v-if="error" class="text-sm text-red-600" role="alert">{{ error }}</p>
      <div v-if="parsed" class="space-y-3 rounded-xl border border-primary-100 bg-primary-50/40 p-4 dark:border-primary-900 dark:bg-primary-950/20">
        <p class="text-sm font-medium text-primary-700 dark:text-primary-300">{{ t('keywordRules.importPreview', { rules: parsed.rules.length, keywords: keywordCount }) }}</p>
        <div v-if="parsed.keywords.length" class="flex flex-wrap gap-2"><span v-for="(word, index) in parsed.keywords.slice(0, 8)" :key="index" class="max-w-full truncate rounded-md bg-white px-2 py-1 text-xs text-gray-600 dark:bg-gray-900 dark:text-gray-300" :title="word">{{ word }}</span></div>
        <p v-for="rule in parsed.rules.slice(0, 5)" :key="rule.id" class="truncate text-xs text-gray-500">{{ rule.name }} · {{ t(`keywordRules.actions.${rule.action}`) }} · {{ rule.keywords.length }} {{ t('keywordRules.keywords') }}</p>
      </div>
      <label v-if="mode === 'rules'" class="flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300"><input v-model="replace" type="checkbox" class="accent-primary-600" />{{ t('keywordRules.replaceRules') }}</label>
    </div>
    <template #footer><button class="btn btn-secondary" @click="$emit('close')">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="!!error || !parsed || !keywordCount" @click="apply">{{ t('keywordRules.applyImport') }}</button></template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { KeywordImportError, parseKeywordImport, type KeywordImport } from '@/utils/keywordRules'
const props = defineProps<{ targetName?: string; preserveWhitespace?: boolean }>()
const emit = defineEmits<{ close: []; apply: [value: KeywordImport & { replace: boolean }] }>()
const { t } = useI18n()
const source = ref(''), mode = ref<'keywords' | 'rules'>('keywords'), format = ref<'txt' | 'csv' | 'json'>('txt'), replace = ref(false), fileError = ref('')
watch(source, () => { fileError.value = '' })
const parsedState = computed(() => { if (!source.value.trim()) return { value: null, error: '' }; try { return { value: parseKeywordImport(source.value, format.value, mode.value, props.preserveWhitespace), error: '' } } catch (error) { return { value: null, error: t(`keywordRules.${error instanceof KeywordImportError ? error.key : 'importInvalidJSON'}`) } } })
const placeholder = computed(() => format.value === 'csv' ? '关键词,回复\n查询使用说明,请查看团队帮助中心' : format.value === 'json' ? '["关键词一", "关键词二"]' : t('keywordRules.keywordsPlaceholder'))
const parsed = computed(() => parsedState.value.value)
const error = computed(() => fileError.value || parsedState.value.error)
const keywordCount = computed(() => parsed.value ? parsed.value.keywords.length + parsed.value.rules.reduce((total, rule) => total + rule.keywords.length, 0) : 0)
function changeMode() { if (mode.value === 'rules' && format.value === 'txt') format.value = 'csv' }
let fileVersion = 0
async function readFile(event: Event) {
  const input = event.target as HTMLInputElement, file = input.files?.[0], version = ++fileVersion
  if (!file) return
  fileError.value = ''
  try {
    if (file.size > 4 * 1024 * 1024) throw new KeywordImportError('importTooLarge')
    const text = await file.text()
    if (version !== fileVersion) return
    const extension = file.name.split('.').pop()?.toLowerCase()
    format.value = extension === 'json' ? 'json' : extension === 'csv' ? 'csv' : 'txt'
    if (format.value === 'txt') mode.value = 'keywords'
    source.value = text
  } catch (error) { if (version === fileVersion) fileError.value = t(`keywordRules.${error instanceof KeywordImportError ? error.key : 'importInvalidJSON'}`) }
  finally { input.value = '' }
}
function apply() { if (!error.value && parsed.value && keywordCount.value) emit('apply', { ...parsed.value, replace: mode.value === 'rules' && replace.value }) }
</script>
