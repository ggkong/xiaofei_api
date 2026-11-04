<template>
  <section class="card overflow-hidden border border-primary-200 dark:border-primary-900" aria-labelledby="keyword-rules-title">
    <div class="flex items-start justify-between gap-4 border-b border-gray-100 px-5 py-4 dark:border-gray-800">
      <div>
        <h2 id="keyword-rules-title" class="font-semibold text-gray-900 dark:text-white">{{ t('conversationArchive.keywords') }}</h2>
        <p class="mt-1 text-sm text-gray-500">{{ t('conversationArchive.keywordHint') }}</p>
      </div>
      <button class="btn btn-secondary" @click="$emit('close')">{{ t('conversationArchive.close') }}</button>
    </div>
    <form class="space-y-4 p-5" @submit.prevent="save">
      <label class="block text-sm font-medium text-gray-700 dark:text-gray-200">
        {{ t('conversationArchive.keywordList') }}
        <textarea v-model="words" class="input mt-2 min-h-36 w-full resize-y font-mono text-sm" :placeholder="t('conversationArchive.keywordPlaceholder')" :disabled="loading || saving" />
      </label>
      <p v-if="error" class="text-sm text-red-600" role="alert">{{ error }}</p>
      <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="text-sm text-gray-500" role="status">{{ loading ? t('conversationArchive.loading') : saved ? t('conversationArchive.rulesSaved') : t('conversationArchive.keywordScope') }}</p>
        <button class="btn btn-primary" :disabled="loading || saving || unavailable" type="submit">{{ saving ? t('common.submitting') : t('conversationArchive.saveRules') }}</button>
      </div>
    </form>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiClient } from '@/api/client'

defineEmits<{ close: [] }>()
const { t } = useI18n()
const words = ref('')
const loading = ref(true)
const saving = ref(false)
const saved = ref(false)
const unavailable = ref(false)
const error = ref('')

onMounted(async () => {
  try {
    const { data } = await apiClient.get<{ blocked_keywords: string[] }>('/admin/risk-control/config')
    words.value = (data.blocked_keywords ?? []).join('\n')
  } catch {
    unavailable.value = true
    error.value = t('conversationArchive.rulesError')
  } finally { loading.value = false }
})

async function save() {
  if (saving.value || loading.value || unavailable.value) return
  saving.value = true
  saved.value = false
  error.value = ''
  const keywords = words.value.split(/\r?\n/).map(word => word.trim()).filter(Boolean)
  try {
    await apiClient.put('/admin/risk-control/config', {
      enabled: keywords.length > 0,
      mode: 'pre_block',
      keyword_blocking_mode: 'keyword_only',
      blocked_keywords: keywords,
      all_groups: true,
      group_ids: [],
      model_filter: { type: 'all', models: [] },
      auto_ban_enabled: false,
      email_on_hit: false,
      pre_hash_check_enabled: false,
      block_status: 403,
      block_message: t('conversationArchive.blockMessage')
    })
    if (keywords.length) await apiClient.put('/admin/settings', { risk_control_enabled: true })
    saved.value = true
  } catch { error.value = t('conversationArchive.rulesError') }
  finally { saving.value = false }
}
</script>
