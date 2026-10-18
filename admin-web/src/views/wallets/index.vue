<script setup lang="ts">
import { h, onMounted, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import { NButton } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api } from '@/service/operations';
import { money } from '@/service/wallet-operations';
import type { WalletAccount } from '@/service/wallet-operations';
import { useOperations } from '@/composables/useOperations';
import WalletLedger from '@/components/operations/wallet-ledger.vue';
import WalletReasons from '@/components/operations/wallet-reasons.vue';
const route = useRoute();
const { loading, error, run } = useOperations();
const rows = ref<WalletAccount[]>([]);
const q = ref('');
const page = ref(1);
const pageSize = ref(25);
const total = ref(0);
const ledgerOpen = ref(false);
const reasonsOpen = ref(false);
const selected = ref<WalletAccount | null>(null);
async function load() {
  const data = await run(() =>
    api<{ items: WalletAccount[]; total: number }>(
      `/wallets?${new URLSearchParams({ q: q.value, page: String(page.value), pageSize: String(pageSize.value) })}`
    )
  );
  if (data) {
    rows.value = data.items;
    total.value = data.total;
  }
}
function search() {
  page.value = 1;
  load();
}
function show(row: WalletAccount) {
  selected.value = row;
  ledgerOpen.value = true;
}
function openRequestedPlayer() {
  const id = Number(route.query.playerId);
  if (!Number.isSafeInteger(id) || id < 1) return;
  selected.value = rows.value.find(row => row.playerId === id) || {
    playerId: id,
    name: '',
    balanceCents: 0,
    revision: 0
  };
  ledgerOpen.value = true;
}
const columns: DataTableColumns<WalletAccount> = [
  {
    title: '游戏角色',
    key: 'name',
    minWidth: 240,
    render: row => h('div', [h('strong', row.name), h('div', { class: 'ops-mono ops-muted' }, `ID ${row.playerId}`)])
  },
  { title: '钱包余额（元）', key: 'balanceCents', width: 190, align: 'right', render: row => money(row.balanceCents) },
  { title: '账本版本', key: 'revision', width: 140, align: 'right' },
  {
    title: '操作',
    key: 'action',
    width: 140,
    render: row => h(NButton, { size: 'small', secondary: true, onClick: () => show(row) }, () => '查看账单')
  }
];
onMounted(async () => {
  await load();
  openRequestedPlayer();
});
watch(() => route.query.playerId, openRequestedPlayer);
</script>

<template>
  <div class="ops-page">
    <div class="ops-toolbar">
      <div class="ops-page-label">蓝绿钱包</div>
      <NSpace>
        <NButton @click="reasonsOpen = true">入账理由</NButton>
        <NButton :loading="loading" @click="load">刷新</NButton>
      </NSpace>
    </div>
    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false" class="ops-panel">
      <div class="ops-filters">
        <NInput v-model:value="q" clearable placeholder="角色名称或 ID" class="ops-search" @keydown.enter="search" />
        <NButton type="primary" @click="search">搜索</NButton>
      </div>
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :row-key="(row: WalletAccount) => row.playerId"
        :bordered="false"
        :scroll-x="750"
      />
      <div class="ops-pagination">
        <NPagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :item-count="total"
          :page-sizes="[25, 50, 100]"
          show-size-picker
          @update:page="load"
          @update:page-size="search"
        />
      </div>
    </NCard>
    <NDrawer v-model:show="ledgerOpen" :width="1080" class="ops-editor-drawer">
      <NDrawerContent
        :title="selected?.name ? `${selected.name} · 钱包账单` : `钱包账单 · ID ${selected?.playerId}`"
        closable
      >
        <WalletLedger v-if="selected" :key="selected.playerId" :player-id="selected.playerId" />
      </NDrawerContent>
    </NDrawer>
    <NDrawer v-model:show="reasonsOpen" :width="980" class="ops-editor-drawer">
      <NDrawerContent title="入账理由" closable><WalletReasons v-if="reasonsOpen" /></NDrawerContent>
    </NDrawer>
  </div>
</template>
