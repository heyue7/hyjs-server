<script setup lang="ts">
import { computed, ref } from 'vue';
import type { ActivityPrize } from '@/service/activity-operations';
import { useActivitySelectorSummary } from '@/service/activity-selector-summary';
import TurntableProbability from './turntable-probability.vue';
import type { ActivityGameplayOptions } from '@/service/activity-operations';
import ResourceSelect from './resource-select.vue';
const options = defineModel<ActivityGameplayOptions>('options', { required: true });
const rows = defineModel<ActivityPrize[]>({ required: true });
defineProps<{ disabled?: boolean }>();
const activeIndex = ref(0);
const selected = computed(() => rows.value.slice(activeIndex.value, activeIndex.value + 1));
const { rewardsText, failed, loading, retry } = useActivitySelectorSummary(() => rows.value);
function hasPity(id: number) { return options.value.prizePityRules?.some(rule => rule.prizeId === id && rule.enabled) || false; }
const totalWeight = computed(() => rows.value.filter(row => !hasPity(row.id)).reduce((sum, row) => sum + (row.weight || 0), 0));
function share(row: ActivityPrize) {
  if (hasPity(row.id)) return '独立概率';
  return totalWeight.value ? `${((row.weight / totalWeight.value) * 100).toFixed(2)}%` : '待设置权重';
}
</script>

<template>
  <div>
    <div class="ops-section-heading">
      <h3>九个普通奖位</h3>
      <span class="ops-muted">承接奖位权重合计 {{ totalWeight.toLocaleString() }}</span>
    </div>
    <div class="ops-muted mb-16px">开启递增后，权重仅在关闭递增时生效。至少保留一个普通奖位关闭递增且有库存，承接剩余概率。</div>
    <div class="prize-navigation" aria-label="选择转盘奖位">
      <button v-for="(row, index) in rows" :key="row.id" type="button" :class="{ selected: activeIndex === index }" :aria-pressed="activeIndex === index" @click="activeIndex = index"><strong>{{ row.id === 0 ? '0 左上角' : `奖位 ${row.id}` }} · {{ rewardsText([row]) }}</strong><span>{{ share(row) }}{{ options.prizePityRules?.some(rule => rule.prizeId === row.id && rule.enabled) ? ' · 递增保底' : '' }} · 库存 {{ row.stock || '不限' }}</span></button>
    </div>
    <NButton v-if="failed" text size="small" :loading="loading" @click="retry">资源名称载入失败，点击重试（当前显示 ID）</NButton>
    <section v-for="row in selected" :key="row.id" class="activity-edit-section">
      <NFormItem label="奖励资源" required>
        <ResourceSelect v-model:value="row.resourceId" :disabled="disabled" />
      </NFormItem>
      <NGrid cols="1 s:3" responsive="screen" :x-gap="16">
        <NGi>
          <NFormItem label="数量">
            <NInputNumber v-model:value="row.amount" :disabled="disabled" :min="1" :precision="0" class="w-full" />
          </NFormItem>
        </NGi>
        <NGi>
          <NFormItem label="相对权重">
            <NInputNumber v-model:value="row.weight" :disabled="disabled" :min="0" :precision="0" class="w-full" />
          </NFormItem>
        </NGi>
        <NGi>
          <NFormItem label="库存（0 不限）">
            <NInputNumber v-model:value="row.stock" :disabled="disabled" :min="0" :precision="0" class="w-full" />
          </NFormItem>
        </NGi>
      </NGrid>
      <TurntableProbability v-model="options" :prize-id="row.id" :disabled="disabled" />
      <div class="ops-muted mt-12px">命中本奖位重新计数；更换奖品重计，同一奖品调概率保留进度。缺货暂停计数；补货后待保底奖位依次兑现。</div>
    </section>
  </div>
</template>

<style scoped>
.prize-navigation { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 9px; }
.prize-navigation button { display: flex; flex-direction: column; gap: 7px; text-align: left; padding: 10px 12px; background: rgb(var(--base-text-color) / .025); border: 1px solid transparent; border-radius: 6px; cursor: pointer; color: rgb(var(--base-text-color)); }
.prize-navigation button.selected { border-color: rgb(var(--primary-color) / .4); background: rgb(var(--primary-color) / .06); }
.prize-navigation button:hover { background: rgb(var(--primary-color) / .06); }
.prize-navigation button:focus-visible { outline: 2px solid rgb(var(--primary-color) / .6); outline-offset: 2px; }
.prize-navigation strong { overflow-wrap: anywhere; font-size: 13px; font-weight: 500; }
.prize-navigation span { font-size: 13px; color: rgb(var(--base-text-color) / .65); }
@media (max-width: 640px) { .prize-navigation { grid-template-columns: 1fr; } }
</style>
