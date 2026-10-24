<script setup lang="ts">
import { computed, h, onBeforeUnmount, ref, watch } from 'vue';
import { NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, ApiError, errorMessage, formatTime, json } from '@/service/operations';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { useOperations } from '@/composables/useOperations';
import { useDraftGuard } from '@/composables/useDraftGuard';
import { createRequestId } from '@/utils/request-id';
import { money } from '@/service/wallet-operations';
import type { WalletLedger, WalletLedgerItem, WalletAdjustmentRequest, WalletAdjustmentResult } from '@/service/wallet-operations';
const scope = useServerScopeStore();
const props = defineProps<{ playerId: number; playerName?: string }>();
const { canWrite } = useOperations();
const loading = ref(false);
const error = ref('');
const data = ref<WalletLedger | null>(null);
const range = ref<[number, number] | null>(null);
const kind = ref<string | null>(null);
const page = ref(1);
const pageSize = ref(20);
let generation = 0;
const action = ref<'credit' | 'debit' | null>(null);
const points = ref<number | null>(null);
const reason = ref('');
const adjustmentError = ref('');
const conflict = ref(false);
const submitting = ref(false);
const confirming = ref(false);
const pending = ref<{ body: WalletAdjustmentRequest; path: string; serverId: number; playerId: number } | null>(null);
const maxPoints = 1_000_000_000_000;
const locked = computed(() => submitting.value || confirming.value || Boolean(pending.value));
const adjustable = computed(() => canWrite.value && Boolean(props.playerId) && Boolean(scope.currentServer));
const actionLabel = computed(() => action.value === 'debit' ? '扣除' : '发放');
const validPoints = computed(() => Number.isSafeInteger(points.value) && points.value! > 0 && points.value! <= maxPoints);
const afterBalance = computed(() => data.value && validPoints.value
  ? data.value.balanceCents + (action.value === 'debit' ? -1 : 1) * points.value! * 100
  : null);
const insufficient = computed(() => afterBalance.value !== null && afterBalance.value < 0);
function resetAdjustment() {
  action.value = null; points.value = null; reason.value = ''; adjustmentError.value = ''; conflict.value = false;
}
const guard = useDraftGuard({
  active: () => action.value !== null || Boolean(pending.value),
  value: () => ({ points: points.value, reason: reason.value }),
  busy: () => locked.value,
  writable: () => canWrite.value,
  close: resetAdjustment,
  label: '点数调整'
});
defineExpose({ canLeave: guard.canLeave });
async function beginAdjustment(value: 'credit' | 'debit') {
  if (!adjustable.value || loading.value || locked.value || !data.value || action.value === value) return;
  if (action.value && !await guard.canLeave()) return;
  resetAdjustment(); action.value = value; guard.capture();
}
function confirmAdjustment() {
  if (!adjustable.value || loading.value || locked.value || !action.value || !data.value || conflict.value || !window.$dialog) return;
  if (!validPoints.value || !reason.value.trim() || insufficient.value || !Number.isSafeInteger(data.value.revision)) return;
  const body: WalletAdjustmentRequest = {
    requestId: createRequestId(), action: action.value, points: points.value!, reason: reason.value.trim(), revision: data.value.revision
  };
  const request = { body, path: scope.serverQuery(`/wallets/${props.playerId}/adjust`), serverId: scope.serverId!, playerId: props.playerId! };
  const label = actionLabel.value;
  let accepted = false;
  confirming.value = true;
  window.$dialog.warning({
    title: `确认${label} ${body.points.toLocaleString('zh-CN')} 超频点数？`,
    content: `${scope.currentServer!.name} · ${props.playerName || `角色 #${props.playerId}`}。角色余额 ${money(data.value.balanceCents)} → ${money(afterBalance.value!)}。原因：${body.reason}`,
    positiveText: `确认${label}`, negativeText: '取消',
    onPositiveClick: () => {
      if (accepted || !canWrite.value || request.serverId !== scope.serverId || request.playerId !== props.playerId) return false;
      accepted = true; pending.value = request; confirming.value = false; void submitAdjustment();
    },
    onAfterLeave: () => { confirming.value = false; }
  });
}
async function submitAdjustment() {
  const request = pending.value;
  if (!request || submitting.value || !canWrite.value || request.serverId !== scope.serverId || request.playerId !== props.playerId) return;
  const seq = generation;
  submitting.value = true; adjustmentError.value = '';
  try {
    const result = await api<WalletAdjustmentResult>(request.path, json('POST', request.body));
    if (seq !== generation) return;
    if (data.value) { data.value.balanceCents = result.balanceCents; data.value.revision = result.revision; }
    pending.value = null; resetAdjustment(); guard.capture(); page.value = 1;
    window.$message?.success(result.alreadyApplied ? '已核对，本次调整已完成' : request.body.action === 'credit' ? '点数已发放' : '点数已扣除');
    submitting.value = false;
    await load();
  } catch (e) {
    if (seq !== generation) return;
    if (e instanceof ApiError && e.status >= 400 && e.status < 500 && ![408, 429].includes(e.status)) {
      pending.value = null; conflict.value = e.status === 409;
      adjustmentError.value = conflict.value ? '余额已更新，请刷新账单后重新确认。输入已保留。' : errorMessage(e);
    } else adjustmentError.value = '结果尚未确认，请重试本次操作。重试不会重复记账。';
  } finally { submitting.value = false; }
}
const kindOptions = [
  { label: '入账', value: 'credit' },
  { label: '支出', value: 'payment' }
];
const rangeLabel = computed(() =>
  data.value?.summary.startAt && data.value.summary.endAt
    ? `${formatTime(data.value.summary.startAt)} 至 ${formatTime(data.value.summary.endAt)}`
    : ''
);
async function load() {
  if (locked.value) return;
  generation += 1;
  const seq = generation;
  loading.value = true;
  error.value = '';
  const params = new URLSearchParams({
    kind: kind.value || '',
    page: String(page.value),
    pageSize: String(pageSize.value)
  });
  if (range.value) {
    params.set('startAt', String(range.value[0]));
    params.set('endAt', String(range.value[1]));
  }
  try {
    if (!props.playerId) throw new Error('请先选择钱包所属角色');
    const result = await api<WalletLedger>(scope.serverQuery(`/wallets/${props.playerId}/ledger?${params}`));
    if (seq === generation) {
      data.value = result;
      if (conflict.value) adjustmentError.value = '';
      conflict.value = false;
    }
  } catch (e) {
    if (seq === generation) error.value = errorMessage(e);
  } finally {
    if (seq === generation) loading.value = false;
  }
}
function filter() {
  page.value = 1;
  load();
}
function signedAmount(row: WalletLedgerItem) {
  const sign = row.kind === 'payment' ? '-' : '+';
  return `${sign}${money(Math.abs(row.amountCents))}`;
}
const columns: DataTableColumns<WalletLedgerItem> = [
  {
    title: '收支事项',
    key: 'label',
    minWidth: 230,
    render: row =>
      h('div', [
        h('strong', row.label),
        ...(row.orderId ? [h('div', { class: 'ops-muted' }, `订单 ${row.orderId}`)] : [])
      ])
  },
  {
    title: '类型',
    key: 'kind',
    width: 100,
    render: row =>
      h(NTag, { size: 'small', bordered: false, type: row.kind === 'credit' ? 'success' : 'default' }, () =>
        row.kind === 'credit' ? '入账' : '支出'
      )
  },
  {
    title: '超频点数',
    key: 'amountCents',
    width: 140,
    align: 'right',
    render: row => h('span', { class: row.kind === 'credit' ? 'ledger-income' : 'ledger-expense' }, signedAmount(row))
  },
  { title: '变更后余额', key: 'balanceCents', width: 150, align: 'right', render: row => money(row.balanceCents) },
  { title: '时间（北京）', key: 'createdAt', width: 185, render: row => formatTime(row.createdAt) }
];
watch(
  () => [props.playerId, scope.serverId],
  () => {
    resetAdjustment(); pending.value = null;
    data.value = null;
    page.value = 1;
    range.value = null;
    kind.value = null;
    load();
  },
  { immediate: true }
);
onBeforeUnmount(() => { generation += 1; });
</script>

<template>
  <div class="wallet-ledger">
    <div class="ops-toolbar mb-20px">
      <div>
        <div class="ops-muted mb-6px">角色余额</div>
        <div class="wallet-balance">
          {{ data ? money(data.balanceCents) : '—' }}
          <span class="wallet-currency">超频点数</span>
        </div>
        <div v-if="playerId" class="ops-muted mt-6px">{{ scope.currentServer?.name }} · {{ playerName || `角色 #${playerId}` }}，余额仅属于本区服角色</div>
      </div>
      <div class="wallet-actions">
        <template v-if="adjustable">
          <NButton type="primary" :disabled="loading || locked || !data" @click="beginAdjustment('credit')">发放</NButton>
          <NButton secondary type="error" :disabled="loading || locked || !data" @click="beginAdjustment('debit')">扣除</NButton>
        </template>
        <NButton :loading="loading" :disabled="locked" @click="load">刷新账单</NButton>
      </div>
    </div>
    <NAlert v-if="error" type="error" :title="error" class="mb-20px" />
    <section v-if="action" class="wallet-adjustment mb-24px">
    <NForm label-placement="top" :disabled="locked || !adjustable" @submit.prevent="confirmAdjustment">
      <h3 class="wallet-adjustment-title">{{ actionLabel }}超频点数</h3>
      <div class="wallet-adjustment-fields">
        <NFormItem label="点数" required :validation-status="insufficient ? 'error' : undefined" :feedback="insufficient ? '余额不足，请减少扣除点数' : undefined">
          <NInputNumber v-model:value="points" :min="1" :max="maxPoints" :precision="0" placeholder="输入点数" class="w-full" :input-props="{ 'aria-label': '调整点数' }" />
        </NFormItem>
        <NFormItem label="操作原因" required>
          <NInput v-model:value="reason" :maxlength="200" placeholder="最多 200 字" :input-props="{ 'aria-label': '点数操作原因' }" />
        </NFormItem>
      </div>
      <div v-if="afterBalance !== null && !insufficient" class="wallet-adjustment-preview">调整后余额 <strong>{{ money(afterBalance) }}</strong> 超频点数</div>
      <NAlert v-if="adjustmentError" type="error" class="mb-16px">{{ adjustmentError }}</NAlert>
      <div v-if="!pending" class="ops-drawer-actions">
        <NButton :disabled="locked" @click="guard.requestClose()">取消</NButton>
        <NButton attr-type="submit" :type="action === 'debit' ? 'error' : 'primary'" :loading="submitting" :disabled="loading || locked || conflict || !validPoints || !reason.trim() || insufficient || !Number.isSafeInteger(data?.revision)">确认{{ actionLabel }}</NButton>
      </div>
    </NForm>
      <div v-if="pending" class="wallet-adjustment-pending" role="status">
        <span>正在核对{{ actionLabel }} {{ pending.body.points.toLocaleString('zh-CN') }} 超频点数的结果</span>
        <NButton :loading="submitting" :disabled="submitting || !canWrite" @click="submitAdjustment">重试本次操作</NButton>
      </div>
    </section>
    <div class="ops-filters">
      <NDatePicker
        v-model:value="range"
        type="datetimerange"
        clearable
        placeholder="选择账期"
        class="ledger-date-picker"
        :disabled="locked"
      />
      <NSelect v-model:value="kind" :options="kindOptions" clearable placeholder="全部收支" class="ops-filter-select" :disabled="locked" />
      <NButton :disabled="locked" @click="filter">查询</NButton>
    </div>
    <template v-if="data">
      <div class="ledger-summary">
        <div>
          <span>收入</span>
          <strong>{{ money(data.summary.incomeCents) }}</strong>
        </div>
        <div>
          <span>支出</span>
          <strong>{{ money(Math.abs(data.summary.expenseCents)) }}</strong>
        </div>
        <div>
          <span>净收入</span>
          <strong>{{ money(data.summary.netCents) }}</strong>
        </div>
        <div>
          <span>交易笔数</span>
          <strong>{{ data.summary.transactionCount.toLocaleString() }}</strong>
        </div>
      </div>
      <div class="ops-muted mb-20px">{{ rangeLabel }}</div>
    </template>
    <NDataTable
      :columns="columns"
      :data="data?.items || []"
      :loading="loading"
      :row-key="(row: WalletLedgerItem) => row.id"
      :bordered="false"
      :scroll-x="850"
    />
    <div class="ops-pagination">
      <NPagination
        v-model:page="page"
        v-model:page-size="pageSize"
        :item-count="data?.total || 0"
        :page-sizes="[20, 50, 100]"
        show-size-picker
        :disabled="locked"
        @update:page="load"
        @update:page-size="filter"
      />
    </div>
  </div>
</template>

<style scoped>
.wallet-actions { display: flex; flex-wrap: wrap; gap: 8px; }
.wallet-adjustment { padding: 20px 0; border-block: 1px solid rgb(var(--base-text-color) / .1); }
.wallet-adjustment-title { margin: 0 0 16px; font-size: 16px; font-weight: 600; }
.wallet-adjustment-fields { display: grid; grid-template-columns: minmax(180px, 1fr) minmax(0, 2fr); gap: 16px; }
.wallet-adjustment-preview { margin-bottom: 16px; font-size: 14px; }
.wallet-adjustment-preview strong { margin: 0 4px; font-variant-numeric: tabular-nums; }
.wallet-adjustment-pending { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; }
@media (max-width: 600px) {
  .wallet-adjustment { padding: 16px 0; }
  .wallet-adjustment-fields { grid-template-columns: 1fr; gap: 0; }
  .wallet-actions { width: 100%; }
  .wallet-adjustment .ops-drawer-actions > * { flex: 1; }
}
</style>
