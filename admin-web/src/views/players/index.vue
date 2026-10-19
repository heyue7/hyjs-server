<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { NButton, NSpace } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, errorMessage, formatTime, json } from '@/service/operations';
import type { Item, ItemsResponse } from '@/service/operations';
import type { GamePlayer, GamePlayerDetail, InventoryItem } from '@/service/player-operations';
import { useOperations } from '@/composables/useOperations';
import { useServerScopeStore } from '@/store/modules/server-scope';
const scope = useServerScopeStore();
const route = useRoute();
const router = useRouter();
const walletTarget = ref<Pick<GamePlayer, 'id' | 'name'> & { accountId?: number } | null>(null);
function openWallet(player: Pick<GamePlayer, 'id' | 'name'> & { accountId?: number }) { walletTarget.value = { id: player.id, name: player.name, accountId: player.accountId }; }
function closeWallet() {
  walletTarget.value = null;
  if (route.query.wallet) { const query = { ...route.query }; delete query.wallet; router.replace({ query }); }
}
function requestedWallet() {
  const id = Number(route.query.wallet);
  if (Number.isSafeInteger(id) && id > 0) openWallet(rows.value.find(row => row.id === id) || { id, name: `用户 #${id}` });
}
import ResourceImage from '@/components/operations/resource-image.vue';
import WalletLedger from '@/components/operations/wallet-ledger.vue';
import GamePlayerCreate from '@/components/operations/game-player-create.vue';
const { canWrite, loading, error, run } = useOperations();
const createOpen = ref(false);
async function createdPlayer(player: GamePlayerDetail) { await load(); await show(player); }
const q = ref('');
const page = ref(1);
const pageSize = ref(25);
const total = ref(0);
const rows = ref<GamePlayer[]>([]);
function sceneLabel(scene: string) {
  const names: Record<string, string> = { town: '主城', town_main: '主城', field_001: '森林野外', field_002: '雪原野外', field_003: '沙漠野外', field_004: '夜森林野外', field_005: '火山野外', field_006: '峡谷野外' };
  return names[scene] || (scene ? '其他场景' : '—');
}
async function load() {
  const data = await run(() =>
    api<{ items: GamePlayer[]; total: number }>(
      scope.serverQuery(`/game-players?${new URLSearchParams({ q: q.value, page: String(page.value), pageSize: String(pageSize.value) })}`)
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
const open = ref(false);
const detail = ref<GamePlayerDetail | null>(null);
const detailLoading = ref(false);
const detailError = ref('');
const activeTab = ref('overview');
const inventoryQ = ref('');
const catalogue = ref<Record<string, Item>>({});
const inventoryPage = ref(1);
const action = ref<'name' | 'kick' | ''>('');
const actionTarget = ref<Pick<GamePlayerDetail, 'id' | 'name' | 'version'> | null>(null);
const actionReason = ref('');
const newName = ref('');
const actionError = ref('');
const acting = ref(false);
let detailRequest = 0;
const inventoryRows = computed(
  () =>
    detail.value?.inventory.filter(
      row =>
        !inventoryQ.value ||
        `${row.resourceId} ${catalogue.value[row.resourceId]?.name || ''} ${row.guid || ''}`
          .toLowerCase()
          .includes(inventoryQ.value.toLowerCase())
    ) || []
);
const inventoryColumns: DataTableColumns<InventoryItem> = [
  {
    title: '资源',
    key: 'resourceId',
    minWidth: 250,
    render: row => {
      const item = catalogue.value[row.resourceId];
      return h('div', { class: 'item-name' }, [
        h(ResourceImage, { item, size: 40 }),
        h('div', [h('strong', item?.name || row.resourceId), h('div', { class: 'ops-mono ops-muted' }, row.resourceId)])
      ]);
    }
  },
  { title: '数量', key: 'amount', width: 120, render: row => row.amount.toLocaleString() },
  { title: '等级', key: 'level', width: 90, render: row => row.level || '—' },
  { title: '实例', key: 'guid', minWidth: 210, ellipsis: { tooltip: true } }
];
async function loadCatalogue(inventory: InventoryItem[]) {
  const ids = [...new Set(inventory.map(row => row.resourceId))].filter(id => !catalogue.value[id]);
  const groups: string[][] = [];
  for (let i = 0; i < ids.length; i += 100) groups.push(ids.slice(i, i + 100));
  const results = await Promise.all(
    groups.map(group => api<ItemsResponse>(`/items?ids=${encodeURIComponent(group.join(','))}&page=1&pageSize=100`))
  );
  for (const result of results) for (const item of result.items) catalogue.value[item.id] = item;
}
async function show(player: GamePlayer) {
  action.value = '';
  open.value = true;
  detail.value = null;
  detailError.value = '';
  activeTab.value = 'overview';
  inventoryQ.value = '';
  inventoryPage.value = 1;
  await loadDetail(player.id);
}
async function loadDetail(id: number) {
  detailRequest += 1;
  const seq = detailRequest;
  detailLoading.value = true;
  detailError.value = '';
  try {
    const result = await api<GamePlayerDetail>(scope.serverQuery(`/game-players/${id}`));
    if (seq !== detailRequest) return;
    detail.value = result;
  } catch (e) {
    if (seq === detailRequest) detailError.value = errorMessage(e);
  } finally {
    if (seq === detailRequest) detailLoading.value = false;
  }
}

function edit(kind: 'name' | 'kick') {
  if (!detail.value) return;
  actionTarget.value = { id: detail.value.id, name: detail.value.name, version: detail.value.version };
  action.value = kind;
  actionReason.value = '';
  newName.value = detail.value?.name || '';
  actionError.value = '';
}
async function submitAction() {
  if (!actionTarget.value || !canWrite.value) return;
  if (!actionReason.value.trim()) {
    actionError.value = '请填写操作原因';
    return;
  }
  if (action.value === 'name' && !newName.value.trim()) {
    actionError.value = '请填写角色名称';
    return;
  }
  acting.value = true;
  actionError.value = '';
  try {
    const target = actionTarget.value;
    const path = `/game-players/${target.id}/${action.value}`;
    const result = await api<GamePlayerDetail>(
      scope.serverQuery(path),
      json(action.value === 'name' ? 'PUT' : 'POST', {
        version: target.version,
        name: newName.value.trim(),
        reason: actionReason.value.trim()
      })
    );
    if (detail.value?.id === target.id) detail.value = result;
    window.$message?.success(action.value === 'name' ? '角色名称已更新' : '角色已下线');
    action.value = '';
    await load();
  } catch (e) {
    actionError.value = errorMessage(e);
  } finally {
    acting.value = false;
  }
}
const balanceNames: Record<string, string> = {
  coin: '金币',
  diamond: '钻石',
  energy: '精力',
  stamina: '体力',
  light: '滞留之光',
  point: '攻略点数',
  popularity: '声望',
  score: '悬赏积分'
};
const countNames: Record<string, string> = {
  characters: '角色',
  equipment: '装备',
  accessories: '饰品',
  items: '道具',
  gems: '宝石',
  cooking: '料理'
};
const columns: DataTableColumns<GamePlayer> = [
  {
    title: '游戏角色',
    key: 'name',
    minWidth: 220,
    render: row => h('div', [h('strong', row.name), h('div', { class: 'ops-mono ops-muted' }, `ID ${row.id}`)])
  },
  { title: '所属用户', key: 'username', minWidth: 180, render: row => `${row.nickname || '—'} · ${row.username || '—'}` },
  { title: '等级', key: 'level', width: 100 },
  { title: 'VIP', key: 'vip', width: 100, render: row => `VIP${row.vip || 0}` },
  { title: '主线', key: 'chapter', width: 140, render: row => `${row.chapter || 0} - ${row.mission || 0}` },
  { title: '所在场景', key: 'scene', minWidth: 170, render: row => sceneLabel(row.scene) },
  {
    title: '操作',
    key: 'action',
    width: 220,
    render: row => h(NSpace, { size: 6 }, () => [
      h(NButton, { size: 'small', secondary: true, onClick: () => show(row) }, () => '查看角色'),
      h(NButton, { size: 'small', onClick: () => openWallet(row) }, () => '蓝绿钱包')
    ])
  }
];
const removeSwitchGuard = scope.addSwitchGuard(() => {
  if (acting.value) return false;
  if (!createOpen.value && !action.value) return true;
  return new Promise<boolean>(resolve => window.$dialog?.warning({
    title: '切换运营区服？', content: '当前打开的用户编辑将关闭，请先确认填写内容已保存。',
    positiveText: '切换区服', negativeText: '继续编辑', onPositiveClick: () => resolve(true),
    onNegativeClick: () => resolve(false), onClose: () => resolve(false), onMaskClick: () => resolve(false)
  }));
});
onBeforeUnmount(removeSwitchGuard);
watch(activeTab, value => {
  if (value === 'inventory' && detail.value)
    loadCatalogue(detail.value.inventory).catch(e => {
      detailError.value = errorMessage(e);
    });
});
onMounted(async () => {
 if (typeof route.query.q === 'string') q.value = route.query.q;
 await load(); requestedWallet();
 const id = Number(route.query.playerId);
 if (Number.isSafeInteger(id) && id > 0) { open.value = true; await loadDetail(id); }
});
watch(() => route.query.wallet, requestedWallet);
</script>

<template>
  <div class="ops-page">
    <div class="ops-toolbar">
      <div class="ops-page-label">
        玩家管理
        <span class="ops-count">{{ total.toLocaleString() }}</span>
      </div>
      <NSpace><NButton :loading="loading" @click="load">刷新</NButton><NButton v-if="canWrite" type="primary" :disabled="!scope.currentServer?.runtimeBound || !scope.currentServer?.enabled" @click="createOpen = true">新增玩家</NButton></NSpace>
    </div>
    <NAlert v-if="!scope.currentServer?.runtimeBound" type="info" title="该区服待部署，可先安排活动；绑定游戏服务后可新增游戏用户。" />
    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false" class="ops-panel">
      <div class="ops-filters">
        <NInput v-model:value="q" placeholder="角色名称或 ID" clearable class="ops-search" @keydown.enter="search" />
        <NButton type="primary" @click="search">搜索</NButton>
      </div>
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :row-key="(row: GamePlayer) => row.id"
        :bordered="false"
        :scroll-x="900"
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
    <NDrawer :show="Boolean(walletTarget)" :width="1020" class="ops-editor-drawer" @update:show="!$event && closeWallet()">
      <NDrawerContent :title="`${walletTarget?.name || ''} · 蓝绿钱包`" closable>
        <WalletLedger v-if="walletTarget" :key="`${scope.serverId}:${walletTarget.id}`" :account-id="walletTarget.accountId || undefined" :player-id="walletTarget.accountId ? undefined : walletTarget.id" />
      </NDrawerContent>
    </NDrawer>
    <GamePlayerCreate v-model:show="createOpen" @created="createdPlayer" />
    <NDrawer v-model:show="open" :width="980" class="ops-editor-drawer">
      <NDrawerContent title="玩家管理" closable>
        <NAlert v-if="detailError" type="error" :title="detailError" class="mb-20px" />
        <NSpin :show="detailLoading">
          <template v-if="detail">
            <div class="player-detail-header">
              <div>
                <h2>{{ detail.name }}</h2>
                <span class="ops-mono ops-muted">ID {{ detail.id }}</span>
              </div>
              <NSpace>
                <NButton :loading="detailLoading" @click="loadDetail(detail.id)">刷新</NButton>
                <NButton @click="openWallet(detail)">蓝绿钱包</NButton>
                <NButton v-if="canWrite" @click="edit('name')">修改名称</NButton>
                <NButton v-if="canWrite" @click="edit('kick')">踢下线</NButton>
              </NSpace>
            </div>
            <NTabs v-model:value="activeTab" type="line">
              <NTabPane name="overview" tab="角色概况">
                <NDescriptions :column="3" label-placement="top" class="mb-28px mt-20px">
                  <NDescriptionsItem label="角色等级">{{ detail.level }}</NDescriptionsItem>
                  <NDescriptionsItem label="会员等级">VIP{{ detail.vip }}</NDescriptionsItem>
                  <NDescriptionsItem label="主线进度">{{ detail.chapter }} - {{ detail.mission }}</NDescriptionsItem>
                  <NDescriptionsItem label="所在场景">{{ sceneLabel(detail.scene) }}</NDescriptionsItem>
                  <NDescriptionsItem label="创建时间">{{ formatTime(detail.createdAt) }}</NDescriptionsItem>
                </NDescriptions>
                <h3 class="player-section-title">当前资源</h3>
                <NDescriptions :column="3" label-placement="top" class="mb-28px">
                  <NDescriptionsItem
                    v-for="(value, key) in detail.balances"
                    :key="key"
                    :label="balanceNames[String(key)] || String(key)"
                  >
                    {{ value.toLocaleString() }}
                  </NDescriptionsItem>
                </NDescriptions>
                <h3 class="player-section-title">持有概况</h3>
                <NDescriptions :column="3" label-placement="top">
                  <NDescriptionsItem
                    v-for="(value, key) in detail.counts"
                    :key="key"
                    :label="countNames[String(key)] || String(key)"
                  >
                    {{ value.toLocaleString() }}
                  </NDescriptionsItem>
                </NDescriptions>
              </NTabPane>
              <NTabPane name="inventory" tab="背包">
                <div class="ops-filters mt-16px">
                  <NInput
                    v-model:value="inventoryQ"
                    clearable
                    placeholder="搜索物品名称或编号"
                    @update:value="inventoryPage = 1"
                  />
                </div>
                <NDataTable
                  :columns="inventoryColumns"
                  :data="inventoryRows"
                  :bordered="false"
                  :scroll-x="750"
                  :pagination="{
                    page: inventoryPage,
                    pageSize: 20,
                    onChange: (value: number) => (inventoryPage = value)
                  }"
                />
              </NTabPane>

            </NTabs>
          </template>
        </NSpin>
      </NDrawerContent>
    </NDrawer>
    <NModal
      :show="Boolean(action)"
      preset="card"
      :title="action === 'name' ? '修改角色名称' : '踢下线'"
      class="ops-modal"
      :mask-closable="!acting"
      @update:show="!$event && (action = '')"
    >
      <NAlert v-if="actionError" type="error" :title="actionError" class="mb-16px" />
      <div class="grant-object">
        <strong>{{ actionTarget?.name }}</strong>
        <span class="ops-muted">ID {{ actionTarget?.id }}</span>
      </div>
      <NForm label-placement="top" :disabled="acting">
        <NFormItem v-if="action === 'name'" label="新名称" required>
          <NInput v-model:value="newName" :maxlength="40" />
        </NFormItem>
        <NFormItem label="操作原因" required>
          <NInput v-model:value="actionReason" type="textarea" :maxlength="200" />
        </NFormItem>
      </NForm>
      <p v-if="action === 'kick'" class="ops-muted">将结束该角色当前登录会话，角色存档保留。</p>
      <template #footer>
        <div class="ops-drawer-actions">
          <NButton :disabled="acting" @click="action = ''">取消</NButton>
          <NButton type="primary" :loading="acting" @click="submitAction">
            确认{{ action === 'name' ? '修改' : '下线' }}
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>
