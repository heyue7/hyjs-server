<script setup lang="ts">
import { computed, h, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NButton, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns, DialogReactive } from 'naive-ui';
import { useOperations } from '@/composables/useOperations';
import { errorMessage } from '@/service/operations';
import { getNativeActivities, removeNativeActivity, saveNativeActivity } from '@/service/native-activities';
import type { NativeActivity } from '@/service/native-activities';
import { renderActivityOption, useActivitySelectorSummary } from '@/service/activity-selector-summary';
import ActivityProducts from './activity-products.vue';
import ActivityMilestones from './activity-milestones.vue';
import RewardList from './reward-list.vue';
import ResourceSelect from './resource-select.vue';
import RecruitSettings from './recruit-settings.vue';
import DailySign from '@/views/daily-sign/index.vue';
import LuxurySignSettings from './luxury-sign-settings.vue';

const { canWrite } = useOperations();
const rows = ref<NativeActivity[]>([]);
const selected = ref<NativeActivity>();
const original = ref('');
const search = ref('');
const kindFilter = ref<string | null>(null);
const enabledFilter = ref('all');
const loading = ref(false);
const saving = ref(false);
const removingId = ref('');
const error = ref('');
const saveError = ref('');
const drawerOpen = ref(false);
const selectedVIP = ref(1);
const selectedPrize = ref(0);
const selectedMilestone = ref(0);
const selectedChild = ref(0);
const maxAmount = 2147483647;
let lastFocused: HTMLElement | null = null;
const drawerWidth = ref('min(940px, calc(100vw - 48px))');
const signTab = ref('daily');
const dailySign = ref<InstanceType<typeof DailySign> | null>(null);
const luxurySign = ref<InstanceType<typeof LuxurySignSettings> | null>(null);
const isDailySign = computed(() => selected.value?.kind === 'daily-sign');
const signBusy = computed(() => isDailySign.value && Boolean(dailySign.value?.busy || luxurySign.value?.busy));
async function beforeSignTabLeave(_name: string | number, oldName: string | number | null) {
  if (signBusy.value) return false;
  return oldName === 'daily' ? dailySign.value?.beforeLeave() ?? true : luxurySign.value?.beforeLeave() ?? true;
}
async function beforeSignLeave() {
  if (signBusy.value) return false;
  if (!await (dailySign.value?.beforeLeave() ?? true)) return false;
  return luxurySign.value?.beforeLeave() ?? true;
}
const recruitRules = ref<InstanceType<typeof RecruitSettings> | null>(null);
function syncDrawerWidth() {
  drawerWidth.value = window.innerWidth <= 800 ? '100vw' : 'min(940px, calc(100vw - 48px))';
}
const activityDirty = computed(() => Boolean(selected.value) && JSON.stringify(selected.value) !== original.value);
const isRecruitRotation = computed(() => selected.value?.kind === 'recruit-rotation');
const isPetRecruit = computed(() => selected.value?.kind === 'pet-recruit');
const isLegacyPetRecruit = computed(() => isPetRecruit.value && Boolean(selected.value?.frozen));
const ruleDirty = computed(() => isRecruitRotation.value && Boolean(recruitRules.value?.dirty));
const ruleLoading = computed(() => isRecruitRotation.value && Boolean(recruitRules.value?.loading));
const ruleInvalid = computed(() => isRecruitRotation.value && (!recruitRules.value || Boolean(recruitRules.value.invalid)));
const dirty = computed(() => activityDirty.value || ruleDirty.value || Boolean(isDailySign.value && (dailySign.value?.dirty || luxurySign.value?.dirty)));
const disabled = computed(() => !canWrite.value || saving.value || loading.value || isLegacyPetRecruit.value);
const rulesDisabled = computed(() => disabled.value || selected.value?.frozen);
const busy = computed(() => loading.value || saving.value || Boolean(removingId.value) || signBusy.value);
const timed = computed(() => Boolean(selected.value) && isTimed(selected.value!));
const payment = computed(() => selected.value?.kind.startsWith('payment-'));
const card = computed(() => ['month-card', 'permanent-card'].includes(selected.value?.kind || ''));
const currentVIP = computed(() => selected.value?.form.vipTiers.filter(row => row.vip === selectedVIP.value) || []);
const currentPrize = computed(() => selected.value?.form.prizes.slice(selectedPrize.value, selectedPrize.value + 1) || []);
const currentMilestone = computed(() => selected.value?.form.milestones.slice(selectedMilestone.value, selectedMilestone.value + 1) || []);
const currentChild = computed(() => selected.value?.form.children.slice(selectedChild.value, selectedChild.value + 1) || []);
const petPrizes = computed(() => isPetRecruit.value ? selected.value?.form.prizes || [] : []);
const validPetWeight = (weight: number) => Number.isSafeInteger(weight) && weight >= 1 && weight <= 1000000000000;
const petWeightInvalid = computed(() => isPetRecruit.value && !isLegacyPetRecruit.value && (petPrizes.value.length !== 59 || petPrizes.value.some(prize => !validPetWeight(prize.weight))));
const petTotalWeight = computed(() => petPrizes.value.reduce((sum, prize) => sum + (validPetWeight(prize.weight) ? prize.weight : 0), 0));
const petOrangeWeight = computed(() => petPrizes.value.reduce((sum, prize) => sum + (prize.guaranteed && validPetWeight(prize.weight) ? prize.weight : 0), 0));
function petProbability(weight: number) {
  if (petWeightInvalid.value || !petTotalWeight.value) return '—';
  const percent = weight / petTotalWeight.value * 100;
  return percent > 0 && percent < 0.0001 ? '<0.0001%' : `${Number(percent.toFixed(4))}%`;
}
const kindOrder = [
  'daily-sign',
  'fund',
  'payment-first',
  'payment-single',
  'payment-total',
  'payment-continued',
  'payment-consume',
  'month-card',
  'permanent-card',
  'vip-benefits',
  'copy-benefits',
  'recruit-rotation',
  'pet-recruit',
  'first-double',
  'celebration',
  'festival',
  'update'
];
const kindLabels: Record<string, string> = {
  'daily-sign': '每日签到',
  fund: '开服基金',
  'payment-first': '首充礼包',
  'payment-single': '单充送礼',
  'payment-total': '累计充值',
  'payment-continued': '连充送礼',
  'payment-consume': '累计消费',
  'month-card': '月卡奖励',
  'permanent-card': '终身卡奖励',
  'vip-benefits': '会员福利',
  'copy-benefits': '福利活动',
  'recruit-rotation': '橙卡招募',
  'pet-recruit': '精灵扭蛋',
  'first-double': '首充双倍',
  celebration: '庆典活动',
  festival: '节日活动',
  update: '新版活动'
};
const enabledOptions = [
  { label: '全部状态', value: 'all' },
  { label: '已启用', value: 'enabled' },
  { label: '已停用', value: 'disabled' }
];
const kindOptions = computed(() => {
  const counts = new Map<string, number>();
  rows.value.forEach(row => counts.set(row.kind, (counts.get(row.kind) || 0) + 1));
  return [...counts.entries()]
    .sort((a, b) => {
      const left = kindOrder.indexOf(a[0]);
      const right = kindOrder.indexOf(b[0]);
      if (left === -1 || right === -1) return kindLabel(a[0]).localeCompare(kindLabel(b[0]), 'zh-CN');
      return left - right;
    })
    .map(([kind, count]) => ({ label: `${kindLabel(kind)}（${count}）`, value: kind }));
});
const filtered = computed(() => rows.value.filter(row => {
  const query = search.value.trim().toLowerCase();
  if (query && !`${row.name} ${row.alias || ''} ${kindLabel(row.kind)}`.toLowerCase().includes(query)) return false;
  if (kindFilter.value && row.kind !== kindFilter.value) return false;
  if (enabledFilter.value === 'enabled' && !row.enabled) return false;
  if (enabledFilter.value === 'disabled' && row.enabled) return false;
  return true;
}));
const { rewardsText, failed: resourceNamesFailed, loading: resourceNamesLoading, retry: retryResourceNames } = useActivitySelectorSummary(() => {
  const form = selected.value?.form;
  if (!form) return [];
  return [
    ...form.milestones.flatMap(tier => tier.rewards),
    ...form.vipTiers.flatMap(tier => [...tier.dailyRewards, tier.weekCost, ...tier.weekRewards]),
    ...form.prizes,
    ...form.children.flatMap(child => [
      ...child.form.products.flatMap(product => [...product.rewards, product.cost]),
      ...child.form.milestones.flatMap(tier => tier.options || tier.rewards)
    ])
  ];
});
const milestoneOptions = computed(() => {
  const row = selected.value;
  if (!row) return [];
  return row.form.milestones.map((tier, index) => ({ value: index, label: `${tier.key.startsWith('everyoneBenefit:') ? '全民福利' : row.kind === 'fund' ? '基金等级' : '登录目标'} · ${tier.goal}${tier.key.startsWith('everyoneBenefit:') ? ' 人' : row.kind === 'fund' ? ' 级' : ' 天'} → 获得：${rewardsText(tier.rewards)}` }));
});
const vipOptions = Array.from({ length: 13 }, (_, value) => ({ value, label: `VIP${value}` }));
const benefitVIPOptions = computed(() => Array.from({ length: 13 }, (_, value) => {
  const tiers = selected.value?.form.vipTiers.filter(tier => tier.vip === value) || [];
  return { value, label: `VIP${value} · ${tiers.map(tier => `每日：${rewardsText(tier.dailyRewards)}；每周消耗：${rewardsText([tier.weekCost], '未配置成本')} → 获得：${rewardsText(tier.weekRewards)}`).join('；') || '未配置奖励'}` };
}));
const prizeOptions = computed(() => (selected.value?.form.prizes || []).map((row, index) => ({
  value: index,
  label: `${index + 1}. ${rewardsText([row])} · 权重 ${row.weight}${isPetRecruit.value && !isLegacyPetRecruit.value ? ` · 普通概率 ${petProbability(row.weight)}` : ''}${row.guaranteed ? isLegacyPetRecruit.value ? ' · 原保底候选' : ' · 橙色保底' : ''}`
})));
const childOptions = computed(() => (selected.value?.form.children || []).map((child, index) => ({
  value: index,
  label: `${child.form.name} · ${child.domain === 'commerce'
    ? child.form.products.map(product => `消耗：${rewardsText([product.cost], '免费领取')} → 获得：${rewardsText(product.rewards)}`).join('；')
    : child.form.milestones.map(tier => `累计登录 ${tier.goal} 天 → ${tier.options ? '选一：' : '获得：'}${rewardsText(tier.options || tier.rewards)}`).join('；') || '未配置奖励'}`
})));

const columns = computed<DataTableColumns<NativeActivity>>(() => [
  {
    title: '正式名称',
    key: 'name',
    minWidth: 165,
    render: row => h('strong', row.name)
  },
  {
    title: '后台别名',
    key: 'alias',
    width: 150,
    render: row => h('span', { class: row.alias ? '' : 'ops-muted' }, row.alias || '未设置')
  },
  {
    title: '类型 / 开放时间',
    key: 'kindPeriod',
    minWidth: 250,
    render: row =>
      h('div', { class: 'native-kind-period' }, [
        h('span', kindLabel(row.kind)),
        h('span', { class: 'ops-muted' }, openTimeText(row))
      ])
  },
  {
    title: '启用 / 冻结',
    key: 'status',
    width: 140,
    render: row =>
      h(NSpace, { size: 4 }, () => [
        h(NTag, { size: 'small', bordered: false, type: row.enabled ? 'success' : 'default' }, () => row.enabled ? '已启用' : '已停用'),
        row.frozen ? h(NTag, { size: 'small', bordered: false, type: 'warning' }, () => '规则冻结') : null
      ])
  },
  {
    title: '操作',
    key: 'actions',
    width: 150,
    fixed: 'right',
    render: row =>
      h('div', { class: 'ops-table-actions' }, [
        h(NButton, { size: 'small', secondary: true, disabled: busy.value, 'aria-label': `配置${row.name}`, onClick: () => openRow(row) }, () => '配置'),
        canWrite.value && !row.enabled && !row.permanent
          ? h(NButton, { size: 'small', quaternary: true, type: 'error', loading: removingId.value === row.id, disabled: busy.value, 'aria-label': `移除${row.name}`, onClick: () => requestRemove(row) }, () => '移除')
          : null
      ])
  }
]);

const activityNotes: Record<string, string> = {
  'vip-benefits': '需有已确认订单且VIP大于0；每日与每周领取周期保留。关闭后停止领取。',
  'first-double': '仅可调整启停和后台别名；首单资格、已生成订单与终身领取记录保留。',
  fund: '包含全民福利；已领取档位保留，未领取档位使用新配置。',
  'payment-first': '金额单位为人民币分；修改用于首次参与的玩家，已参与玩家保留原规则与领取记录。',
  'payment-single': '金额单位为人民币分；修改用于首次参与的玩家，已参与玩家保留原规则与领取记录。',
  'payment-total': '金额单位为人民币分；修改用于首次参与的玩家，已参与玩家保留原规则与领取记录。',
  'payment-continued': '连充按天数计算；修改用于首次参与的玩家，已参与玩家保留原规则与领取记录。',
  'payment-consume': '累计消费按钻石计算；修改用于首次参与的玩家，已参与玩家保留原规则与领取记录。',
  'month-card': '修改用于新订单，已确认与待支付订单的价格及奖励保留。',
  'permanent-card': '修改用于新订单，已确认与待支付订单的价格及奖励保留。',
  'copy-benefits': '未冻结时保存即生效，冻结后仅可调整启停和后台别名。',
  'recruit-rotation': '奖池、每日次数与保底用于橙卡招募，费用、免费冷却与已有进度保留；轮换冻结后仅可调整启停、别名及全局招募规则。',
  'pet-recruit': '修改后的权重用于后续抽取；已有结果、免费次数和保底进度保留。价格及免费规则沿用游戏配置。',
  celebration: '按本期累计进度与领奖周期执行，冻结后仅可调整启停和后台别名。',
  festival: '子活动类型固定，父子开放时间一致，全部配置完整后可启用。父或任一子活动冻结后仅可调整启停和别名；父子启停同步。',
  update: '子活动类型固定，父子开放时间一致，全部配置完整后可启用。父或任一子活动冻结后仅可调整启停和别名；父子启停同步。'
};
const selectedNote = computed(() => selected.value && !isLegacyPetRecruit.value ? activityNotes[selected.value.kind] || selected.value.note : '');

function kindLabel(kind: string) {
  return kindLabels[kind] || kind;
}
function isTimed(row: NativeActivity) {
  return !row.permanent;
}
function openTimeText(row: NativeActivity) {
  if (!isTimed(row)) return '常驻开放';
  if (!row.form.startAt && !row.form.endAt) return '未设置';
  return `${shortTime(row.form.startAt)} 至 ${shortTime(row.form.endAt)}`;
}
function shortTime(value: number) {
  if (!value) return '未设置';
  return new Date(value).toLocaleString('zh-CN', {
    timeZone: 'Asia/Shanghai',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false
  });
}
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
  signTab.value = 'daily';
  selectedPrize.value = 0;
  selectedMilestone.value = 0;
  selectedChild.value = 0;
  saveError.value = '';
}
let pendingDiscard: { promise: Promise<boolean>; dispose: () => void } | null = null;
function confirmDiscard(): Promise<boolean> {
  if (!dirty.value) return Promise.resolve(true);
  if (pendingDiscard) return pendingDiscard.promise;
  if (!window.$dialog) return Promise.resolve(false);
  let resolveConfirmation!: (accepted: boolean) => void;
  const promise = new Promise<boolean>(resolve => { resolveConfirmation = resolve; });
  let accepted = false;
  let decided = false;
  let settled = false;
  let dialog: DialogReactive;
  const settle = (value: boolean) => {
    if (settled) return;
    settled = true;
    pendingDiscard = null;
    resolveConfirmation(value);
  };
  const dismiss = (value: boolean) => {
    if (!decided) { accepted = value; decided = true; }
    dialog.destroy();
    return false;
  };
  dialog = window.$dialog.warning({
    title: '放弃尚未保存的活动配置？',
    content: '只丢弃本次未保存的修改，已保存配置和玩家进度不变。',
    positiveText: '放弃修改',
    negativeText: '继续编辑',
    onPositiveClick: () => dismiss(true),
    onNegativeClick: () => dismiss(false),
    onClose: () => dismiss(false),
    onMaskClick: () => dismiss(false),
    onEsc: () => dismiss(false),
    onAfterLeave: () => settle(accepted)
  });
  pendingDiscard = { promise, dispose: () => { dialog.destroy(); settle(false); } };
  return promise;
}
async function openRow(row: NativeActivity) {
  if (busy.value || drawerOpen.value) return;
  lastFocused = document.activeElement as HTMLElement | null;
  apply(row);
  error.value = '';
  drawerOpen.value = true;
  await nextTick();
}
async function closeDrawer() {
  if (saving.value || signBusy.value) return;
  if (isDailySign.value ? !await beforeSignLeave() : !await confirmDiscard()) return;
  drawerOpen.value = false;
  await load();
  await nextTick();
  if (!drawerOpen.value) lastFocused?.focus?.();
}
function drawerShow(value: boolean) {
  if (!value) closeDrawer();
}
function afterDrawerLeave() {
  if (drawerOpen.value) return;
  selected.value = undefined;
  original.value = '';
  saveError.value = '';
}
async function leaveWorkspace() {
  if (saving.value || signBusy.value) return false;
  if (isDailySign.value ? !await beforeSignLeave() : !await confirmDiscard()) return false;
  if (drawerOpen.value) {
    drawerOpen.value = false;
    selected.value = undefined;
    original.value = '';
  }
  return true;
}
async function load() {
  if (loading.value || saving.value) return;
  loading.value = true;
  error.value = '';
  try {
    const result = await getNativeActivities();
    rows.value = result.items;
  } catch (cause) {
    error.value = errorMessage(cause);
  } finally {
    loading.value = false;
  }
}
async function save() {
  if (!selected.value || disabled.value || isDailySign.value) return;
  if (petWeightInvalid.value) {
    saveError.value = '请将每个精灵的权重设为 1 至 1000000000000 的整数。';
    return;
  }
  if (isRecruitRotation.value && (ruleLoading.value || ruleInvalid.value)) {
    saveError.value = '请先修正或重新载入招募规则后再保存。';
    return;
  }
  const recruitSettings = isRecruitRotation.value ? recruitRules.value?.request() : undefined;
  saving.value = true;
  saveError.value = '';
  try {
    const result = await saveNativeActivity(selected.value, recruitSettings);
    rows.value = rows.value.map(row => row.id === result.id ? result : row);
    apply(result);
    window.$message?.success('活动配置已保存');
    if (isRecruitRotation.value) await recruitRules.value?.accept();
  } catch (cause) {
    saveError.value = errorMessage(cause);
  } finally {
    saving.value = false;
  }
}
function requestRemove(row: NativeActivity) {
  if (!canWrite.value || busy.value || row.enabled || !window.$dialog) return;
  const alias = (row.alias || '').trim();
  const identity = alias ? `“${row.name}”（后台别名：${alias}）` : `“${row.name}”`;
  let submitting = false;
  const dialog = window.$dialog.warning({
    title: '移除停用配置？',
    content: `将移除${identity}。活动源配置、玩家进度和领取记录保留；启用中的活动不能移除。`,
    positiveText: '移除配置',
    negativeText: '取消',
    positiveButtonProps: { type: 'error' },
    maskClosable: false,
    closeOnEsc: false,
    closable: false,
    onPositiveClick: async () => {
      if (submitting) return false;
      submitting = true;
      removingId.value = row.id;
      dialog.loading = true;
      dialog.negativeButtonProps = { disabled: true };
      try {
        await removeNativeActivity(row);
        rows.value = rows.value.filter(item => item.id !== row.id);
        window.$message?.success('停用配置已移除，活动源和玩家历史保留');
        await load();
        return true;
      } catch (cause) {
        window.$message?.error(errorMessage(cause));
        return false;
      } finally {
        removingId.value = '';
        submitting = false;
        dialog.loading = false;
        dialog.negativeButtonProps = { disabled: false };
      }
    }
  });
}
function weekday(day: number, on: boolean) {
  if (!selected.value) return;
  const mask = selected.value.form.weekdayMask || 0;
  selected.value.form.weekdayMask = on ? mask | (1 << day) : mask & ~(1 << day);
}
function beforeUnload(event: BeforeUnloadEvent) {
  if (dirty.value || saving.value || signBusy.value) {
    event.preventDefault();
    event.returnValue = '';
  }
}
onBeforeRouteLeave(() => leaveWorkspace());
onMounted(() => {
  syncDrawerWidth();
  window.addEventListener('resize', syncDrawerWidth);
  window.addEventListener('beforeunload', beforeUnload);
  load();
});
onBeforeUnmount(() => {
  pendingDiscard?.dispose();
  window.removeEventListener('resize', syncDrawerWidth);
  window.removeEventListener('beforeunload', beforeUnload);
});
defineExpose({ beforeLeave: leaveWorkspace, confirmDiscard, dirty, saving });
</script>

<template>
  <section class="native-settings ops-page">
    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false" class="ops-panel">
      <div class="ops-filters">
        <NInput
          v-model:value="search"
          clearable
          placeholder="搜索活动名称或后台别名"
          class="ops-search"
          aria-label="搜索活动名称或后台别名"
        />
        <NSelect
          v-model:value="kindFilter"
          :options="kindOptions"
          clearable
          placeholder="全部类型"
          class="ops-filter-select"
          aria-label="按活动类型筛选"
        />
        <NSelect
          v-model:value="enabledFilter"
          :options="enabledOptions"
          class="ops-filter-select"
          aria-label="按启停状态筛选"
        />
        <span class="ops-muted native-count">{{ filtered.length }} / {{ rows.length }} 项</span>
        <NButton :loading="loading" :disabled="busy" @click="load">刷新</NButton>
      </div>
      <NDataTable
        :columns="columns"
        :data="filtered"
        :loading="loading"
        :row-key="(row: NativeActivity) => row.id"
        :bordered="false"
        :scroll-x="855"
        :pagination="{ pageSize: 20 }"
      >
        <template #empty>
          <NEmpty :description="rows.length ? '没有符合筛选条件的活动' : '暂无固定活动配置'" />
        </template>
      </NDataTable>
    </NCard>
    <NDrawer
      :show="drawerOpen"
      :width="drawerWidth"
      placement="right"
      class="native-activity-drawer"
      :mask-closable="!saving && !signBusy"
      :close-on-esc="!saving && !signBusy"
      :trap-focus="true"
      :auto-focus="true"
      @update:show="drawerShow"
      @after-leave="afterDrawerLeave"
    >
      <NDrawerContent v-if="selected" :title="selected.name" closable :native-scrollbar="false">
        <NTabs v-if="isDailySign" v-model:value="signTab" type="line" :on-before-leave="beforeSignTabLeave">
          <NTabPane name="daily" tab="每日签到"><DailySign ref="dailySign" embedded /></NTabPane>
          <NTabPane name="luxury" tab="豪华签到" display-directive="show:lazy"><LuxurySignSettings ref="luxurySign" /></NTabPane>
        </NTabs>
        <template v-else>
        <div class="native-drawer-head">
          <NSpace :size="6">
            <NTag :bordered="false">{{ kindLabel(selected.kind) }}</NTag>
            <NTag :bordered="false" :type="selected.enabled ? 'success' : 'default'">{{ selected.enabled ? '已启用' : '已停用' }}</NTag>
            <NTag v-if="selected.permanent" :bordered="false">常驻开放</NTag>
            <NTag v-if="selected.frozen && !['first-double'].includes(selected.kind)" :bordered="false" type="warning">规则冻结</NTag>
          </NSpace>
          <div class="qualification-line native-enable">
            <span>活动启用</span>
            <NSwitch v-model:value="selected.enabled" :disabled="disabled" aria-label="活动启用" />
          </div>
        </div>

        <NAlert v-if="isLegacyPetRecruit" type="warning" :show-icon="false" class="mb-16px">{{ selected.note }}</NAlert>
        <NAlert v-else-if="selected.frozen && !['first-double', 'recruit-rotation'].includes(selected.kind)" type="warning" :show-icon="false" class="mb-16px">
          本期规则已被使用并冻结，可以调整启停及后台别名。
        </NAlert>
        <NAlert v-if="saveError" type="error" :title="saveError" class="mb-16px" />
        <NButton v-if="resourceNamesFailed" text size="small" :loading="resourceNamesLoading" @click="retryResourceNames">资源名称载入失败，点击重试（当前显示 ID）</NButton>
        <NForm label-placement="top">
          <NGrid cols="1 m:2" responsive="screen" :x-gap="20">
            <NGi><NFormItem label="正式名称"><NInput :value="selected.name" disabled /></NFormItem></NGi>
            <NGi><NFormItem label="后台别名"><NInput v-model:value="selected.alias" :disabled="disabled" :maxlength="80" placeholder="仅供后台区分，不影响游戏名称" /></NFormItem></NGi>
          </NGrid>
          <template v-if="timed">
            <div class="native-section-label">开放时间</div>
            <NGrid cols="1 m:3" responsive="screen" :x-gap="20">
              <NGi><NFormItem label="开始时间"><NDatePicker v-model:value="selected.form.startAt" type="datetime" :disabled="rulesDisabled" class="w-full" /></NFormItem></NGi>
              <NGi><NFormItem label="结束时间"><NDatePicker v-model:value="selected.form.endAt" type="datetime" :disabled="rulesDisabled" class="w-full" /></NFormItem></NGi>
              <NGi v-if="!['copy-benefits', 'recruit-rotation'].includes(selected.kind)"><NFormItem label="领奖截止"><NDatePicker v-model:value="selected.form.claimEndAt" type="datetime" :disabled="rulesDisabled" class="w-full" /></NFormItem></NGi>
            </NGrid>
          </template>
          <template v-if="payment">
            <div class="native-section-label">支付条件与奖励</div>
            <NFormItem :label="selected.kind === 'payment-consume' ? '累计消费钻石' : selected.kind === 'payment-first' ? '首笔确认订单（固定 1 次）' : selected.kind === 'payment-continued' ? '连续充值日数（固定 3 天）' : '充值门槛（分）'"><NInputNumber v-model:value="selected.form.threshold" :min="1" :max="maxAmount" :precision="0" :disabled="rulesDisabled || ['payment-first', 'payment-continued'].includes(selected.kind)" /></NFormItem>
            <NFormItem label="活动奖励"><RewardList v-model:value="selected.form.rewards" :disabled="rulesDisabled" /></NFormItem>
          </template>
          <template v-if="card">
            <div class="native-section-label">售卖与每日奖励</div>
            <NGrid cols="1 m:3" responsive="screen" :x-gap="20">
              <NGi><NFormItem label="售价（分）"><NInputNumber v-model:value="selected.form.cents" :min="1" :max="maxAmount" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
              <NGi><NFormItem label="购买时钻石"><NInputNumber v-model:value="selected.form.charge" :min="0" :max="maxAmount" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
              <NGi><NFormItem label="每日钻石"><NInputNumber v-model:value="selected.form.dailyDiamond" :min="1" :max="maxAmount" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
            </NGrid>
            <NFormItem label="购买时附加奖励"><RewardList v-model:value="selected.form.rewards" :disabled="rulesDisabled" /></NFormItem>
          </template>
          <template v-if="selected.kind === 'fund'">
            <div class="native-section-label">购买条件</div>
            <NGrid cols="1 m:2" responsive="screen" :x-gap="20">
              <NGi><NFormItem label="购买所需会员等级"><NInputNumber v-model:value="selected.form.minVip" :min="0" :max="12" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
              <NGi><NFormItem label="购买消耗钻石"><NInputNumber v-model:value="selected.form.diamondCost" :min="1" :max="maxAmount" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
            </NGrid>
          </template>
          <template v-if="['fund', 'celebration'].includes(selected.kind)">
            <div class="native-section-label">档位奖励</div>
            <NSelect v-model:value="selectedMilestone" :options="milestoneOptions" :render-label="renderActivityOption" aria-label="选择固定奖励档" class="mb-20px" />
            <section v-for="tier in currentMilestone" :key="tier.key" class="activity-edit-section">
              <NFormItem :label="tier.key.startsWith('everyoneBenefit:') ? '购买人数' : selected.kind === 'fund' ? '角色等级' : '登录日数'"><NInputNumber v-model:value="tier.goal" :min="1" :max="maxAmount" :precision="0" :disabled="rulesDisabled || selected.kind === 'fund' && !tier.key.startsWith('everyoneBenefit:')" /></NFormItem>
              <NFormItem label="奖励"><RewardList v-model:value="tier.rewards" :disabled="rulesDisabled" /></NFormItem>
            </section>
          </template>
          <template v-if="selected.kind === 'vip-benefits'">
            <div class="native-section-label">会员档位</div>
            <NSelect v-model:value="selectedVIP" :options="benefitVIPOptions" :render-label="renderActivityOption" aria-label="选择会员福利档" class="mb-20px" />
            <section v-for="tier in currentVIP" :key="tier.vip" class="activity-edit-section">
              <NFormItem label="每日奖励"><RewardList v-model:value="tier.dailyRewards" :disabled="rulesDisabled" /></NFormItem>
              <NFormItem label="每周礼包钻石成本"><NInputNumber v-model:value="tier.weekCost.amount" :min="1" :max="maxAmount" :precision="0" :disabled="rulesDisabled" /></NFormItem>
              <NFormItem label="每周礼包奖励"><RewardList v-model:value="tier.weekRewards" :disabled="rulesDisabled" /></NFormItem>
            </section>
          </template>
          <template v-if="selected.kind === 'copy-benefits'">
            <div class="native-section-label">每周开放规则</div>
            <NFormItem label="每周开放日"><NSpace><NCheckbox v-for="(label, day) in ['周日', '周一', '周二', '周三', '周四', '周五', '周六']" :key="day" :checked="Boolean(selected.form.weekdayMask & (1 << day))" :disabled="rulesDisabled" @update:checked="weekday(day, $event)">{{ label }}</NCheckbox></NSpace></NFormItem>
            <NGrid cols="1 m:2" responsive="screen" :x-gap="20">
              <NGi><NFormItem label="每日开始（距零点分钟）"><NInputNumber v-model:value="selected.form.startMinute" :min="0" :max="1439" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
              <NGi><NFormItem label="每日结束（距零点分钟）"><NInputNumber v-model:value="selected.form.endMinute" :min="1" :max="1440" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
              <NGi><NFormItem label="金币与经验倍数"><NInputNumber v-model:value="selected.form.currencyFactor" :min="1" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
              <NGi><NFormItem label="掉落倍数"><NInputNumber v-model:value="selected.form.dropFactor" :min="1" :precision="0" :disabled="rulesDisabled" /></NFormItem></NGi>
            </NGrid>
          </template>
          <template v-if="selected.kind === 'recruit-rotation'">
            <RecruitSettings
              ref="recruitRules"
              :key="selected.id"
              :kind="2"
              :disabled="disabled"
              class="native-recruit-rules"
            />
            <div class="native-section-label">橙卡招募角色</div>
            <NSelect v-model:value="selectedPrize" :options="prizeOptions" :render-label="renderActivityOption" aria-label="选择橙卡招募角色" class="mb-20px" />
            <section v-for="prize in currentPrize" :key="prize.id" class="activity-edit-section">
              <NFormItem label="角色"><ResourceSelect v-model:value="prize.resourceId" category="character" :disabled="rulesDisabled" /></NFormItem>
              <NFormItem label="相对权重"><NInputNumber v-model:value="prize.weight" :min="1" :max="1000000000000" :precision="0" :disabled="rulesDisabled" /></NFormItem>
              <NFormItem label="保底品质"><NInput value="橙卡（固定）" disabled /></NFormItem>
            </section>
          </template>
          <template v-if="isPetRecruit">
            <div class="native-section-label">精灵奖池</div>
            <div v-if="!isLegacyPetRecruit" class="native-pet-summary" aria-label="普通抽取概率汇总">
              <span>紫色 {{ petProbability(petTotalWeight - petOrangeWeight) }}</span>
              <span>橙色 {{ petProbability(petOrangeWeight) }}</span>
              <span class="ops-muted">普通概率，不含十抽保底</span>
            </div>
            <NSelect v-model:value="selectedPrize" :options="prizeOptions" :render-label="renderActivityOption" aria-label="选择精灵" class="mb-20px" />
            <section v-for="prize in currentPrize" :key="prize.id" class="activity-edit-section">
              <NFormItem label="精灵"><ResourceSelect :value="prize.resourceId" category="pet" disabled /></NFormItem>
              <NFormItem label="相对权重" :validation-status="validPetWeight(prize.weight) ? undefined : 'error'" :feedback="validPetWeight(prize.weight) ? undefined : '请输入 1 至 1000000000000 的整数'">
                <NInputNumber v-model:value="prize.weight" :min="1" :max="1000000000000" :precision="0" :disabled="rulesDisabled" />
              </NFormItem>
              <NFormItem :label="isLegacyPetRecruit ? '保底资格' : '品质'"><NInput :value="isLegacyPetRecruit ? (prize.guaranteed ? '原保底候选' : '普通候选') : (prize.guaranteed ? '橙色（保底候选）' : '紫色')" disabled /></NFormItem>
            </section>
          </template>
          <template v-if="['festival', 'update'].includes(selected.kind)">
            <div class="native-section-label">子活动</div>
            <NSelect v-model:value="selectedChild" :options="childOptions" :render-label="renderActivityOption" aria-label="选择子活动" class="mb-20px" />
            <section v-for="child in currentChild" :key="child.id" class="activity-edit-section">
              <NFormItem label="子活动名称"><NInput :value="child.form.name" disabled /></NFormItem>
              <ActivityProducts v-if="child.domain === 'commerce'" v-model="child.form.products" :vip-options="vipOptions" :disabled="rulesDisabled" />
              <ActivityMilestones v-else v-model="child.form.milestones" mode="login-total" :disabled="rulesDisabled" :max="31" />
            </section>
          </template>
        </NForm>
        </template>
        <template v-if="!isDailySign" #footer>
          <p v-if="selectedNote" class="native-save-note">{{ selectedNote }}</p>
          <div class="ops-drawer-actions">
            <span class="ops-muted">{{ saving ? '正在保存…' : dirty ? '有尚未保存的修改' : '配置已载入' }}</span>
            <NButton :disabled="saving" @click="closeDrawer">取消</NButton>
            <NButton v-if="canWrite" type="primary" :loading="saving" :disabled="disabled || !dirty || ruleLoading || ruleInvalid || petWeightInvalid" @click="save">保存配置</NButton>
          </div>
        </template>
      </NDrawerContent>
    </NDrawer>
  </section>
</template>

<style scoped>
.native-count { margin-left: auto; font-variant-numeric: tabular-nums; }
:deep(.native-kind-period) { display: flex; flex-direction: column; gap: 2px; font-variant-numeric: tabular-nums; }
.native-drawer-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  padding-bottom: 12px;
  margin-bottom: 12px;
  border-bottom: 1px solid rgb(var(--base-text-color) / .09);
}
.native-enable { margin: 0; gap: 12px; }
.native-enable span { color: rgb(var(--base-text-color) / .65); font-size: 13px; }
.native-save-note { margin: 0 0 12px; line-height: 1.6; overflow-wrap: anywhere; color: rgb(var(--base-text-color) / .65); font-size: 13px; }
.native-recruit-rules { margin-bottom: 24px; }
.native-pet-summary { display: flex; flex-wrap: wrap; gap: 4px 20px; margin: 0 0 14px; font-size: 13px; font-variant-numeric: tabular-nums; }
.native-section-label {
  margin: 4px 0 16px;
  font-size: 13px;
  font-weight: 500;
  color: rgb(var(--base-text-color) / .55);
}
:global(.n-drawer.native-activity-drawer) { max-width: 100vw; }
@media (max-width: 800px) {
  .native-count { margin-left: 0; }
  :global(.n-drawer.native-activity-drawer) { width: 100vw !important; }
  .native-drawer-head { align-items: flex-start; }
}
</style>
