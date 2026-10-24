<script setup lang="ts">
import { useDraftGuard } from '@/composables/useDraftGuard';
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useWindowSize } from '@vueuse/core';
import { useRoute, useRouter } from 'vue-router';
import { NButton, NSpace } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { ApiError, api, errorMessage, formatTime, json } from '@/service/operations';
import type { GamePlayer, GamePlayerDetail } from '@/service/player-operations';
import { useOperations } from '@/composables/useOperations';
import { useServerScopeStore } from '@/store/modules/server-scope';
const scope = useServerScopeStore();
const route = useRoute();
const router = useRouter();
const walletTarget = ref<(Pick<GamePlayer, 'id' | 'name'> & { accountId?: number }) | null>(null);
const walletRef = ref<InstanceType<typeof WalletLedger> | null>(null);
async function openWallet(player: Pick<GamePlayer, 'id' | 'name'> & { accountId?: number }) {
  if (walletTarget.value && (await walletRef.value?.canLeave()) === false) return;
  walletTarget.value = {
    id: player.id,
    name: player.name,
    accountId: player.accountId
  };
}
async function closeWallet() {
  if ((await walletRef.value?.canLeave()) === false) return;
  walletTarget.value = null;
  if (route.query.wallet) {
    const query = { ...route.query };
    delete query.wallet;
    router.replace({ query });
  }
}
function requestedWallet() {
  const id = Number(route.query.wallet);
  if (Number.isSafeInteger(id) && id > 0)
    openWallet(rows.value.find(row => row.id === id) || { id, name: `角色 #${id}` });
}
import PlayerProfileEditor from '@/components/operations/player-profile-editor.vue';
import PlayerInventory from '@/components/operations/player-inventory.vue';
const profileOpen = ref(false);
const inventoryRef = ref<InstanceType<typeof PlayerInventory> | null>(null);
async function closeDetail(value: boolean) {
  if (value || (await inventoryRef.value?.canLeave()) !== false) open.value = value;
}
async function profileSaved(value: GamePlayerDetail) {
  detail.value = value;
  await load();
}
async function inventoryChanged() {
  if (detail.value) await loadDetail(detail.value.id);
  await load();
}
import WalletLedger from '@/components/operations/wallet-ledger.vue';
import GamePlayerCreate from '@/components/operations/game-player-create.vue';
const { canWrite, loading, error, run } = useOperations();
const createOpen = ref(false);
async function createdPlayer(player: GamePlayerDetail) {
  await load();
  await show(player);
}
const q = ref('');
const page = ref(1);
const pageSize = ref(25);
const total = ref(0);
const rows = ref<GamePlayer[]>([]);
function sceneLabel(scene: string) {
  const names: Record<string, string> = {
    town: '主城',
    town_main: '主城',
    field_001: '森林野外',
    field_002: '雪原野外',
    field_003: '沙漠野外',
    field_004: '夜森林野外',
    field_005: '火山野外',
    field_006: '峡谷野外'
  };
  return names[scene] || (scene ? '其他场景' : '—');
}
async function load() {
  const data = await run(() =>
    api<{ items: GamePlayer[]; total: number }>(
      scope.serverQuery(
        `/game-players?${new URLSearchParams({ q: q.value, page: String(page.value), pageSize: String(pageSize.value) })}`
      )
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
const action = ref<'name' | 'level' | 'kick' | ''>('');
const actionTarget = ref<Pick<GamePlayerDetail, 'id' | 'name' | 'level' | 'version'> | null>(null);
const actionReason = ref('');
const newName = ref('');
const newLevel = ref<number | null>(null);
const actionError = ref('');
const actionConflict = ref(false);
const acting = ref(false);
const actionGuard = useDraftGuard({
  active: () => Boolean(action.value),
  value: () => ({
    name: newName.value,
    level: newLevel.value,
    reason: actionReason.value
  }),
  busy: () => acting.value,
  writable: () => canWrite.value,
  label: '玩家修改',
  close: () => {
    action.value = '';
  }
});
let detailRequest = 0;
async function show(player: GamePlayer) {
  action.value = '';
  open.value = true;
  detail.value = null;
  detailError.value = '';
  activeTab.value = 'overview';
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

async function editPlayer(player: GamePlayer) {
  await show(player);
  if (open.value && detail.value?.id === player.id) profileOpen.value = true;
}
function edit(kind: 'name' | 'level' | 'kick') {
  if (!detail.value) return;
  actionTarget.value = {
    id: detail.value.id,
    name: detail.value.name,
    level: detail.value.level,
    version: detail.value.version
  };
  action.value = kind;
  actionReason.value = '';
  newName.value = detail.value?.name || '';
  newLevel.value = detail.value.level;
  actionError.value = '';
  actionConflict.value = false;
  actionGuard.capture();
}
async function refreshAction() {
  const target = actionTarget.value;
  if (!target || acting.value) return;
  await loadDetail(target.id);
  if (detail.value?.id === target.id && !detailError.value) {
    actionTarget.value = {
      id: detail.value.id,
      name: detail.value.name,
      level: detail.value.level,
      version: detail.value.version
    };
    actionError.value = '';
    actionConflict.value = false;
    window.$message?.info('已读取最新数据，请核对后保存');
  } else actionError.value = detailError.value;
}
async function submitAction() {
  if (!actionTarget.value || !canWrite.value || acting.value) return;
  if (!actionReason.value.trim()) {
    actionError.value = '请填写操作原因';
    return;
  }
  if (action.value === 'name' && !newName.value.trim()) {
    actionError.value = '请填写角色名称';
    return;
  }
  if (action.value === 'level' && (!Number.isInteger(newLevel.value) || newLevel.value! < 1 || newLevel.value! > 100)) {
    actionError.value = '请输入 1～100 的整数等级';
    return;
  }
  acting.value = true;
  actionError.value = '';
  actionConflict.value = false;
  try {
    const target = actionTarget.value;
    const path = `/game-players/${target.id}/${action.value}`;
    const result = await api<GamePlayerDetail>(
      scope.serverQuery(path),
      json(action.value === 'kick' ? 'POST' : 'PUT', {
        version: target.version,
        name: newName.value.trim(),
        level: newLevel.value,
        reason: actionReason.value.trim()
      })
    );
    if (detail.value?.id === target.id) detail.value = result;
    window.$message?.success(
      action.value === 'name'
        ? '角色名称已更新'
        : action.value === 'level'
          ? '角色等级已更新，重新登录游戏后可见'
          : '角色已下线'
    );
    action.value = '';
    await load();
  } catch (e) {
    actionConflict.value = e instanceof ApiError && e.status === 409;
    actionError.value = actionConflict.value ? '角色数据已更新，请读取最新数据后再次保存' : errorMessage(e);
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
  score: '悬赏积分',
  coinpet: '精灵币',
  contribution: '公会贡献'
};
const countNames: Record<string, string> = {
  characters: '角色',
  equipment: '装备',
  accessories: '饰品',
  items: '道具',
  gems: '宝石',
  cooking: '料理'
};
const { width: viewportWidth } = useWindowSize();
const columns = computed<DataTableColumns<GamePlayer>>(() => [
  {
    title: '游戏角色',
    key: 'name',
    minWidth: 220,
    render: row => h('div', [h('strong', row.name), h('div', { class: 'ops-mono ops-muted' }, `ID ${row.id}`)])
  },
  {
    title: 'Brain Burst 2039 账号',
    key: 'username',
    minWidth: 180,
    render: row =>
      row.accountId
        ? h(
            NButton,
            {
              text: true,
              type: 'primary',
              onClick: () =>
                router.push({
                  path: '/game-accounts',
                  query: { accountId: row.accountId, q: row.username || '' }
                })
            },
            () => `${row.nickname || '—'} · ${row.username || '账号 #' + row.accountId}`
          )
        : `${row.nickname || '—'} · ${row.username || '—'}`
  },
  { title: '等级', key: 'level', width: 100 },
  {
    title: 'VIP',
    key: 'vip',
    width: 100,
    render: row => `VIP${row.vip || 0}`
  },
  {
    title: '主线',
    key: 'chapter',
    width: 140,
    render: row => `${row.chapter || 0} - ${row.mission || 0}`
  },
  {
    title: '所在场景',
    key: 'scene',
    minWidth: 170,
    render: row => sceneLabel(row.scene)
  },
  {
    title: '操作',
    key: 'action',
    width: viewportWidth.value < 768 ? 150 : 290,
    fixed: 'right',
    render: row =>
      h(NSpace, { size: 6 }, () => [
        h(NButton, { size: 'small', secondary: true, onClick: () => show(row) }, () => '查看角色'),
        ...(canWrite.value
          ? [
              h(
                NButton,
                {
                  size: 'small',
                  type: 'primary',
                  secondary: true,
                  disabled: detailLoading.value,
                  onClick: () => editPlayer(row)
                },
                () => '编辑'
              )
            ]
          : []),
        h(NButton, { size: 'small', onClick: () => openWallet(row) }, () => '蓝绿终端')
      ])
  }
]);
onMounted(async () => {
  if (typeof route.query.q === 'string') q.value = route.query.q;
  await load();
  requestedWallet();
  const id = Number(route.query.playerId);
  if (!route.query.wallet && Number.isSafeInteger(id) && id > 0) {
    open.value = true;
    await loadDetail(id);
  }
});
onBeforeUnmount(() => {
  detailRequest += 1;
});
watch(() => route.query.wallet, requestedWallet);
watch(
  () => scope.serverId,
  () => {
    detailRequest += 1;
    open.value = false;
    detail.value = null;
    walletTarget.value = null;
    action.value = '';
    actionTarget.value = null;
    profileOpen.value = false;
    page.value = 1;
    load();
  }
);
</script>

<template>
  <div class="ops-page">
    <NAlert v-if="!scope.currentServer?.runtimeBound" type="info" title="该区服尚未部署，绑定游戏服务后可新增角色。" />
    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false" class="ops-panel">
      <div class="ops-filters">
        <NInput
          v-model:value="q"
          placeholder="角色名称或 ID"
          :input-props="{ 'aria-label': '角色搜索' }"
          clearable
          class="ops-search"
          @keydown.enter="search"
        />
        <NButton type="primary" @click="search">搜索</NButton>
        <span class="ops-muted">共 {{ total.toLocaleString() }} 个角色</span>
        <NSpace>
          <NButton :loading="loading" @click="load">刷新</NButton>
          <NButton
            v-if="canWrite"
            type="primary"
            :disabled="!scope.currentServer?.runtimeBound || !scope.currentServer?.enabled"
            @click="createOpen = true"
          >
            新增玩家
          </NButton>
        </NSpace>
      </div>
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :row-key="(row: GamePlayer) => row.id"
        :bordered="false"
        :scroll-x="1020"
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
    <NDrawer
      :show="Boolean(walletTarget)"
      :width="Math.min(1020, viewportWidth)"
      class="ops-editor-drawer"
      @update:show="!$event && closeWallet()"
    >
      <NDrawerContent :title="`${walletTarget?.name || ''} · 蓝绿终端`" closable>
        <WalletLedger
          v-if="walletTarget"
          ref="walletRef"
          :key="`${scope.serverId}:${walletTarget.id}`"
          :player-id="walletTarget.id"
          :player-name="walletTarget.name"
        />
      </NDrawerContent>
    </NDrawer>
    <GamePlayerCreate v-model:show="createOpen" @created="createdPlayer" />
    <PlayerProfileEditor v-model:show="profileOpen" :player="detail" @saved="profileSaved" />
    <NDrawer :show="open" @update:show="closeDetail" width="min(980px, 100vw)" class="ops-editor-drawer">
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
                <NButton @click="openWallet(detail)">蓝绿终端</NButton>

                <NButton v-if="canWrite" type="primary" secondary @click="profileOpen = true">编辑</NButton>
                <NButton v-if="canWrite" @click="edit('kick')">踢下线</NButton>
              </NSpace>
            </div>
            <NTabs v-model:value="activeTab" type="line">
              <NTabPane name="overview" tab="角色概况">
                <NDescriptions :column="viewportWidth < 600 ? 2 : 3" label-placement="top" class="mb-28px mt-20px">
                  <NDescriptionsItem label="角色等级">{{ detail.level }}</NDescriptionsItem>
                  <NDescriptionsItem label="本级经验">{{ detail.exp }}</NDescriptionsItem>
                  <NDescriptionsItem label="所属区服">{{ scope.currentServer?.name }}</NDescriptionsItem>
                  <NDescriptionsItem label="Brain Burst 2039 账号" :span="viewportWidth < 600 ? 2 : 1"><NButton v-if="detail.accountId" text type="primary" @click="router.push({ path: '/game-accounts', query: { accountId: detail.accountId, q: detail.username || '' } })">{{ detail.nickname || detail.username }} · {{ detail.accountId }}</NButton><span v-else>—</span></NDescriptionsItem>
                  <NDescriptionsItem label="蓝绿终端">本区服角色独立余额</NDescriptionsItem>
                  <NDescriptionsItem label="会员等级">VIP{{ detail.vip }}</NDescriptionsItem>
                  <NDescriptionsItem label="主线进度">{{ detail.chapter }} - {{ detail.mission }}</NDescriptionsItem>
                  <NDescriptionsItem label="所在场景">{{ sceneLabel(detail.scene) }}</NDescriptionsItem>
                  <NDescriptionsItem label="创建时间">{{ formatTime(detail.createdAt) }}</NDescriptionsItem>
                </NDescriptions>
                <h3 class="player-section-title">当前资源</h3>
                <NDescriptions :column="viewportWidth < 600 ? 2 : 3" label-placement="top" class="mb-28px">
                  <NDescriptionsItem
                    v-for="(value, key) in detail.balances"
                    :key="key"
                    :label="balanceNames[String(key)] || String(key)"
                  >
                    {{ value.toLocaleString() }}
                  </NDescriptionsItem>
                </NDescriptions>
                <h3 class="player-section-title">持有概况</h3>
                <NDescriptions :column="viewportWidth < 600 ? 2 : 3" label-placement="top">
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
                <PlayerInventory
                  ref="inventoryRef"
                  :key="`${scope.serverId}:${detail.id}`"
                  :player-id="detail.id"
                  :player-name="detail.name"
                  @changed="inventoryChanged"
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
      :title="action === 'name' ? '修改角色名称' : action === 'level' ? '编辑玩家' : '踢下线'"
      class="ops-modal"
      :mask-closable="!acting"
      :closable="!acting"
      @update:show="!$event && actionGuard.requestClose()"
    >
      <NAlert v-if="actionError" type="error" :title="actionError" class="mb-16px" />
      <NButton v-if="actionConflict" :disabled="acting" :loading="detailLoading" class="mb-16px" @click="refreshAction"
        >读取最新数据</NButton
      >
      <div class="grant-object">
        <strong>{{ actionTarget?.name }}</strong>
        <span class="ops-muted">{{ scope.currentServer?.name }} · ID {{ actionTarget?.id }}</span>
      </div>
      <NForm label-placement="top" :disabled="acting">
        <NFormItem v-if="action === 'name'" label="新名称" required>
          <NInput v-model:value="newName" :maxlength="40" />
        </NFormItem>
        <NFormItem v-if="action === 'level'" label="角色等级" required>
          <NInputNumber
            v-model:value="newLevel"
            :min="1"
            :max="100"
            :precision="0"
            :input-props="{ 'aria-label': '角色等级' }"
            class="w-full"
          />
        </NFormItem>
        <p v-if="action === 'level'" class="ops-muted mb-16px">
          当前
          {{ actionTarget?.level }}
          级。等级变化后本级经验归零，重新登录游戏后显示。
        </p>
        <NFormItem label="操作原因" required>
          <NInput
            v-model:value="actionReason"
            type="textarea"
            :maxlength="200"
            :input-props="{ 'aria-label': '操作原因' }"
          />
        </NFormItem>
      </NForm>
      <p v-if="action === 'kick'" class="ops-muted">将结束该角色当前登录会话，角色存档保留。</p>
      <template #footer>
        <div class="ops-drawer-actions">
          <NButton :disabled="acting" @click="actionGuard.requestClose()">取消</NButton>
          <NButton type="primary" :loading="acting" :disabled="detailLoading" @click="submitAction">
            {{ action === 'kick' ? '确认下线' : '保存修改' }}
          </NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>
