<template>
  <section class="card overflow-hidden border border-primary-200 dark:border-primary-900" aria-labelledby="keyword-rules-title">
    <div class="flex items-start justify-between gap-4 border-b border-gray-100 px-5 py-4 dark:border-gray-800">
      <div>
        <h2 id="keyword-rules-title" class="font-semibold text-gray-900 dark:text-white">{{ t('conversationArchive.keywords') }}</h2>
        <p class="mt-1 text-sm text-gray-500">{{ t('conversationArchive.keywordHint') }}</p>
      </div>
      <button class="btn btn-secondary shrink-0 whitespace-nowrap" @click="$emit('close')">{{ t('conversationArchive.close') }}</button>
    </div>
    <form class="space-y-4 p-5" @submit.prevent="save">
      <div class="flex items-center justify-between gap-4 rounded-xl bg-gray-50 p-4 dark:bg-gray-900">
        <div><label for="keyword-master" class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('conversationArchive.ruleSwitch') }}</label><p class="mt-1 text-xs text-gray-500">{{ t('conversationArchive.ruleSwitchHint') }}</p></div>
        <input id="keyword-master" v-model="rulesEnabled" type="checkbox" role="switch" class="h-5 w-5 accent-primary-600" :disabled="loading || saving" />
      </div>
      <div class="grid gap-4 sm:grid-cols-2">
        <label class="block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('conversationArchive.matchMode') }}
          <select v-model="matchMode" class="input mt-2 block w-full" :disabled="loading || saving">
            <option value="exact">{{ t('conversationArchive.matchExact') }}</option>
            <option value="literal">{{ t('conversationArchive.matchLiteral') }}</option>
            <option v-if="matchMode === 'contains' && !autoReplyEnabled" value="contains">{{ t('conversationArchive.matchLegacy') }}</option>
          </select>
        </label>
        <label class="flex items-center gap-3 pt-5 text-sm font-medium text-gray-700 dark:text-gray-200"><input v-model="autoReplyEnabled" type="checkbox" class="h-4 w-4 accent-primary-600" :disabled="loading || saving" @change="ensurePreciseReply" />{{ t('conversationArchive.autoReplySwitch') }}</label>
      </div>
      <p class="text-xs leading-5 text-gray-500">{{ matchMode === 'exact' ? t('conversationArchive.exactHint') : matchMode === 'literal' ? t('conversationArchive.literalHint') : t('conversationArchive.legacyHint') }}</p>
      <label class="block text-sm font-medium text-gray-700 dark:text-gray-200">
        {{ t('conversationArchive.keywordList') }}
        <textarea v-model="words" class="input mt-2 min-h-36 w-full resize-y font-mono text-sm" :placeholder="t('conversationArchive.keywordPlaceholder')" :disabled="loading || saving" />
      </label>
      <label v-if="autoReplyEnabled" class="block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('conversationArchive.replyText') }}<textarea v-model="replyText" class="input mt-2 min-h-28 w-full resize-y text-sm" maxlength="4000" :placeholder="t('conversationArchive.replyPlaceholder')" :disabled="loading || saving" /><span class="mt-1 block text-xs font-normal text-gray-500">{{ t('conversationArchive.replyHint') }}</span></label>
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
const rulesEnabled = ref(false)
const autoReplyEnabled = ref(false)
const replyText = ref('')
const matchMode = ref<'exact' | 'literal' | 'contains'>('exact')
const loading = ref(true)
const saving = ref(false)
const saved = ref(false)
const unavailable = ref(false)
const error = ref('')

onMounted(async () => {
  try {
    const { data } = await apiClient.get<{ blocked_keywords: string[]; enabled?: boolean; keyword_auto_reply_enabled?: boolean; keyword_reply?: string; keyword_match_mode?: 'exact' | 'literal' | 'contains' }>('/admin/risk-control/config')
    words.value = (data.blocked_keywords ?? []).join('\n')
    rulesEnabled.value = !!data.enabled
    autoReplyEnabled.value = !!data.keyword_auto_reply_enabled
    replyText.value = data.keyword_reply ?? ''
    matchMode.value = data.keyword_match_mode ?? (data.enabled ? 'contains' : 'exact')
    ensurePreciseReply()
  } catch {
    unavailable.value = true
    error.value = t('conversationArchive.rulesError')
  } finally { loading.value = false }
})

function ensurePreciseReply() {
  if (autoReplyEnabled.value && matchMode.value === 'contains') matchMode.value = 'exact'
}

async function save() {
  if (saving.value || loading.value || unavailable.value) return
  saving.value = true
  saved.value = false
  error.value = ''
  const keywords = words.value.split(/\r?\n/).map(word => word.trim()).filter(Boolean)
  if (!keywords.length) rulesEnabled.value = false
  if (rulesEnabled.value && autoReplyEnabled.value && !replyText.value.trim()) {
    error.value = t('conversationArchive.replyRequired')
    saving.value = false
    return
  }
  try {
    await apiClient.put('/admin/risk-control/config', {
      enabled: rulesEnabled.value && keywords.length > 0,
      mode: 'pre_block',
      keyword_blocking_mode: 'keyword_only',
      blocked_keywords: keywords,
      keyword_match_mode: matchMode.value,
      keyword_auto_reply_enabled: autoReplyEnabled.value,
      keyword_reply: replyText.value,
      all_groups: true,
      group_ids: [],
      model_filter: { type: 'all', models: [] },
      auto_ban_enabled: false,
      email_on_hit: false,
      pre_hash_check_enabled: false,
      block_status: 403,
      block_message: t('conversationArchive.blockMessage')
    })
    if (rulesEnabled.value && keywords.length) {
      await apiClient.put('/admin/settings', { risk_control_enabled: true })
      // Publish the rules with the freshly enabled global gate, before reporting success.
      await apiClient.put('/admin/risk-control/config', { enabled: true })
    }
    saved.value = true
  } catch { error.value = t('conversationArchive.rulesError') }
  finally { saving.value = false }
}
</script>
