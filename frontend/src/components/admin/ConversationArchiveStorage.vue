<template>
  <section class="card overflow-hidden" aria-labelledby="archive-storage-title">
    <div class="flex items-start justify-between gap-4 border-b border-gray-100 px-5 py-4 dark:border-gray-800">
      <div><h2 id="archive-storage-title" class="font-semibold text-gray-900 dark:text-white">{{ t('conversationArchive.storageTitle') }}</h2><p class="mt-1 text-sm text-gray-500">{{ t('conversationArchive.permanentHint') }}</p></div>
      <button class="btn btn-secondary shrink-0 whitespace-nowrap" @click="$emit('close')">{{ t('conversationArchive.close') }}</button>
    </div>
    <form class="space-y-4 p-5" @submit.prevent="save">
      <div v-if="info" class="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div v-for="stat in [{ label: 'storedCalls', value: info.records.toLocaleString() }, { label: 'physicalSize', value: bytes(info.storage_bytes) }, { label: 'availableSpace', value: bytes(info.free_bytes) }, { label: 'retention', value: info.retention_days === 0 ? t('conversationArchive.permanent') : t('conversationArchive.retentionDays', { days: info.retention_days }) }]" :key="stat.label" class="rounded-xl bg-gray-50 p-3 dark:bg-gray-900"><p class="text-xs text-gray-500">{{ t(`conversationArchive.${stat.label}`) }}</p><p class="mt-2 font-semibold text-gray-900 dark:text-white">{{ stat.value }}</p></div>
      </div>
      <p v-if="info?.capture_paused" class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-950 dark:text-amber-200">{{ t('conversationArchive.capacityPaused') }}</p>
      <div class="flex flex-wrap items-end justify-between gap-4">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('conversationArchive.contextLimit') }}<select v-model.number="limit" class="input mt-2 block w-full sm:w-56" :disabled="loading || saving"><option v-if="![4194304, 8388608, 16777216].includes(limit)" :value="limit">{{ bytes(limit) }}</option><option :value="4194304">4 MB</option><option :value="8388608">8 MB</option><option :value="16777216">16 MB</option></select><span class="mt-2 block max-w-xl text-xs font-normal leading-5 text-gray-500">{{ t('conversationArchive.contextLimitHint') }}</span></label>
        <button class="btn btn-primary" type="submit" :disabled="loading || saving || !info">{{ saving ? t('common.submitting') : t('conversationArchive.saveStorage') }}</button>
      </div>
      <p v-if="error" class="text-sm text-red-600" role="alert">{{ error }}</p><p v-if="saved" class="text-sm text-emerald-700 dark:text-emerald-300" role="status">{{ t('conversationArchive.storageSaved') }}</p>
    </form>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiClient } from '@/api/client'

interface StorageInfo { retention_days: number; max_context_bytes: number; records: number; storage_bytes: number; free_bytes: number; capture_paused: boolean }
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const info = ref<StorageInfo | null>(null)
const limit = ref(4194304)
const loading = ref(true)
const saving = ref(false)
const saved = ref(false)
const error = ref('')
const bytes = (value: number) => value > 0 ? `${(value / 1048576).toLocaleString(undefined, { maximumFractionDigits: 1 })} MB` : '—'
async function load() {
  const { data } = await apiClient.get<StorageInfo>('/admin/conversation-archives/settings')
  info.value = data; limit.value = data.max_context_bytes
}
onMounted(async () => { try { await load() } catch { error.value = t('conversationArchive.loadError') } finally { loading.value = false } })
async function save() {
  if (!info.value || saving.value) return
  saving.value = true; saved.value = false; error.value = ''
  try {
    await apiClient.put('/admin/conversation-archives/settings', { retention_days: 0, max_context_bytes: limit.value })
    await load(); saved.value = true; emit('saved')
  } catch { error.value = t('conversationArchive.loadError') }
  finally { saving.value = false }
}
</script>
