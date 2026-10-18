<script setup lang="ts">
import type { FieldDrop } from '@/service/fields';
import type { Item } from '@/service/operations';
import ResourceSelect from '@/components/operations/resource-select.vue';
const props = defineProps<{
  value: FieldDrop[];
  disabled?: boolean;
  fixed?: boolean;
  resources: (Item & { maxAmount?: number })[];
}>();
const emit = defineEmits<{ 'update:value': [value: FieldDrop[]] }>();
function add() {
  emit('update:value', [...props.value, { resourceId: '', quantities: [{ amount: 1, weight: 1 }] }]);
}
function remove(index: number) {
  emit(
    'update:value',
    props.value.filter((_, i) => i !== index)
  );
}
</script>

<template>
  <div class="space-y-3">
    <div v-for="(drop, i) in value" :key="i" class="border border-gray-200 rounded p-3 dark:border-gray-700">
      <div class="mb-2 flex items-center gap-2">
        <ResourceSelect
          v-model:value="drop.resourceId"
          class="min-w-50 flex-1"
          :disabled="disabled"
          :resources="resources"
        />
        <NButton size="small" :disabled="disabled" @click="remove(i)">移除道具</NButton>
      </div>
      <div v-for="(q, j) in drop.quantities" :key="j" class="mb-2 flex flex-wrap items-center gap-2">
        <span>数量</span>
        <NInputNumber
          v-model:value="q.amount"
          :min="fixed ? 1 : 0"
          :max="resources.find(r => r.id === drop.resourceId)?.maxAmount || 2147483647"
          :disabled="disabled"
          class="w-36"
        />
        <template v-if="!fixed">
          <span>权重</span>
          <NInputNumber v-model:value="q.weight" :min="1" :max="1000000" :disabled="disabled" class="w-28" />
          <NButton text :disabled="disabled || drop.quantities.length < 2" @click="drop.quantities.splice(j, 1)">
            删除档位
          </NButton>
        </template>
      </div>
      <NButton
        v-if="!fixed"
        size="small"
        dashed
        :disabled="disabled"
        @click="drop.quantities.push({ amount: 0, weight: 1 })"
      >
        添加数量档位
      </NButton>
    </div>
    <NButton dashed :disabled="disabled || value.length >= 32" @click="add">添加掉落道具</NButton>
    <p v-if="!fixed" class="text-xs text-gray-500">
      每种道具独立抽取一个数量档位，权重按实际总和计算。数量 0 表示未掉落。
    </p>
  </div>
</template>
