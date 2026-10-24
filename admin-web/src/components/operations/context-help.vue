<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useWindowSize } from '@vueuse/core';

withDefaults(defineProps<{ title: string; text?: string; iconOnly?: boolean }>(), { iconOnly: true });
const open = ref(false);
const trigger = ref<{ $el: HTMLButtonElement } | null>(null);
const content = ref<HTMLElement | null>(null);
const { width } = useWindowSize();
const compact = computed(() => width.value < 600);
watch(compact, () => { open.value = false; });
function close() {
  open.value = false;
  trigger.value?.$el.focus();
}
watch(open, async value => {
  if (value) {
    await nextTick();
    content.value?.focus();
  }
});
</script>

<template>
  <NPopover v-if="!compact" v-model:show="open" trigger="click" placement="bottom-start" :show-arrow="false">
    <template #trigger>
      <NButton
        ref="trigger"
        quaternary
        size="tiny"
        :aria-label="title"
        :aria-expanded="open"
        class="context-help-trigger"
        :class="{ 'context-help-icon-only': iconOnly }"
        @keydown.esc.stop="close"
      >
        <template #icon>
          <svg v-if="iconOnly" viewBox="0 0 20 20" width="15" height="15" fill="none" aria-hidden="true">
            <circle cx="10" cy="10" r="7.5" stroke="currentColor" stroke-width="1.4" />
            <path d="M10 9v4.6" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" />
            <circle cx="10" cy="6.6" r=".85" fill="currentColor" />
          </svg>
          <svg v-else viewBox="0 0 20 20" width="15" height="15" fill="none" aria-hidden="true">
            <circle cx="10" cy="10" r="7.5" stroke="currentColor" stroke-width="1.4" />
            <path d="M7.9 7.4a2.2 2.2 0 0 1 4.3.6c0 1.6-2.2 1.7-2.2 3.3" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" />
            <circle cx="10" cy="13.8" r=".8" fill="currentColor" />
          </svg>
        </template>
        <template v-if="!iconOnly">说明</template>
      </NButton>
    </template>
    <section ref="content" class="context-help-content" :aria-label="title" tabindex="-1" @keydown.esc.stop="close">
      <div class="context-help-heading">
        <strong>{{ title }}</strong>
        <NButton quaternary size="tiny" aria-label="关闭说明" @click="close">关闭</NButton>
      </div>
      <div class="context-help-copy"><slot>{{ text }}</slot></div>
    </section>
  </NPopover>
  <template v-else>
    <NButton
      ref="trigger"
      quaternary
      size="tiny"
      :aria-label="title"
      :aria-expanded="open"
      class="context-help-trigger"
      :class="{ 'context-help-icon-only': iconOnly }"
      @click="open = !open"
      @keydown.esc.stop="close"
    >
      <template v-if="iconOnly" #icon>
        <svg viewBox="0 0 20 20" width="15" height="15" fill="none" aria-hidden="true">
          <circle cx="10" cy="10" r="7.5" stroke="currentColor" stroke-width="1.4" />
          <path d="M10 9v4.6" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" />
          <circle cx="10" cy="6.6" r=".85" fill="currentColor" />
        </svg>
      </template>
      <template v-if="!iconOnly">说明</template>
    </NButton>
    <NModal :show="open" preset="card" :title="title" class="context-help-dialog" @update:show="!$event && close()">
      <div ref="content" class="context-help-copy context-help-dialog-content" tabindex="-1" @keydown.esc.stop="close"><slot>{{ text }}</slot></div>
      <template #footer><NButton size="small" @click="close">关闭</NButton></template>
    </NModal>
  </template>
</template>

<style scoped>
.context-help-trigger {
  font-weight: 400;
  color: rgb(var(--base-text-color) / .6);
}
.context-help-icon-only {
  width: 26px;
  height: 26px;
  padding: 0;
}
.context-help-content {
  width: min(350px, calc(100vw - 56px));
  max-height: min(440px, 65vh);
  overflow-y: auto;
  outline: none;
}
.context-help-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 8px;
}
.context-help-heading strong {
  font-size: 13px;
  font-weight: 600;
}
.context-help-copy {
  font-size: 13px;
  line-height: 1.75;
  white-space: pre-line;
  overflow-wrap: anywhere;
  color: rgb(var(--base-text-color) / .75);
}
:global(.n-card.context-help-dialog) {
  width: min(360px, calc(100vw - 32px));
}
.context-help-dialog-content {
  max-height: min(440px, 65vh);
  overflow-y: auto;
  outline: none;
}
@media (pointer: coarse) {
  .context-help-icon-only { width: 34px; height: 34px; }
}
</style>
