<script setup lang="ts">
import { computed, ref } from 'vue';
import type { ActivityPrize } from '@/service/activity-operations';
import { useActivitySelectorSummary } from '@/service/activity-selector-summary';
import ResourceSelect from './resource-select.vue';
const rows = defineModel<ActivityPrize[]>({ required: true });
defineProps<{ disabled?: boolean }>();
const activeIndex = ref(0);
const selected = computed(() => rows.value.slice(activeIndex.value, activeIndex.value + 1));
const { rewardsText, failed, loading, retry } = useActivitySelectorSummary(() => rows.value);
const totalWeight = computed(() => rows.value.reduce((sum, row) => sum + (row.weight || 0), 0));
function share(row: ActivityPrize) {
  return totalWeight.value ? `${((row.weight / totalWeight.value) * 100).toFixed(2)}%` : '待设置权重';
}
</script>

<template>
  <div>
    <div class="ops-section-heading">
      <h3>九个普通奖位</h3>
      <span class="ops-muted">权重合计 {{ totalWeight.toLocaleString() }}</span>
    </div>
    <div class="ops-muted mb-16px">设置各奖位的相对权重；占比不包含保底及超级大奖规则。</div>
    <div class="prize-navigation" aria-label="选择转盘奖位">
      <button v-for="(row, index) in rows" :key="row.id" type="button" :class="{ selected: activeIndex === index }" :aria-pressed="activeIndex === index" @click="activeIndex = index"><strong>{{ index + 1 }}. {{ rewardsText([row]) }}</strong><span>{{ share(row) }}{{ row.guaranteed ? ' · 保底候选' : '' }} · 库存 {{ row.stock || '不限' }}</span></button>
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
      <div class="qualification-line">
        <span>可作为保底奖励</span>
        <NSwitch v-model:value="row.guaranteed" :disabled="disabled" />
      </div>
    </section>
  </div>
</template>

<style scoped>
.prize-navigation { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 9px; }
.prize-navigation button { display: flex; flex-direction: column; gap: 7px; text-align: left; padding: 14px; background: rgb(var(--base-text-color) / .025); border: 1px solid transparent; border-radius: 7px; cursor: pointer; color: rgb(var(--base-text-color)); }
.prize-navigation button.selected { border-color: rgb(var(--primary-color) / .4); background: rgb(var(--primary-color) / .06); }
.prize-navigation button:hover { background: rgb(var(--primary-color) / .06); }
.prize-navigation button:focus-visible { outline: 2px solid rgb(var(--primary-color) / .6); outline-offset: 2px; }
.prize-navigation strong { overflow-wrap: anywhere; font-size: 13px; font-weight: 500; }
.prize-navigation span { font-size: 12px; color: rgb(var(--base-text-color) / .5); }
@media (max-width: 640px) { .prize-navigation { grid-template-columns: 1fr; } }
</style>
