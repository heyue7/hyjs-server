<script setup lang="ts">
import { computed, h, onBeforeUnmount, ref, watch } from 'vue';
import type { SelectOption } from 'naive-ui';
import { NTag } from 'naive-ui';
import { api } from '@/service/operations';
import type { Item, ItemFacets, ItemsResponse } from '@/service/operations';
import ResourceImage from './resource-image.vue';
import ItemQuality from './item-quality.vue';
const props = defineProps<{
  value?: string;
  disabled?: boolean;
  category?: string;
  placeholder?: string;
  resources?: Item[];
  commerceCost?: boolean;
  recommendedIds?: string[];
}>();
const emit = defineEmits<{ 'update:value': [value: string] }>();
interface Option extends SelectOption { value: string; label: string; item?: Item }
const pageSize = 40;
const rows = ref<Item[]>([]);
const known = ref<Record<string, Item>>({});
const loading = ref(false);
const loadError = ref('');
const searchText = ref('');
const categoryFilter = ref<string | null>(null);
const targetFilter = ref<string | null>(null);
const facets = ref<ItemFacets>({ categories: [], qualities: [], applicableTypes: [] });
const page = ref(0);
const total = ref(0);
const opened = ref(false);
let timer: ReturnType<typeof setTimeout>;
let generation = 0;
let selectionGeneration = 0;
let alive = true;
const categoryLabels: Record<string, string> = { currency: '货币', character: '角色', 'character-exp': '角色经验', equipment: '装备', accessory: '饰品', pet: '精灵', 'pet-equipment': '精灵装备', material: '生活材料', item: '道具', fragment: '碎片', gem: '宝石', cooking: '料理' };
function remember(items: Item[]) { for (const item of items) known.value[item.id] = item; }
const localFacets = computed<ItemFacets>(() => {
  const categories = new Map<string, string>();
  const targets = new Map<string, string>();
  for (const item of props.resources || []) {
    if (item.category) categories.set(item.category, categoryLabels[item.category] || '其他资源');
    for (const key of item.applicableTypes || (item.applicableType ? [item.applicableType] : []))
      targets.set(key, item.applicableTypeLabel || facets.value.applicableTypes.find(row => row.value === key)?.label || '其他适用对象');
  }
  return { categories: [...categories].map(([value, label]) => ({ value, label })), qualities: [], applicableTypes: [...targets].map(([value, label]) => ({ value, label })) };
});
const availableFacets = computed(() => props.resources !== undefined ? localFacets.value : facets.value);
const recommended = computed(() => new Set(props.recommendedIds || []));
const categoryOptions = computed(() => availableFacets.value.categories.map(option => ({ label: String(option.label), value: String(option.value) })));
const targetOptions = computed(() => availableFacets.value.applicableTypes.map(option => ({ label: String(option.label), value: String(option.value) })));
const options = computed<Option[]>(() => {
  const result = rows.value.map(item => ({ value: item.id, label: item.name || item.id, item }));
  if (props.value && !result.some(row => row.value === props.value)) {
    const item = known.value[props.value];
    result.unshift({ value: props.value, label: item?.name || props.value, item });
  }
  return result;
});
const hasMore = computed(() => rows.value.length < total.value);
function invalidate() { clearTimeout(timer); generation += 1; loading.value = false; }
async function loadMore(reset = false) {
  if (!alive || props.disabled || (loading.value && !reset) || (!reset && !hasMore.value)) return;
  if (reset) { invalidate(); page.value = 0; rows.value = []; total.value = 0; }
  const current = generation;
  const nextPage = page.value + 1;
  loading.value = true;
  loadError.value = '';
  try {
    let result: ItemsResponse;
    if (props.resources !== undefined) {
      const keyword = searchText.value.toLowerCase().trim();
      const category = props.category || categoryFilter.value;
      const filtered = props.resources.filter(item =>
        (!category || item.category === category) &&
        (!targetFilter.value || (item.applicableTypes || [item.applicableType]).includes(targetFilter.value)) &&
        `${item.name} ${item.id} ${item.description || ''}`.toLowerCase().includes(keyword)
      ).sort((a, b) => Number(recommended.value.has(b.id)) - Number(recommended.value.has(a.id)));
      result = { items: filtered.slice((nextPage - 1) * pageSize, nextPage * pageSize), total: filtered.length };
    } else {
      const path = props.commerceCost ? '/activity-cost-resources' : '/items';
      const params = new URLSearchParams({ q: searchText.value, category: props.category || categoryFilter.value || '', applicableType: targetFilter.value || '', page: String(nextPage), pageSize: String(pageSize) });
      if (props.recommendedIds?.length) params.set('preferredIds', props.recommendedIds.join(','));
      result = await api<ItemsResponse>(`${path}?${params}`);
    }
    if (!alive || current !== generation) return;
    remember(result.items);
    const seen = new Set(rows.value.map(item => item.id));
    rows.value = [...rows.value, ...result.items.filter(item => !seen.has(item.id))];
    total.value = result.total;
    page.value = nextPage;
    if (result.facets) facets.value = result.facets;
  } catch (error) {
    if (alive && current === generation) loadError.value = error instanceof Error ? error.message : '资源加载失败';
  } finally { if (alive && current === generation) loading.value = false; }
}
function search(q: string) {
  searchText.value = q;
  invalidate(); page.value = 0; rows.value = []; total.value = 0;
  timer = setTimeout(() => loadMore(true), 250);
}
function filterChanged() { loadMore(true); }
function showChanged(show: boolean) {
  opened.value = show;
  if (show && !page.value) loadMore(true);
}
function onScroll(event: Event) {
  const el = event.target as HTMLElement | null;
  if (el && el.scrollTop + el.clientHeight >= el.scrollHeight - 60) loadMore();
}
function renderLabel(raw: SelectOption) {
  const option = raw as Option;
  return h('div', { class: 'resource-select-option' }, [
    h(ResourceImage, { item: option.item, size: 32, preview: false }),
    h('div', { class: 'resource-select-copy' }, [
      h('div', { class: 'resource-select-heading' }, [h('span', { class: 'resource-select-name' }, option.label), ...(option.item?.qualityLabel ? [h(ItemQuality, { item: option.item })] : []), ...(recommended.value.has(option.value) ? [h(NTag, { size: 'small', bordered: false, type: 'info' }, () => '推荐')] : [])]),
      h('div', { class: 'ops-mono ops-muted' }, option.value)
    ])
  ]);
}
watch(() => [props.resources, props.category, props.commerceCost, props.recommendedIds?.join(',')], () => {
  invalidate(); page.value = 0; rows.value = []; total.value = 0;
  if (props.resources) remember(props.resources);
  if (opened.value) loadMore(true);
}, { immediate: true });
watch(() => props.value, async value => {
  const current = ++selectionGeneration;
  if (!value || known.value[value]) return;
  try {
    const result = await api<ItemsResponse>(`/items?${new URLSearchParams({ ids: value, page: '1', pageSize: '1' })}`);
    if (alive && current === selectionGeneration && value === props.value) remember(result.items);
  } catch { /* Preserve the selected ID when display metadata cannot be loaded. */ }
}, { immediate: true });
onBeforeUnmount(() => { alive = false; invalidate(); selectionGeneration += 1; });
</script>

<template>
  <NSelect
    :value="value" :options="options" :render-label="renderLabel" :loading="loading" :disabled="disabled"
    :reset-menu-on-options-change="false" :theme-overrides="{ peers: { InternalSelectMenu: { height: '220px' } } }"
    filterable remote clearable :placeholder="placeholder || '搜索资源名称或 ID'"
    @search="search" @update:show="showChanged" @scroll="onScroll"
    @update:value="emit('update:value', $event || '')"
  >
    <template #header>
      <div class="resource-picker-filters" @keydown.stop>
        <NSelect v-if="!category" v-model:value="categoryFilter" :options="categoryOptions" :to="false" size="small" filterable clearable placeholder="全部分类" aria-label="资源分类" @update:value="filterChanged" />
        <NSelect v-model:value="targetFilter" :options="targetOptions" :to="false" size="small" filterable clearable placeholder="全部适用对象" aria-label="资源适用对象" @update:value="filterChanged" />
      </div>
    </template>
    <template #empty>{{ loadError || (searchText || categoryFilter || targetFilter ? '没有匹配的资源，试试其他筛选条件' : '暂无可用资源') }}</template>
    <template #action>
      <div class="resource-picker-status">
        <span>{{ loading ? '加载中…' : `${rows.length} / ${total} 项${hasMore ? ' · 向下滚动加载更多' : ''}` }}</span>
        <NButton v-if="loadError" size="tiny" text type="primary" @click="loadMore(page === 0)">重试</NButton>
      </div>
    </template>
  </NSelect>
</template>

<style scoped>
.resource-picker-filters { position: relative; z-index: 2; display: flex; gap: 8px; padding: 8px 10px; min-width: 260px; }
.resource-picker-filters > :deep(.n-select) { flex: 1; min-width: 0; }
.resource-picker-status { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 7px 12px; color: rgb(var(--base-text-color) / .6); font-size: 12px; }
:deep(.n-base-selection .resource-select-option) { width: 100%; gap: 6px; padding: 0; }
:deep(.n-base-selection .resource-select-copy) { flex: 1; min-width: 0; }
:deep(.n-base-selection .resource-select-heading) { display: block; }
:deep(.n-base-selection .resource-select-option .item-quality),
:deep(.n-base-selection .resource-select-option .n-tag),
:deep(.n-base-selection .resource-select-option .ops-mono) { display: none; }
:deep(.n-base-selection .resource-select-option .resource-thumb) { width: 24px !important; height: 24px !important; }
@media(max-width: 480px) { .resource-picker-filters { min-width: 0; } }
</style>
