<script setup lang="ts">
defineProps<{
  title: string;
  subtitle?: string;
  editing?: boolean;
  saving?: boolean;
  readonly?: boolean;
  dirty?: boolean;
  sections?: { key: string; label: string }[];
  section?: string;
}>();
defineEmits<{ back: []; save: []; 'update:section': [value: string] }>();
</script>

<template>
  <div class="ops-page management-workspace">
    <div class="ops-toolbar">
      <div class="management-title">
        <NButton v-if="editing" quaternary :disabled="saving" @click="$emit('back')">← 返回列表</NButton>
        <div>
          <div class="ops-page-label">{{ title }}</div>
          <p v-if="subtitle" class="ops-muted">{{ subtitle }}</p>
        </div>
      </div>
      <NSpace>
        <NTag v-if="editing && readonly" :bordered="false">只读</NTag>
        <NTag v-if="editing && dirty && !readonly" type="warning" :bordered="false">未保存</NTag>
        <slot name="actions" />
      </NSpace>
    </div>
    <slot name="alert" />
    <div v-if="editing" class="management-layout" :class="{ 'management-layout-simple': !sections?.length }">
      <nav v-if="sections?.length" class="management-nav" aria-label="编辑分区">
        <button
          v-for="item in sections"
          :key="item.key"
          type="button"
          :aria-current="section === item.key ? 'step' : undefined"
          :class="{ selected: section === item.key }"
          @click="$emit('update:section', item.key)"
        >
          {{ item.label }}
        </button>
      </nav>
      <NCard :bordered="false" class="ops-panel management-content"><slot /></NCard>
    </div>
    <slot v-else />
    <footer v-if="editing" class="management-footer">
      <span class="ops-muted">
        <slot name="status">{{ readonly ? '只读权限' : dirty ? '有未保存的修改' : '' }}</slot>
      </span>
      <NSpace>
        <NButton :disabled="saving" @click="$emit('back')">{{ readonly ? '返回列表' : '取消' }}</NButton>
        <NButton v-if="!readonly" type="primary" :loading="saving" @click="$emit('save')">保存</NButton>
      </NSpace>
    </footer>
  </div>
</template>

<style scoped>
.management-title {
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 0;
}
.management-title p {
  margin: 6px 0 0;
  line-height: 1.6;
}
.management-layout {
  display: grid;
  grid-template-columns: 160px minmax(0, 1fr);
  gap: 20px;
  align-items: start;
}
.management-layout-simple {
  grid-template-columns: minmax(0, 1fr);
}
.management-nav {
  display: flex;
  flex-direction: column;
  gap: 5px;
  position: sticky;
  top: 10px;
}
.management-nav button {
  text-align: left;
  padding: 10px 12px;
  background: transparent;
  border: 1px solid transparent;
  transition: background-color 120ms, color 120ms;
  border-radius: 7px;
  color: rgb(var(--base-text-color) / 0.75);
  cursor: pointer;
}
.management-nav button:focus-visible {
  outline: 2px solid rgb(var(--primary-color));
  outline-offset: 2px;
}
.management-nav button:hover {
  background: rgb(var(--base-text-color) / 0.035);
}
.management-nav button.selected {
  color: rgb(var(--primary-color));
  background: rgb(var(--primary-color) / 0.09);
  font-weight: 500;
}
.management-content {
  min-width: 0;
}
.management-footer {
  position: sticky;
  bottom: 0;
  z-index: 5;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 12px 18px;
  border: 1px solid rgb(var(--base-text-color) / 0.08);
  background: rgb(var(--container-bg-color));
  border-radius: 9px;
  box-shadow: 0 -2px 8px rgb(0 0 0/0.025);
}
@media (max-width: 800px) {
  .management-layout {
    grid-template-columns: minmax(0, 1fr);
    gap: 12px;
  }
  .management-nav {
    position: static;
    flex-direction: row;
    overflow-x: auto;
  }
  .management-nav button {
    flex-shrink: 0;
    padding: 10px 12px;
  }
  .management-title {
    align-items: flex-start;
    gap: 7px;
  }
  .management-footer {
    align-items: flex-start;
    flex-wrap: wrap;
    padding: 14px;
  }
  .management-footer > .ops-muted {
    width: 100%;
  }
}
</style>
