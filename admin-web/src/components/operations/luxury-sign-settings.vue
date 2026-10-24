<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import type { DialogReactive } from 'naive-ui';
import { useOperations } from '@/composables/useOperations';
import { ApiError, errorMessage } from '@/service/operations';
import type { ResourceAmount } from '@/service/operations';
import { getLuxurySignRules, saveLuxurySignRules } from '@/service/luxury-sign';
import type { LuxurySignRules } from '@/service/luxury-sign';
import RewardList from './reward-list.vue';

const { canWrite } = useOperations();
const snapshot = ref<LuxurySignRules>();
const enabled = ref(false);
const rewards = ref<ResourceAmount[]>([]);
const original = ref('');
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const conflict = ref(false);
let alive = true;
const windowStatus = computed(() => {
  if (!snapshot.value?.enabled) return '';
  if (snapshot.value.endAt <= Date.now()) return '当前配置已到期，保存开启后将常驻开放。';
  if (snapshot.value.startAt > Date.now()) return `将于 ${new Date(snapshot.value.startAt).toLocaleString('zh-CN')} 开放。`;
  return '';
});
const content = () => JSON.stringify({ enabled: enabled.value, rewards: rewards.value });
const dirty = computed(() => Boolean(snapshot.value) && content() !== original.value);
const canReopen = computed(() => Boolean(snapshot.value?.enabled && enabled.value && snapshot.value.endAt <= Date.now()));
const busy = computed(() => loading.value || saving.value);
const disabled = computed(() => !canWrite.value || busy.value || conflict.value);
function apply(data: LuxurySignRules) {
  snapshot.value = data;
  enabled.value = data.enabled;
  rewards.value = data.rewards.map(row => ({ resourceId: row.resourceId, amount: row.amount }));
  original.value = content();
  conflict.value = false;
}
let pendingDiscard: { promise: Promise<boolean>; dispose: () => void } | null = null;
function confirmDiscard(): Promise<boolean> {
  if (pendingDiscard) return pendingDiscard.promise;
  if (!window.$dialog) return Promise.resolve(false);
  let resolveConfirmation!: (value: boolean) => void;
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
    title: '放弃豪华签到修改？',
    content: '尚未保存的修改将丢失。',
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
async function beforeLeave() {
  if (busy.value) return false;
  if (!canWrite.value || !dirty.value) return true;
  const accepted = await confirmDiscard();
  if (!alive) return false;
  if (accepted && snapshot.value) {
    const wasConflict = conflict.value;
    apply(snapshot.value);
    conflict.value = wasConflict;
  }
  return accepted;
}

async function load() {
  if (busy.value) return;
  if (!await beforeLeave()) return;
  loading.value = true;
  error.value = '';
  try { const data = await getLuxurySignRules(); if (alive) apply(data); }
  catch (cause) { if (alive) error.value = errorMessage(cause); }
  finally { if (alive) loading.value = false; }
}
async function save() {
  if (!snapshot.value || disabled.value || (!dirty.value && !canReopen.value)) return;
  error.value = '';
  if (enabled.value && !rewards.value.length) { error.value = '请先配置签到奖励。'; return; }
  if (rewards.value.some(row => !row.resourceId || !Number.isInteger(row.amount) || row.amount < 1 || row.amount > 2147483647)) {
    error.value = '请选择奖励，数量需为 1–2,147,483,647 的整数。'; return;
  }
  saving.value = true;
  try {
    const data = await saveLuxurySignRules({ version: snapshot.value.version, enabled: enabled.value, rewards: rewards.value.map(row => ({ ...row })) });
    if (alive) { apply(data); window.$message?.success('豪华签到配置已保存'); }
  } catch (cause) {
    if (alive) { error.value = errorMessage(cause); conflict.value = cause instanceof ApiError && cause.status === 409; }
  } finally { if (alive) saving.value = false; }
}
onMounted(load);
onBeforeUnmount(() => { alive = false; pendingDiscard?.dispose(); });
defineExpose({ beforeLeave, dirty, busy });
</script>

<template>
  <section class="ops-page luxury-sign-settings">
    <div class="ops-toolbar"><NSpace align="center" :size="8">
      <NTag v-if="dirty && canWrite" type="warning" :bordered="false">未保存</NTag>
      <NTag v-if="!canWrite" :bordered="false">只读</NTag>
      <NButton :loading="loading" :disabled="busy" @click="load">刷新</NButton>
    </NSpace></div>
    <NAlert v-if="error" type="error" :title="error" />
    <NAlert v-if="conflict" type="warning" title="配置已被其他操作更新。当前输入已保留，请刷新后重新编辑。" />
    <template v-if="snapshot">
      <NAlert v-if="windowStatus" type="info" :title="windowStatus" class="mb-16px" />
      <NForm label-placement="top">
        <NFormItem label="开放签到"><NSpace align="center"><NSwitch v-model:value="enabled" :disabled="disabled" aria-label="开放豪华签到" /><span class="ops-muted">{{ enabled ? '已开启' : '已关闭' }}</span></NSpace></NFormItem>
        <NFormItem label="当日充值门槛"><span>当日确认支付累计满 {{ (snapshot.rechargeCents / 100).toLocaleString('zh-CN') }} 元</span></NFormItem>
        <NFormItem label="每日奖励"><RewardList v-model:value="rewards" :disabled="disabled" /></NFormItem>
      </NForm>
      <div class="luxury-sign-footer"><span class="ops-muted">每天限领一次，已领取记录保留。</span><NButton v-if="canWrite" type="primary" :loading="saving" :disabled="disabled || (!dirty && !canReopen)" @click="save">保存配置</NButton></div>
    </template>
    <NSpin v-else-if="loading" class="luxury-sign-loading" />
    <NEmpty v-else description="签到配置尚未载入，请刷新重试" />
  </section>
</template>

<style scoped>
.luxury-sign-footer { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 16px; padding-top: 16px; border-top: 1px solid rgb(var(--base-text-color) / .08); }
.luxury-sign-loading { display: block; margin: 48px auto; }
.luxury-sign-settings :deep(.reward-list) { width: 100%; }
@media (max-width: 640px) {
  .luxury-sign-settings :deep(.reward-list-line) { grid-template-columns: minmax(0, 1fr) auto; }
  .luxury-sign-settings :deep(.reward-list-line > :first-child) { grid-column: 1 / -1; }
}
</style>
