<template>
  <div class="mx-auto max-w-4xl space-y-4">
    <div class="flex items-center justify-between gap-3"><p class="text-xs text-gray-500">{{ t('conversationArchive.contextCount', { count: messages.length }) }}</p><button v-if="start > 0" class="text-xs font-medium text-primary-600 hover:underline" @click="visible += 50">{{ t('conversationArchive.loadEarlier') }}</button></div>
    <template v-for="(message, index) in messages.slice(start)" :key="start + index">
      <details v-if="message.kind === 'tool' || ['system', 'developer', 'tool'].includes(message.role)" class="overflow-hidden rounded-xl border border-gray-200 dark:border-gray-800">
        <summary class="cursor-pointer bg-gray-50 px-4 py-3 text-xs font-medium text-gray-600 dark:bg-gray-900 dark:text-gray-300">{{ role(message.role) }}<span v-if="message.name" class="ml-2 font-mono">{{ message.name }}</span><span class="ml-2 font-normal text-gray-400">{{ t('conversationArchive.expand') }}</span></summary>
        <pre class="max-h-[32rem] overflow-auto whitespace-pre-wrap break-words p-4 text-xs leading-6 text-gray-700 dark:text-gray-200">{{ message.text }}</pre>
      </details>
      <div v-else class="flex" :class="message.role === 'user' ? 'justify-end pl-5 sm:pl-16' : 'justify-start pr-5 sm:pr-16'">
        <div class="min-w-0 max-w-full rounded-2xl px-4 py-3" :class="message.role === 'user' ? 'rounded-tr-sm bg-primary-600 text-white' : 'rounded-tl-sm border border-gray-200 bg-white text-gray-900 dark:border-gray-700 dark:bg-gray-800 dark:text-gray-100'">
          <p class="mb-2 text-[11px] font-medium" :class="message.role === 'user' ? 'text-primary-100' : 'text-gray-400'">{{ role(message.role) }}</p>
          <p class="max-h-[32rem] overflow-auto whitespace-pre-wrap break-words text-sm leading-7">{{ message.text }}</p>
          <p v-if="message.mediaCount" class="mt-2 text-xs opacity-70">{{ t('conversationArchive.mediaOmitted', { count: message.mediaCount }) }}</p>
        </div>
      </div>
    </template>
    <p v-if="!messages.length" class="py-8 text-center text-sm text-gray-400">{{ t('conversationArchive.noText') }}</p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ArchiveMessage } from '@/utils/conversationArchive'
const props = defineProps<{ messages: ArchiveMessage[] }>()
const { t } = useI18n()
const visible = ref(50)
const start = computed(() => Math.max(0, props.messages.length - visible.value))
const role = (value: string) => t(`conversationArchive.roles.${['system', 'developer', 'user', 'assistant', 'tool'].includes(value) ? value : 'message'}`)
</script>
