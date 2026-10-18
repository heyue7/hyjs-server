<script setup lang="ts">
import type { FieldPoint } from '@/service/fields';
defineProps<{ value: FieldPoint[]; disabled?: boolean; groups: { label: string; value: string }[] }>();
</script>

<template>
  <NCollapse>
    <NCollapseItem v-for="point in value" :key="point.id" :name="point.id" :title="`${point.id} · 坐标 ${point.point}`">
      <div v-for="(choice, i) in point.choices" :key="i" class="mb-2 flex items-center gap-2">
        <NSelect v-model:value="choice.id" :options="groups" :disabled="disabled" filterable class="flex-1" />
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
        添加候选组
      </NButton>
    </NCollapseItem>
  </NCollapse>
</template>
