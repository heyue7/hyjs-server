import { computed, h, onBeforeUnmount, ref, watch } from 'vue';
import type { SelectGroupOption, SelectOption } from 'naive-ui';
import { api } from './operations';
import type { Item, ItemsResponse, ResourceAmount } from './operations';

/** One catalogue request per batch, shared by all options in this editor. Never changes form data. */
export function useActivitySelectorSummary(resources: () => ResourceAmount[]) {
  const catalogue = ref<Record<string, Item>>({});
  const failed = ref(false);
  const loading = ref(false);
  const ids = computed(() => [...new Set(resources().map(row => row.resourceId).filter(Boolean))].sort());
  let generation = 0;
  async function retry() {
    const seq = ++generation;
    const missing = ids.value.filter(id => !catalogue.value[id]);
    failed.value = false;
    loading.value = Boolean(missing.length);
    try {
      for (let i = 0; i < missing.length; i += 100) {
        const data = await api<ItemsResponse>(`/items?${new URLSearchParams({ ids: missing.slice(i, i + 100).join(','), page: '1', pageSize: '100' })}`);
        if (seq !== generation) return;
        for (const item of data.items) catalogue.value[item.id] = item;
      }
      if (seq === generation) failed.value = missing.some(id => !catalogue.value[id]);
    } catch {
      if (seq === generation) failed.value = true;
    } finally {
      if (seq === generation) loading.value = false;
    }
  }
  watch(() => ids.value.join(','), retry, { immediate: true });
  onBeforeUnmount(() => { generation += 1; });
  function rewardsText(rows: ResourceAmount[], empty = '未配置奖励') {
    return rows.filter(row => row.resourceId).map(row => `${catalogue.value[row.resourceId]?.name || row.resourceId} × ${Number(row.amount || 0).toLocaleString()}`).join('、') || empty;
  }
  return { rewardsText, failed, loading, retry };
}

export function renderActivityOption(option: SelectOption | SelectGroupOption) {
  return h('span', { style: { whiteSpace: 'normal', overflowWrap: 'anywhere', lineHeight: '1.6' } }, String(option.label || ''));
}
