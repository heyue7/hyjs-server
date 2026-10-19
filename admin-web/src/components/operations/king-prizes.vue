<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { api, errorMessage } from '@/service/operations';
import type { ActivityPrize, KingRecruitResource } from '@/service/activity-operations';
import ResourceSelect from './resource-select.vue';
import RewardSummary from './reward-summary.vue';

const rows = defineModel<ActivityPrize[]>({ required: true });
const props = defineProps<{ disabled?: boolean; readonly?: boolean }>();
const resources = ref<KingRecruitResource[]>([]);
const loading = ref(false);
const loadError = ref('');
let generation = 0;
let nextId = 1;
const resourceMap = computed(() => new Map(resources.value.map(item => [item.id, item] as const)));
const pickerResources = computed(() => resources.value.map(item => ({ ...item, applicableType: `king.world:${item.space}`, applicableTypes: [`king.world:${item.space}`], applicableTypeLabel: item.spaceLabel })));
const editingDisabled = computed(() => Boolean(props.disabled || props.readonly || loading.value || loadError.value));
const worlds = computed(() => [...new Map(resources.value.map(item => [item.space, item.spaceLabel] as const)).entries()].sort(([a], [b]) => a - b));
const missingGuarantees = computed(() => rows.value.length ? worlds.value.filter(([space]) => !rows.value.some(row => row.guaranteed && Number(resourceMap.value.get(row.resourceId)?.quality) === 6 && resourceMap.value.get(row.resourceId)?.space === space)).map(([, label]) => label) : []);
async function loadResources() {
  const current = ++generation;
  loading.value = true;
  loadError.value = '';
  try {
    const result = await api<{ items: KingRecruitResource[] }>('/activity-king-resources');
    if (current === generation) resources.value = result.items;
  } catch (error) {
    if (current === generation) loadError.value = errorMessage(error);
  } finally {
    if (current === generation) loading.value = false;
  }
}
function add() {
  if (editingDisabled.value) return;
  const id = Math.max(nextId, 1, ...rows.value.map(row => row.id + 1));
  nextId = id + 1;
  rows.value = [...rows.value, { id, resourceId: '', amount: 1, weight: 1, stock: 0, guaranteed: false }];
}
function changeResource(row: ActivityPrize, resourceId: string) {
  if (editingDisabled.value) return;
  row.resourceId = resourceId;
  row.amount = 1;
  row.stock = 0;
  if (Number(resourceMap.value.get(resourceId)?.quality) !== 6) row.guaranteed = false;
}
function share(row: ActivityPrize) {
  const resource = resourceMap.value.get(row.resourceId);
  if (!resource) return '';
  const total = rows.value.filter(prize => resourceMap.value.get(prize.resourceId)?.space === resource.space).reduce((sum, prize) => sum + (prize.weight || 0), 0);
  return total > 0 ? `同世界权重占比 ${((row.weight || 0) * 100 / total).toFixed(2)}%` : '请设置正整数权重';
}
onMounted(loadResources);
onBeforeUnmount(() => { generation += 1; });
</script>

<template>
  <div class="king-prizes">
    <div class="ops-section-heading">
      <h3>王者招募角色池</h3>
      <NButton v-if="!readonly" size="small" :disabled="editingDisabled || !resources.length" @click="add">添加角色</NButton>
    </div>
    <p class="ops-inline-note">每次获得 1 名角色，不限库存。按所属世界分别抽取，保底候选仅限红色角色。</p>
    <NAlert v-if="loadError" type="error" :title="loadError" class="mb-16px"><NButton size="small" @click="loadResources">重新加载角色</NButton></NAlert>
    <p v-if="!loading && !loadError && missingGuarantees.length" class="ops-inline-note">尚缺红色保底候选：{{ missingGuarantees.join('、') }}。</p>
    <NSpin :show="loading">
      <NEmpty v-if="!rows.length" description="尚未配置招募角色" />
      <section v-for="(row, index) in rows" :key="row.id" class="king-prize-row">
        <template v-if="readonly">
          <RewardSummary :rewards="row.resourceId ? [{ resourceId: row.resourceId, amount: row.amount }] : []" />
          <div class="king-prize-details">
            <NTag v-if="resourceMap.get(row.resourceId)" size="small" :bordered="false">{{ resourceMap.get(row.resourceId)?.spaceLabel }}</NTag>
            <NTag v-if="row.guaranteed" size="small" :bordered="false" type="info">红色保底候选</NTag>
            <span>权重 {{ row.weight }}</span><span>{{ share(row) }}</span>
          </div>
        </template>
        <template v-else>
          <div class="king-prize-heading"><strong>角色 {{ index + 1 }}</strong><NButton size="small" quaternary :disabled="editingDisabled" @click="rows = rows.filter(prize => prize.id !== row.id)">移除</NButton></div>
          <div class="king-prize-fields">
            <NFormItem label="招募角色" required>
              <div class="king-prize-resource">
                <ResourceSelect :value="row.resourceId" :resources="pickerResources" category="character" :disabled="editingDisabled" placeholder="搜索可招募的橙色或红色角色" @update:value="changeResource(row, $event)" />
                <span v-if="resourceMap.get(row.resourceId)" class="ops-muted">{{ resourceMap.get(row.resourceId)?.spaceLabel }}</span>
                <span v-else-if="row.resourceId && !loading && !loadError" class="ops-muted">当前角色不在可招募范围，请重新选择。</span>
              </div>
            </NFormItem>
            <NFormItem label="相对权重" required><NInputNumber v-model:value="row.weight" :min="1" :precision="0" :disabled="editingDisabled" class="w-full" /></NFormItem>
            <NFormItem label="红色保底候选">
              <NCheckbox v-model:checked="row.guaranteed" :disabled="editingDisabled || (Number(resourceMap.get(row.resourceId)?.quality) !== 6 && !row.guaranteed)">参与保底</NCheckbox>
            </NFormItem>
          </div>
          <p v-if="row.resourceId" class="ops-inline-note king-prize-share">{{ share(row) }}；不含保底规则。</p>
        </template>
      </section>
    </NSpin>
  </div>
</template>

<style scoped>
.king-prize-row { padding: 16px 0; border-top: 1px solid rgb(var(--base-text-color) / .09); }
.king-prize-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.king-prize-heading strong { font-size: 13px; font-weight: 500; }
.king-prize-fields { display: grid; grid-template-columns: minmax(0, 1fr) 130px 120px; gap: 16px; }
.king-prize-resource { display: flex; flex-direction: column; gap: 6px; width: 100%; min-width: 0; }
.king-prize-share { margin: 0; }
.king-prize-details { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-top: 10px; font-size: 13px; color: rgb(var(--base-text-color) / .65); }
@media (max-width: 760px) { .king-prize-fields { grid-template-columns: minmax(0, 1fr) 110px; } .king-prize-fields > :first-child { grid-column: 1 / -1; } }
</style>
