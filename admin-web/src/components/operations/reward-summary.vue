<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { api } from '@/service/operations';
import type { Item, ItemsResponse, ResourceAmount } from '@/service/operations';
import ResourceImage from './resource-image.vue';
import ItemQuality from './item-quality.vue';
const props = defineProps<{
  rewards?: ResourceAmount[];
  emptyText?: string;
  resources?: Item[];
}>();
const catalogue = ref<Record<string, Item>>({});
const failed = ref(false);
let generation = 0;
const rows = computed(() => (props.rewards || []).filter((row) => row.resourceId));
watch(
  () => [rows.value.map((row) => row.resourceId).join(','), props.resources],
  async () => {
    const seq = ++generation;
    if (props.resources !== undefined) {
      for (const item of props.resources) catalogue.value[item.id] = item;
      return;
    }
    const ids = [...new Set(rows.value.map((row) => row.resourceId))].filter((id) => !catalogue.value[id]);
    if (!ids.length) return;
    failed.value = false;
    try {
      for (let i = 0; i < ids.length; i += 100) {
        const data = await api<ItemsResponse>(
          `/items?${new URLSearchParams({ ids: ids.slice(i, i + 100).join(','), page: '1', pageSize: '100' })}`
        );
        if (seq !== generation) return;
        for (const item of data.items) catalogue.value[item.id] = item;
      }
    } catch {
      if (seq === generation) failed.value = true;
    }
  },
  { immediate: true }
);
onBeforeUnmount(() => {
  generation += 1;
});
</script>

<template>
  <div class="reward-summary">
    <span v-if="!rows.length" class="ops-muted">{{ emptyText || '未配置奖励' }}</span>
    <div v-for="(reward, index) in rows" :key="`${reward.resourceId}:${index}`" class="reward-summary-item">
      <ResourceImage :item="catalogue[reward.resourceId]" :size="36" />
      <span>{{ catalogue[reward.resourceId]?.name || reward.resourceId }}</span>
      <ItemQuality v-if="catalogue[reward.resourceId]?.qualityLabel" :item="catalogue[reward.resourceId]" />
      <strong>× {{ Number(reward.amount || 0).toLocaleString() }}</strong>
    </div>
    <span v-if="failed" class="ops-muted">资源名称暂未载入，当前显示原ID。</span>
  </div>
</template>

<style scoped>
.reward-summary {
  display: flex;
  gap: 12px 20px;
  flex-wrap: wrap;
}
.reward-summary-item {
  display: flex;
  gap: 9px;
  align-items: center;
  min-width: 0;
}
.reward-summary-item strong {
  font-variant-numeric: tabular-nums;
  font-weight: 500;
  white-space: nowrap;
}
</style>
