<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { useOperations } from '@/composables/useOperations';
import { errorMessage } from '@/service/operations';
import { getNativeActivities, saveNativeActivity } from '@/service/native-activities';
import type { NativeActivity } from '@/service/native-activities';
import ActivityProducts from './activity-products.vue';
import ActivityMilestones from './activity-milestones.vue';
import RewardList from './reward-list.vue';
import ResourceSelect from './resource-select.vue';

const { canWrite } = useOperations();
const rows = ref<NativeActivity[]>([]);
const selected = ref<NativeActivity>();
const original = ref('');
const search = ref('');
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const selectedVIP = ref(1);
const selectedPrize = ref(0);
const selectedMilestone = ref(0);
const selectedChild = ref(0);
const maxAmount = 2147483647;
const dirty = computed(() => Boolean(selected.value) && JSON.stringify(selected.value) !== original.value);
const disabled = computed(() => !canWrite.value || saving.value || loading.value);
const rulesDisabled = computed(() => disabled.value || selected.value?.frozen);
const filtered = computed(() => rows.value.filter(row => `${row.name} ${row.alias || ''}`.includes(search.value.trim())));
const options = computed(() => filtered.value.map(row => ({ value: row.id, label: `${row.name}${row.alias ? ` · ${row.alias}` : ''}${row.form.startAt ? ` · ${new Date(row.form.startAt).toLocaleDateString('zh-CN')}` : ''}` })));
function fallbackOption(value: string | number) {
  const row = rows.value.find(item => item.id === value);
  return { value, label: row ? row.name + (row.alias ? ` · ${row.alias}` : '') : '请选择活动' };
}
const payment = computed(() => selected.value?.kind.startsWith('payment-'));
const card = computed(() => ['month-card', 'permanent-card'].includes(selected.value?.kind || ''));
const timed = computed(() => Boolean(selected.value) && !card.value && !['first-double', 'vip-benefits'].includes(selected.value!.kind));
const currentVIP = computed(() => selected.value?.form.vipTiers.filter(row => row.vip === selectedVIP.value) || []);
const currentPrize = computed(() => selected.value?.form.prizes.slice(selectedPrize.value, selectedPrize.value + 1) || []);
const currentMilestone = computed(() => selected.value?.form.milestones.slice(selectedMilestone.value, selectedMilestone.value + 1) || []);
const currentChild = computed(() => selected.value?.form.children.slice(selectedChild.value, selectedChild.value + 1) || []);
const milestoneOptions = computed(() => {
  const row = selected.value;
  if (!row) return [];
  return row.form.milestones.map((tier, index) => ({ value: index, label: `${tier.key.startsWith('everyoneBenefit:') ? '全民福利' : row.kind === 'fund' ? '基金等级' : '登录目标'} · ${tier.goal}${tier.key.startsWith('everyoneBenefit:') ? ' 人' : row.kind === 'fund' ? ' 级' : ' 天'}` }));
});
const vipOptions = Array.from({ length: 13 }, (_, value) => ({ value, label: `VIP${value}` }));

function apply(row: NativeActivity) {
  const draft: NativeActivity = JSON.parse(JSON.stringify(row));
  draft.alias ||= '';
  draft.form.rewards ||= [];
  draft.form.milestones ||= [];
  draft.form.prizes ||= [];
  draft.form.vipTiers ||= [];
  draft.form.children ||= [];
  selected.value = draft;
  original.value = JSON.stringify(draft);
  selectedPrize.value = 0;
  selectedMilestone.value = 0;
  selectedChild.value = 0;
}
function discard(): Promise<boolean> {
  if (!dirty.value) return Promise.resolve(true);
  return new Promise(resolve => {
    if (!window.$dialog) return resolve(false);
    window.$dialog.warning({ title: '放弃尚未保存的配置？', positiveText: '放弃修改', negativeText: '继续编辑', onPositiveClick: () => resolve(true), onNegativeClick: () => resolve(false), onClose: () => resolve(false), onMaskClick: () => resolve(false) });
  });
}
async function select(id: string) {
  if (disabled.value || !await discard()) return;
  const row = rows.value.find(item => item.id === id);
  if (row) { apply(row); error.value = ''; }
}
async function load() {
  if (loading.value || saving.value || !await discard()) return;
  loading.value = true;
  error.value = '';
  try {
    const result = await getNativeActivities();
    rows.value = result.items;
    const row = rows.value.find(item => item.id === selected.value?.id) || rows.value[0];
    if (row) apply(row);
  } catch (cause) { error.value = errorMessage(cause); }
  finally { loading.value = false; }
}
async function save() {
  if (!selected.value || disabled.value) return;
  saving.value = true;
  error.value = '';
  try {
    const result = await saveNativeActivity(selected.value);
    rows.value = rows.value.map(row => row.id === result.id ? result : row);
    apply(result);
    window.$message?.success('活动配置已保存');
  } catch (cause) { error.value = errorMessage(cause); }
  finally { saving.value = false; }
}
function weekday(day: number, on: boolean) {
  if (!selected.value) return;
  const mask = selected.value.form.weekdayMask || 0;
  selected.value.form.weekdayMask = on ? mask | (1 << day) : mask & ~(1 << day);
}
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value) { event.preventDefault(); event.returnValue = ''; } }
onBeforeRouteLeave(async () => !saving.value && await discard());
onMounted(() => { window.addEventListener('beforeunload', beforeUnload); load(); });
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));
defineExpose({ beforeLeave: discard });
</script>

<template>
  <section class="native-settings ops-page">
    <div class="ops-section-heading"><div><h2>固定活动配置</h2><p class="ops-muted">管理精彩活动中的常驻福利、支付礼包及独立活动目录。</p></div><NButton :loading="loading" :disabled="saving" @click="load">刷新</NButton></div>
    <NAlert v-if="error" type="error" class="mb-16px" :show-icon="false">{{ error }}</NAlert>
    <div class="native-picker" role="search" aria-label="查找固定活动"><NInput v-model:value="search" clearable placeholder="搜索活动名称或后台别名" aria-label="搜索固定活动" /><NSelect :value="selected?.id" :options="options" :fallback-option="fallbackOption" filterable :disabled="disabled" placeholder="选择活动配置" aria-label="选择固定活动" @update:value="select" /></div>
    <NEmpty v-if="!loading && !rows.length && !error" description="暂无固定活动配置" />
    <NSpin :show="loading">
      <template v-if="selected">
        <NAlert v-if="selected.note" type="info" :show-icon="false" class="my-16px">{{ selected.note }}</NAlert>
        <NAlert v-if="selected.frozen && selected.kind !== 'first-double'" type="warning" :show-icon="false" class="mb-16px">本期规则已被使用并冻结，可以调整启停及后台别名。</NAlert>
        <NForm label-placement="top">
          <NGrid cols="1 m:2" responsive="screen" :x-gap="20">
            <NGi><NFormItem label="活动名称"><NInput :value="selected.name" disabled /></NFormItem></NGi>
            <NGi><NFormItem label="后台别名"><NInput v-model:value="selected.alias" :disabled="disabled" :maxlength="80" placeholder="仅供后台区分，不影响游戏名称" /></NFormItem></NGi>
          </NGrid>
          <div class="qualification-line mb-20px"><span>活动启用</span><NSwitch v-model:value="selected.enabled" :disabled="disabled" /></div>
          <NGrid v-if="timed" cols="1 m:3" responsive="screen" :x-gap="20">
            <NGi><NFormItem label="开始时间"><NDatePicker v-model:value="selected.form.startAt" type="datetime" :disabled="rulesDisabled" class="w-full" /></NFormItem></NGi>
            <NGi><NFormItem label="结束时间"><NDatePicker v-model:value="selected.form.endAt" type="datetime" :disabled="rulesDisabled" class="w-full" /></NFormItem></NGi>
            <NGi v-if="!['copy-benefits', 'recruit-rotation'].includes(selected.kind)"><NFormItem label="领奖截止"><NDatePicker v-model:value="selected.form.claimEndAt" type="datetime" :disabled="rulesDisabled" class="w-full" /></NFormItem></NGi>
          </NGrid>
          <template v-if="payment">
            <NFormItem :label="selected.kind === 'payment-consume' ? '累计消费钻石' : selected.kind === 'payment-first' ? '首笔确认订单（固定 1 次）' : selected.kind === 'payment-continued' ? '连续充值日数（固定 3 天）' : '充值门槛（分）'"><NInputNumber v-model:value="selected.form.threshold" :min="1" :max="maxAmount" :precision="0" :disabled="rulesDisabled || ['payment-first', 'payment-continued'].includes(selected.kind)" /></NFormItem>
            <NFormItem label="活动奖励"><RewardList v-model:value="selected.form.rewards" :disabled="rulesDisabled" /></NFormItem>
          </template>
          <template v-if="card">
            <NGrid cols="1 m:3" responsive="screen" :x-gap="20">
              <NGi><NFormItem label="售价（分）"><NInputNumber v-model:value="selected.form.cents" :min="1" :max="maxAmount" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
              <NGi><NFormItem label="购买时钻石"><NInputNumber v-model:value="selected.form.charge" :min="0" :max="maxAmount" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
              <NGi><NFormItem label="每日钻石"><NInputNumber v-model:value="selected.form.dailyDiamond" :min="1" :max="maxAmount" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
            </NGrid>
            <NFormItem label="购买时附加奖励"><RewardList v-model:value="selected.form.rewards" :disabled="rulesDisabled" /></NFormItem>
          </template>
          <template v-if="selected.kind === 'fund'">
            <NGrid cols="1 m:2" responsive="screen" :x-gap="20">
              <NGi><NFormItem label="购买所需会员等级"><NInputNumber v-model:value="selected.form.minVip" :min="0" :max="12" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
              <NGi><NFormItem label="购买消耗钻石"><NInputNumber v-model:value="selected.form.diamondCost" :min="1" :max="maxAmount" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
            </NGrid>
          </template>
          <template v-if="['fund', 'celebration'].includes(selected.kind)">
            <NSelect v-model:value="selectedMilestone" :options="milestoneOptions" aria-label="选择固定奖励档" class="mb-20px" />
            <section v-for="tier in currentMilestone" :key="tier.key" class="activity-edit-section">
              <NFormItem :label="tier.key.startsWith('everyoneBenefit:') ? '真实购买人数' : selected.kind === 'fund' ? '角色等级' : '登录日数'"><NInputNumber v-model:value="tier.goal" :min="1" :max="maxAmount" :precision="0" :disabled="rulesDisabled || selected.kind === 'fund' && !tier.key.startsWith('everyoneBenefit:')" /></NFormItem>
              <NFormItem label="奖励"><RewardList v-model:value="tier.rewards" :disabled="rulesDisabled" /></NFormItem>
            </section>
          </template>
          <template v-if="selected.kind === 'vip-benefits'">
            <NSelect v-model:value="selectedVIP" :options="vipOptions" aria-label="选择会员福利档" class="mb-20px" />
            <section v-for="tier in currentVIP" :key="tier.vip" class="activity-edit-section">
              <NFormItem label="每日奖励"><RewardList v-model:value="tier.dailyRewards" :disabled="rulesDisabled" /></NFormItem>
              <NFormItem label="每周礼包钻石成本"><NInputNumber v-model:value="tier.weekCost.amount" :min="1" :max="maxAmount" :precision="0" :disabled="rulesDisabled" /></NFormItem>
              <NFormItem label="每周礼包奖励"><RewardList v-model:value="tier.weekRewards" :disabled="rulesDisabled" /></NFormItem>
            </section>
          </template>
          <template v-if="selected.kind === 'copy-benefits'">
            <NFormItem label="每周开放日"><NSpace><NCheckbox v-for="(label, day) in ['周日', '周一', '周二', '周三', '周四', '周五', '周六']" :key="day" :checked="Boolean(selected.form.weekdayMask & (1 << day))" :disabled="rulesDisabled" @update:checked="weekday(day, $event)">{{ label }}</NCheckbox></NSpace></NFormItem>
            <NGrid cols="1 m:2" responsive="screen" :x-gap="20">
              <NGi><NFormItem label="每日开始（距零点分钟）"><NInputNumber v-model:value="selected.form.startMinute" :min="0" :max="1439" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
              <NGi><NFormItem label="每日结束（距零点分钟）"><NInputNumber v-model:value="selected.form.endMinute" :min="1" :max="1440" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
              <NGi><NFormItem label="金币与经验倍数"><NInputNumber v-model:value="selected.form.currencyFactor" :min="1" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
              <NGi><NFormItem label="掉落倍数"><NInputNumber v-model:value="selected.form.dropFactor" :min="1" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
            </NGrid>
          </template>
          <template v-if="selected.kind === 'recruit-rotation'">
            <NSelect v-model:value="selectedPrize" :options="selected.form.prizes.map((row, index) => ({ value: index, label: `角色 ${index + 1}${row.guaranteed ? ' · 橙色保底' : ''}` }))" aria-label="选择橙卡招募角色" class="mb-20px" />
            <section v-for="prize in currentPrize" :key="prize.id" class="activity-edit-section">
              <NFormItem label="角色"><ResourceSelect v-model:value="prize.resourceId" category="character" :disabled="rulesDisabled" /></NFormItem>
              <NFormItem label="相对权重"><NInputNumber v-model:value="prize.weight" :min="1" :max="1000000000000" :precision="0" :disabled="rulesDisabled" /></NFormItem>
              <NFormItem label="橙色保底候选"><NSwitch v-model:value="prize.guaranteed" :disabled="rulesDisabled" /></NFormItem>
            </section>
          </template>
          <template v-if="['festival', 'update'].includes(selected.kind)">
            <NSelect v-model:value="selectedChild" :options="selected.form.children.map((child, index) => ({ value: index, label: child.form.name }))" aria-label="选择子活动" class="mb-20px" />
            <section v-for="child in currentChild" :key="child.id" class="activity-edit-section">
              <NFormItem label="子活动名称"><NInput :value="child.form.name" disabled /></NFormItem>
              <ActivityProducts v-if="child.domain === 'commerce'" v-model="child.form.products" :vip-options="vipOptions" :disabled="rulesDisabled" />
              <ActivityMilestones v-else v-model="child.form.milestones" mode="login-total" :disabled="rulesDisabled" :max="31" />
            </section>
          </template>
          <div class="native-footer"><span class="ops-muted">{{ dirty ? '有尚未保存的修改' : '配置已载入' }}</span><NButton type="primary" :loading="saving" :disabled="disabled || !dirty" @click="save">保存配置</NButton></div>
        </NForm>
      </template>
    </NSpin>
  </section>
</template>

<style scoped>
.native-settings { padding: 24px; }
.native-picker { display: grid; grid-template-columns: minmax(180px, 1fr) minmax(250px, 2fr); gap: 16px; margin: 20px 0; }
.native-footer { display: flex; justify-content: space-between; align-items: center; border-top: 1px solid rgb(var(--base-text-color) / .08); padding-top: 20px; margin-top: 20px; }
@media (max-width: 720px) { .native-picker { grid-template-columns: 1fr; } .native-settings { padding: 16px; } }
</style>
