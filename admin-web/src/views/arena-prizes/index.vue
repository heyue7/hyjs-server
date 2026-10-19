<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { onBeforeRouteLeave, useRouter } from 'vue-router';
import { ApiError, api, errorMessage, json } from '@/service/operations';
import type { Item, ItemsResponse } from '@/service/operations';
import type { ArenaOperatorPool, ArenaPrizeSaveRequest, ArenaPrizeSnapshot } from '@/service/arena-prizes';
import { useOperations } from '@/composables/useOperations';
import ResourceSelect from '@/components/operations/resource-select.vue';
import RewardSummary from '@/components/operations/reward-summary.vue';

const { canWrite } = useOperations();
const router = useRouter();
const snapshot = ref<ArenaPrizeSnapshot>();
const coin = ref({ min: 3000, max: 6000 });
const pool = ref<ArenaOperatorPool>({
  enabled: false,
  startAt: 0,
  endAt: 0,
  minLevel: 10,
  dailyWins: 1,
  windowWins: 1,
  items: []
});
const resources = ref<Item[]>([]);
const original = ref('');
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const resourceError = ref('');
const conflict = ref(false);
const poolReadonly = computed(() => !canWrite.value || Boolean(snapshot.value?.operatorPoolFrozen));
const contents = () => JSON.stringify({ regularCoin: coin.value, operatorPool: pool.value });
const dirty = computed(() => Boolean(snapshot.value) && contents() !== original.value);
const poolDirty = computed(
  () => snapshot.value && JSON.stringify(pool.value) !== JSON.stringify(snapshot.value.operatorPool)
);
const totalWeight = computed(() => pool.value.items.reduce((sum, item) => sum + (item.weight || 0), 0));
const maxInteger = 2147483647;

function apply(data: ArenaPrizeSnapshot) {
  snapshot.value = data;
  coin.value = { ...data.regularCoin };
  pool.value = structuredClone(data.operatorPool);
  original.value = contents();
  conflict.value = false;
}
async function loadResources(ids: string[]) {
  resourceError.value = '';
  if (!ids.length) {
    resources.value = [];
    return;
  }
  try {
    const data = await api<ItemsResponse>(
      `/items?${new URLSearchParams({ ids: ids.join(','), page: '1', pageSize: '100' })}`
    );
    resources.value = data.items.filter(item => ids.includes(item.id));
  } catch (e) {
    resourceError.value = errorMessage(e);
  }
}
async function load() {
  if (saving.value || loading.value) return;
  loading.value = true;
  error.value = '';
  try {
    const data = await api<ArenaPrizeSnapshot>('/arena/prizes');
    apply(data);
    await loadResources(data.allowedResourceIds);
  } catch (e) {
    error.value = errorMessage(e);
  } finally {
    loading.value = false;
  }
}
function refresh() {
  if (!dirty.value) {
    load();
    return;
  }
  window.$dialog?.warning({
    title: '重新载入竞技奖励？',
    content: '当前未保存的修改将丢失。',
    positiveText: '重新载入',
    negativeText: '继续编辑',
    onPositiveClick: load
  });
}
const integer = (value: number, min = 0) => Number.isInteger(value) && value >= min && value <= maxInteger;
function validate() {
  if (!integer(coin.value.min, 1) || !integer(coin.value.max, 1) || coin.value.max < coin.value.min)
    return '金币范围需为正整数，最大值不能小于最小值';
  if (!poolDirty.value || snapshot.value?.operatorPoolFrozen) return '';
  return validatePool();
}
function validatePool() {
  const value = pool.value;
  if (value.items.length > 32) return '额外奖励池最多配置 32 项奖励';
  if (!integer(value.minLevel, 10)) return '额外奖励池最低角色等级不能小于 10';
  if (
    !integer(value.dailyWins, 1) ||
    value.dailyWins > 100 ||
    !integer(value.windowWins, 1) ||
    value.windowWins > 3100 ||
    value.windowWins < value.dailyWins
  )
    return '每日胜场限 1–100，整期胜场限 1–3100，且整期不能少于每日';
  if ((value.enabled || value.startAt || value.endAt) && (!value.startAt || value.endAt <= value.startAt))
    return '请填写完整的奖池开始、结束时间，结束需晚于开始';
  if (!value.items.length) return '额外奖励池请至少配置一项奖励；只调整常规金币时可保持额外奖励池原样';
  if (
    value.items.some(
      item =>
        !snapshot.value?.allowedResourceIds.includes(item.resourceId) ||
        !integer(item.amount, 1) ||
        !integer(item.weight, 1) ||
        item.weight > 1000000000
    )
  )
    return '请选择允许的资源，并填写正整数奖励数量和权重';
  return '';
}
async function save() {
  if (!snapshot.value || !canWrite.value || saving.value || loading.value || conflict.value || !dirty.value) return;
  error.value = validate();
  if (error.value) return;
  saving.value = true;
  const request: ArenaPrizeSaveRequest = { version: snapshot.value.version, regularCoin: { ...coin.value } };
  if (poolDirty.value && !snapshot.value.operatorPoolFrozen)
    request.operatorPool = { ...pool.value, items: pool.value.items.map(item => ({ ...item })) };
  try {
    apply(await api<ArenaPrizeSnapshot>('/arena/prizes', json('PUT', request)));
    window.$message?.success('竞技胜利翻牌配置已保存');
  } catch (e) {
    error.value = errorMessage(e);
    if (e instanceof ApiError && e.status === 409) conflict.value = true;
  } finally {
    saving.value = false;
  }
}
function leave() {
  if (!dirty.value || !canWrite.value) return true;
  return new Promise<boolean>(resolve => {
    window.$dialog?.warning({
      title: '离开竞技奖励配置？',
      content: '未保存的修改将丢失。',
      positiveText: '离开',
      negativeText: '继续编辑',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false)
    });
  });
}
function beforeUnload(event: BeforeUnloadEvent) {
  if (dirty.value && canWrite.value) {
    event.preventDefault();
    event.returnValue = '';
  }
}
onBeforeRouteLeave(leave);
onMounted(() => {
  load();
  window.addEventListener('beforeunload', beforeUnload);
});
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));
</script>

<template>
  <div class="ops-page arena-prizes-page">
    <div class="ops-toolbar">
      <div class="ops-page-label">竞技胜利翻牌</div>
      <NSpace align="center">
        <NTag v-if="dirty && canWrite" type="warning" :bordered="false">未保存</NTag>
        <NTag v-if="!canWrite" :bordered="false">只读</NTag>
        <NButton quaternary @click="router.push('/shops/popularity-rank')">声望商店奖励 →</NButton>
        <NButton :loading="loading" :disabled="saving" @click="refresh">刷新</NButton>
      </NSpace>
    </div>
    <NAlert v-if="error" type="error" :title="error" />
    <NAlert v-if="conflict" type="warning" title="配置已被其他操作更新，请先刷新后重新编辑。当前输入已保留。" />
    <NSpin :show="loading">
      <template v-if="snapshot">
        <NCard :bordered="false" class="ops-panel mb-20px">
          <h3 class="prize-section-title">常规金币</h3>
          <p class="ops-muted mb-20px">
            每次胜利生成 3 张随机金币牌，玩家选 1 张领取。
          </p>
          <NForm label-placement="top" :disabled="!canWrite || saving || loading || conflict">
            <div class="coin-range-grid">
              <NFormItem label="最少金币" required>
                <NInputNumber v-model:value="coin.min" :min="1" :max="maxInteger" :precision="0" />
              </NFormItem>
              <NFormItem label="最多金币" required>
                <NInputNumber v-model:value="coin.max" :min="1" :max="maxInteger" :precision="0" />
              </NFormItem>
            </div>
          </NForm>
          <p class="ops-muted">保存后用于下一次胜利翻牌，已生成的奖励不变。</p>
        </NCard>
        <NCard :bordered="false" class="ops-panel">
          <NCollapse>
            <NCollapseItem title="额外奖励池" name="operator-pool">
              <template #header-extra>
                <NTag :type="pool.enabled ? 'success' : 'default'" :bordered="false">
                  {{ pool.enabled ? '已启用' : '未启用' }}
                </NTag>
              </template>
              <p class="ops-muted mb-20px">
                关闭时只发放常规金币。
              </p>
              <NAlert
                v-if="snapshot.operatorPoolFrozen"
                type="info"
                title="本月奖励已开始发放，额外奖池已锁定。金币范围仍可修改。"
                class="mb-20px"
              />
              <NAlert v-if="resourceError" type="error" :title="`可选资源载入失败：${resourceError}`" class="mb-20px" />
              <NForm label-placement="top" :disabled="poolReadonly || saving || loading || conflict">
                <NFormItem label="额外奖励池状态">
                  <NSwitch v-model:value="pool.enabled" :disabled="poolReadonly || saving || conflict" />
                </NFormItem>
                <div class="pool-fields-grid">
                  <NFormItem label="开始时间">
                    <NDatePicker
                      :value="pool.startAt || null"
                      type="datetime"
                      clearable
                      @update:value="pool.startAt = $event || 0"
                    />
                  </NFormItem>
                  <NFormItem label="结束时间">
                    <NDatePicker
                      :value="pool.endAt || null"
                      type="datetime"
                      clearable
                      @update:value="pool.endAt = $event || 0"
                    />
                  </NFormItem>
                  <NFormItem label="最低角色等级">
                    <NInputNumber v-model:value="pool.minLevel" :min="10" :max="maxInteger" :precision="0" />
                  </NFormItem>
                  <NFormItem label="每日可进入额外奖励池的胜场">
                    <NInputNumber v-model:value="pool.dailyWins" :min="1" :max="100" :precision="0" />
                  </NFormItem>
                  <NFormItem label="整期可进入额外奖励池的胜场">
                    <NInputNumber v-model:value="pool.windowWins" :min="1" :max="3100" :precision="0" />
                  </NFormItem>
                </div>
                <div class="prize-pool-heading">
                  <h4>随机奖励</h4>
                  <span class="ops-muted">最多 32 项 · 权重合计 {{ totalWeight.toLocaleString() }}</span>
                </div>
                <div v-for="(item, index) in pool.items" :key="index" class="prize-pool-row">
                  <NFormItem :label="`奖励 ${index + 1}`" required>
                    <ResourceSelect
                      v-model:value="item.resourceId"
                      :resources="resources"
                      :disabled="poolReadonly || saving || conflict"
                    />
                  </NFormItem>
                  <NFormItem label="数量" required>
                    <NInputNumber v-model:value="item.amount" :min="1" :max="maxInteger" :precision="0" />
                  </NFormItem>
                  <NFormItem label="权重" required>
                    <NInputNumber v-model:value="item.weight" :min="1" :max="1000000000" :precision="0" />
                  </NFormItem>
                  <NButton
                    quaternary
                    :disabled="poolReadonly || saving || conflict"
                    @click="pool.items.splice(index, 1)"
                  >
                    移除
                  </NButton>
                </div>
                <NEmpty v-if="!pool.items.length" description="尚未配置额外奖励" class="mb-16px" />
                <NButton
                  v-if="pool.items.length < 32"
                  dashed
                  :disabled="poolReadonly || saving || conflict"
                  @click="pool.items.push({ resourceId: '', amount: 1, weight: 1 })"
                >
                  添加奖励
                </NButton>
                <p class="ops-muted mt-12px">
                  概率按单项权重 ÷
                  总权重计算。开始和结束时间需位于同一个自然月，且从未来时间开始。
                </p>
                <RewardSummary v-if="pool.items.length" class="mt-20px" :rewards="pool.items" :resources="resources" />
              </NForm>
            </NCollapseItem>
          </NCollapse>
        </NCard>
        <div class="prize-footer">
          <span class="ops-muted">{{ dirty ? '有未保存的修改' : '' }}</span>
          <NButton
            v-if="canWrite"
            type="primary"
            :loading="saving"
            :disabled="loading || conflict || !dirty"
            @click="save"
          >
            保存配置
          </NButton>
        </div>
      </template>
      <NEmpty v-else-if="!loading" description="竞技奖励配置尚未载入，请刷新重试" />
    </NSpin>
  </div>
</template>

<style scoped>
.arena-prizes-page {
  max-width: 1200px;
}
.prize-section-title {
  margin: 0 0 12px;
  font-size: 16px;
  font-weight: 600;
}
.coin-range-grid,
.pool-fields-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 24px;
}
.coin-range-grid {
  max-width: 660px;
}
.prize-pool-heading,
.prize-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
}
.prize-pool-heading {
  margin: 12px 0;
}
.prize-pool-row {
  display: grid;
  grid-template-columns: minmax(220px, 1fr) 130px 130px auto;
  gap: 14px;
  align-items: center;
}
.prize-pool-row > .n-button {
  margin-top: 5px;
}
.prize-notes {
  margin: 18px 0;
  line-height: 1.7;
}
.prize-footer {
  position: sticky;
  bottom: 0;
  padding: 16px 20px;
  margin-top: 20px;
  border-radius: 8px;
  background: rgb(var(--container-bg-color));
  border: 1px solid rgb(var(--base-text-color) / 0.08);
}
@media (max-width: 760px) {
  .coin-range-grid,
  .pool-fields-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .prize-pool-row {
    grid-template-columns: 1fr 1fr;
    gap: 0 14px;
  }
  .prize-pool-row > :first-child {
    grid-column: 1 / -1;
  }
  .prize-pool-row > .n-button {
    grid-column: 1 / -1;
    justify-self: end;
    margin: -8px 0 12px;
  }
  .prize-footer,
  .prize-pool-heading {
    flex-wrap: wrap;
  }
}
</style>
