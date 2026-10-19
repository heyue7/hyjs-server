<script setup lang="ts">
import { h } from 'vue';
import type { SelectOption } from 'naive-ui';
import type { FieldPoint } from '@/service/fields';
defineProps<{ value: FieldPoint[]; disabled?: boolean; groups: { label: string; value: string; iconUrl?: string }[] }>();
const renderLabel = (option: SelectOption) => h('span', { class: 'flex items-center gap-2' }, [
  option.iconUrl ? h('img', { src: String(option.iconUrl), width: 28, height: 28, style: 'object-fit:contain', alt: '' }) : null,
  h('span', String(option.label || '请选择资源'))
]);
const pointLabel = (id: string, index: number) => `${id.startsWith('mine') ? '矿点' : id.startsWith('fish') ? '钓鱼点' : '怪物刷新点'} ${index + 1}`;
</script>

<template>
  <NCollapse>
    <NCollapseItem v-for="(point, index) in value" :key="point.id" :name="point.id" :title="`${pointLabel(point.id, index)} · 坐标 ${point.point}`">
      <div v-for="(choice, i) in point.choices" :key="i" class="mb-2 flex items-center gap-2">
        <NSelect v-model:value="choice.id" :options="groups" :render-label="renderLabel" :disabled="disabled" filterable class="flex-1" />
        <span>权重</span>
        <NInputNumber v-model:value="choice.weight" :min="1" :max="1000000" :disabled="disabled" class="w-28" />
        <NButton size="small" :disabled="disabled || point.choices.length < 2" @click="point.choices.splice(i, 1)">
          移除
        </NButton>
      </div>
      <NButton
        size="small"
        dashed
        :disabled="disabled"
        @click="
          point.choices.push({
            id: groups.find(g => !point.choices.some(c => c.id === g.value))?.value || '',
            weight: 1
          })
        "
      >
        添加刷新资源
      </NButton>
    </NCollapseItem>
  </NCollapse>
</template>
