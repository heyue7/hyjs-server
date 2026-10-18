<script setup lang="ts">
import { h, onBeforeUnmount, ref, watch } from 'vue';
import type { SelectOption } from 'naive-ui';
import { api } from '@/service/operations';
import type { Item, ItemsResponse } from '@/service/operations';
import ResourceImage from './resource-image.vue';
import ItemQuality from './item-quality.vue';
const props = defineProps<{
  value?: string;
  disabled?: boolean;
  category?: string;
  placeholder?: string;
  resources?: Item[];
}>();
const emit = defineEmits<{ 'update:value': [value: string] }>();
interface Option extends SelectOption {
  value: string;
  label: string;
  item?: Item;
}
const options = ref<Option[]>([]);
const known = new Map<string, Item>();
const loading = ref(false);
let timer: ReturnType<typeof setTimeout>;
let generation = 0;
function keepSelected(items: Item[]) {
  for (const item of items) known.set(item.id, item);
  const rows: Option[] = items.map((item) => ({
    value: item.id,
    label: item.name || item.id,
    item
  }));
  if (props.value && !rows.some((option) => option.value === props.value)) {
    const item = known.get(props.value);
    rows.unshift({
      value: props.value,
      label: item?.name || props.value,
      item
    });
  }
  options.value = rows;
}
async function load(q = '') {
  generation += 1;
  const current = generation;
  loading.value = true;
  try {
    if (props.resources !== undefined) {
      const keyword = q.toLowerCase().trim();
      keepSelected(props.resources.filter((item) => `${item.name} ${item.id}`.toLowerCase().includes(keyword)));
      return;
    }
    const result = await api<ItemsResponse>(
      `/items?${new URLSearchParams({ q, category: props.category || '', page: '1', pageSize: '40' })}`
    );
    if (current === generation) keepSelected(result.items);
  } finally {
    if (current === generation) loading.value = false;
  }
}
watch(
  () => props.resources,
  () => {
    if (props.resources !== undefined) load().catch(() => {});
  },
  { immediate: true }
);
function search(q: string) {
  clearTimeout(timer);
  timer = setTimeout(() => load(q).catch(() => {}), 250);
}
function renderLabel(raw: SelectOption) {
  const option = raw as Option;
  return h('div', { class: 'resource-select-option' }, [
    h(ResourceImage, { item: option.item, size: 32, preview: false }),
    h('div', { class: 'resource-select-copy' }, [
      h('div', { class: 'resource-select-heading' }, [
        h('span', { class: 'resource-select-name' }, option.label),
        ...(option.item?.qualityLabel ? [h(ItemQuality, { item: option.item })] : [])
      ]),
      h('div', { class: 'ops-mono ops-muted' }, option.value)
    ])
  ]);
}
watch(
  () => props.value,
  (value) => {
    if (!value) return;
    keepSelected([...known.values()]);
    if (!known.has(value)) load(value).catch(() => {});
  },
  { immediate: true }
);
onBeforeUnmount(() => {
  clearTimeout(timer);
  generation += 1;
});
</script>

<template>
  <NSelect
    :value="value"
    :options="options"
    :render-label="renderLabel"
    :loading="loading"
    :disabled="disabled"
    filterable
    remote
    clearable
    :placeholder="placeholder || '搜索资源名称或 ID'"
    @search="search"
    @focus="load().catch(() => {})"
    @update:value="emit('update:value', $event || '')"
  />
</template>
