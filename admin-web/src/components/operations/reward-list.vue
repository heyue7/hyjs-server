<script setup lang="ts">
import { computed } from 'vue';
import type { Item, ResourceAmount as Reward } from '@/service/operations';
import ResourceSelect from './resource-select.vue';
const props = withDefaults(
  defineProps<{
    value: Reward[];
    disabled?: boolean;
    max?: number;
    label?: string;
    resources?: Item[];
  }>(),
  {
    max: 100,
    label: '奖励'
  }
);
const emit = defineEmits<{ 'update:value': [value: Reward[]] }>();
const rows = computed(() => props.value || []);
function update(index: number, key: 'resourceId' | 'amount', value: string | number | null) {
  const copy = rows.value.map((row) => ({ ...row }));
  copy[index] = { ...copy[index], [key]: value };
  emit('update:value', copy);
}
function remove(index: number) {
  emit(
    'update:value',
    rows.value.filter((_, i) => i !== index)
  );
}
function add() {
  emit('update:value', [...rows.value, { resourceId: '', amount: 1 }]);
}
</script>

<template>
  <div class="reward-list">
    <div v-for="(reward, index) in rows" :key="index" class="reward-list-line">
      <ResourceSelect
        :value="reward.resourceId"
        :disabled="disabled"
        :resources="resources"
        @update:value="update(index, 'resourceId', $event)"
      />
      <NInputNumber
        :value="reward.amount"
        :disabled="disabled"
        :min="1"
        :precision="0"
        placeholder="数量"
        @update:value="update(index, 'amount', $event)"
      />
      <NButton quaternary :disabled="disabled" @click="remove(index)">移除</NButton>
    </div>
    <NButton v-if="rows.length < max" dashed size="small" :disabled="disabled" @click="add">添加{{ label }}</NButton>
  </div>
</template>
