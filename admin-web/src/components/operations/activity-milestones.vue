<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { ActivityMilestone } from '@/service/activity-operations';
import { activityMilestoneTarget, activityMilestoneTitle } from '@/service/activity-operations';
import { renderActivityOption, useActivitySelectorSummary } from '@/service/activity-selector-summary';
import RewardList from './reward-list.vue';
const rows = defineModel<ActivityMilestone[]>({ required: true });
const props = defineProps<{ mode?: string; disabled?: boolean; single?: boolean; max?: number; choice?: boolean; dayBased?: boolean }>();
const activeIndex = ref(0);
const selectedRows = computed(() => rows.value.slice(activeIndex.value, activeIndex.value + 1));
const target = computed(() => activityMilestoneTarget(props.mode));
const { rewardsText, failed, loading, retry } = useActivitySelectorSummary(() => rows.value.flatMap(row => row.options || row.rewards));
const milestoneOptions = computed(() => rows.value.map((row, index) => ({ value: index, label: `${activityMilestoneTitle(props.mode, row.goal, props.dayBased)} → ${row.options ? '选一：' : '获得：'}${rewardsText(row.options || row.rewards)}` })));
watch(() => rows.value.length, length => { activeIndex.value = Math.max(0, Math.min(activeIndex.value, length - 1)); });
function add() {
  activeIndex.value = rows.value.length;
  rows.value = [...rows.value, { goal: rows.value.length + 1, rewards: [] }];
}
function remove(index: number) {
  rows.value = rows.value.filter((_, i) => i !== index);
}
function switchChoice(row: ActivityMilestone, value: string) {
  if (value === 'choice') {
    row.options = [];
    row.rewards = [];
  } else {
    delete row.options;
    row.rewards = [];
  }
}
</script>

<template>
  <div>
    <div class="ops-section-heading">
      <h3>{{ dayBased ? '每日奖励' : '达成奖励' }}</h3>
      <NButton
        v-if="(!single || !rows.length) && (!max || rows.length < max)"
        size="small"
        :disabled="disabled"
        @click="add"
      >
        添加奖励档
      </NButton>
    </div>
    <NEmpty v-if="!rows.length" description="尚未配置奖励" />
    <NSelect v-if="rows.length" v-model:value="activeIndex" :options="milestoneOptions" :render-label="renderActivityOption" class="mb-20px" aria-label="选择奖励档" />
    <NButton v-if="failed" text size="small" :loading="loading" @click="retry">资源名称载入失败，点击重试（当前显示 ID）</NButton>
    <section v-for="row in selectedRows" :key="activeIndex" class="activity-edit-section">
      <div class="flex justify-end mb-12px">
        <NButton quaternary size="small" :disabled="disabled" @click="remove(activeIndex)">移除奖励档</NButton>
      </div>
      <NFormItem :label="dayBased ? '第几天' : target.label" required>
        <NInputNumber v-model:value="row.goal" :disabled="disabled" :min="1" :precision="0" class="w-full">
          <template v-if="!dayBased && target.unit" #suffix>{{ target.unit }}</template>
        </NInputNumber>
      </NFormItem>
      <p v-if="!dayBased && target.note" class="ops-inline-note">{{ target.note }}</p>
      <NFormItem v-if="choice" label="奖励方式">
        <NRadioGroup
          :value="row.options ? 'choice' : 'fixed'"
          :disabled="disabled"
          @update:value="switchChoice(row, String($event))"
        >
          <NRadioButton value="fixed">固定奖励</NRadioButton>
          <NRadioButton value="choice">单项选一</NRadioButton>
        </NRadioGroup>
      </NFormItem>
      <NFormItem v-if="row.options" label="可选奖励（玩家选其中一项）" required>
        <RewardList v-model:value="row.options" :disabled="disabled" :max="3" label="选项" />
      </NFormItem>
      <NFormItem v-else label="奖励资源" required>
        <RewardList v-model:value="row.rewards" :disabled="disabled" :max="dayBased && !choice ? 1 : 100" />
      </NFormItem>
    </section>
  </div>
</template>
