<script setup lang="ts">
import { computed, h, onBeforeUnmount, ref, watch } from 'vue';
import { NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, errorMessage, formatTime } from '@/service/operations';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { money } from '@/service/wallet-operations';
import type { WalletLedger, WalletLedgerItem } from '@/service/wallet-operations';
const scope = useServerScopeStore();
const props = defineProps<{ playerId?: number; accountId?: number }>();
const loading = ref(false);
const error = ref('');
const data = ref<WalletLedger | null>(null);
const range = ref<[number, number] | null>(null);
const kind = ref<string | null>(null);
const page = ref(1);
const pageSize = ref(20);
let generation = 0;
const kindOptions = [
  { label: '入账', value: 'credit' },
  { label: '充值支出', value: 'payment' }
];
const rangeLabel = computed(() =>
  data.value?.summary.startAt && data.value.summary.endAt
    ? `${formatTime(data.value.summary.startAt)} 至 ${formatTime(data.value.summary.endAt)}`
    : ''
);
async function load() {
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
    let result: WalletLedger;
    if (props.accountId) {
      const ledger = await api<Omit<WalletLedger, 'items'> & { entries: WalletLedgerItem[] | null }>(
        `/game-accounts/${props.accountId}/wallet?${params}`
      );
      if (!('entries' in ledger)) throw new Error('钱包账单数据不完整，请刷新重试');
      result = { ...ledger, items: ledger.entries || [] };
    } else {
      if (!props.playerId) throw new Error('请先选择钱包所属用户');
      result = await api<WalletLedger>(scope.serverQuery(`/wallets/${props.playerId}/ledger?${params}`));
    }
    if (seq === generation) data.value = result;
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
        row.kind === 'credit' ? '入账' : '充值支出'
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
  () => [props.accountId, props.playerId, props.accountId ? null : scope.serverId],
  () => {
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
    <div class="ops-toolbar mb-24px">
      <div>
        <div class="ops-muted mb-6px">蓝绿终端余额</div>
        <div class="wallet-balance">
          {{ data ? money(data.balanceCents) : '—' }}
          <span class="wallet-currency">超频点数</span>
        </div>
      </div>
      <NButton :loading="loading" @click="load">刷新账单</NButton>
    </div>
    <NAlert v-if="error" type="error" :title="error" class="mb-20px" />
    <div class="ops-filters">
      <NDatePicker
        v-model:value="range"
        type="datetimerange"
        clearable
        placeholder="选择账期"
        class="ledger-date-picker"
      />
      <NSelect v-model:value="kind" :options="kindOptions" clearable placeholder="全部收支" class="ops-filter-select" />
      <NButton type="primary" @click="filter">查询</NButton>
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
        @update:page="load"
        @update:page-size="filter"
      />
    </div>
  </div>
</template>
