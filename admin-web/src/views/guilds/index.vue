<script setup lang="ts">
import { useWindowSize } from '@vueuse/core';
import { useDraftGuard } from '@/composables/useDraftGuard';
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NButton, NInput, NInputNumber, NSelect, NSpace, NSwitch } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, errorMessage, formatTime } from '@/service/operations';
import {
  addGuildMember,
  dissolveGuild,
  getGuild,
  getGuilds,
  removeGuildMember,
  updateGuild,
  updateGuildMember
} from '@/service/guilds';
import type { GuildDetail, GuildMember, GuildSummary, GuildUpdate } from '@/service/guilds';
import { useOperations } from '@/composables/useOperations';
import { useServerScopeStore } from '@/store/modules/server-scope';

const { width: viewportWidth } = useWindowSize();
const formLabelPlacement = computed(() => viewportWidth.value < 600 ? 'top' : 'left');
const scope = useServerScopeStore();
const { canWrite, loading, error, run } = useOperations();
const q = ref('');
const page = ref(1);
const pageSize = ref(25);
const total = ref(0);
const rows = ref<GuildSummary[]>([]);
const drawer = ref(false);
const detail = ref<GuildDetail | null>(null);
const detailLoading = ref(false);
const detailError = ref('');
const form = ref<GuildUpdate | null>(null);
const baseline = ref('');
const saving = ref(false);
const saveError = ref('');
const reason = ref('');
const memberOpen = ref(false);
const memberTarget = ref<GuildMember | null>(null);
const memberDraft = ref({ contribution: 0, spendableContribution: 0, todayCoin: 0, todayBadge: 0, role: 0, reason: '' });
const memberSaving = ref(false);
const memberError = ref('');
const addOpen = ref(false);
const addDraft = ref<{ playerId: number | null; reason: string }>({ playerId: null, reason: '' });
const addSaving = ref(false);
const addError = ref('');
const removeTarget = ref<GuildMember | null>(null);
const removeReason = ref('');
const removing = ref(false);
const removeError = ref('');
const dissolveOpen = ref(false);
const dissolveName = ref('');
const dissolveReason = ref('');
const dissolving = ref(false);
const dissolveError = ref('');

const busy = computed(() => saving.value || memberSaving.value || addSaving.value || removing.value || dissolving.value);
const guildGuard = useDraftGuard({ active: () => drawer.value, value: () => ({ form: form.value, reason: reason.value }), busy: () => busy.value, writable: () => canWrite.value, label: '公会修改', close: () => { drawer.value = false; }, route: false, scope: false });
const memberGuard = useDraftGuard({ active: () => memberOpen.value, value: () => memberDraft.value, busy: () => busy.value, label: '成员修改', close: () => { memberOpen.value = false; }, route: false, scope: false });
const addGuard = useDraftGuard({ active: () => addOpen.value, value: () => addDraft.value, busy: () => busy.value, label: '加入成员草稿', close: () => { addOpen.value = false; }, route: false, scope: false });
const removeGuard = useDraftGuard({ active: () => Boolean(removeTarget.value), value: () => removeReason.value, busy: () => busy.value, label: '移除原因', close: () => { removeTarget.value = null; }, route: false, scope: false });
const dissolveGuard = useDraftGuard({ active: () => dissolveOpen.value, value: () => ({ name: dissolveName.value, reason: dissolveReason.value }), busy: () => busy.value, label: '解散输入', close: () => { dissolveOpen.value = false; }, route: false, scope: false });
const playerOptions = ref<{ value: number; label: string }[]>([]);
const playerLoading = ref(false);
let playerGeneration = 0;
let detailGeneration = 0;
async function searchPlayers(query = '') {
  const current = ++playerGeneration;
  const serverId = currentServerID.value;
  if (!serverId || !addOpen.value) return;
  playerLoading.value = true;
  addError.value = '';
  try {
    const result = await api<{ items: { id: number; name: string; level: number }[] }>(`/game-players?${new URLSearchParams({ serverId: String(serverId), q: query.trim(), page: '1', pageSize: '50' })}`);
    if (current !== playerGeneration || serverId !== currentServerID.value || !addOpen.value) return;
    const selected = playerOptions.value.find(row => row.value === addDraft.value.playerId);
    playerOptions.value = result.items.map(row => ({ value: row.id, label: `${row.name} · ID ${row.id} · Lv.${row.level}` }));
    if (selected && !playerOptions.value.some(row => row.value === selected.value)) playerOptions.value.unshift(selected);
  } catch (e) { if (current === playerGeneration && addOpen.value && serverId === currentServerID.value) addError.value = errorMessage(e); }
  finally { if (current === playerGeneration) playerLoading.value = false; }
}
const roleOptions = [
  { label: '普通成员', value: 0 },
  { label: '副会长', value: 1 },
  { label: '会长（原子转让）', value: 2 }
];
const guildDirty = computed(() => Boolean(detail.value && form.value && JSON.stringify(form.value) !== baseline.value && canWrite.value));
const drawerDirty = computed(() => guildGuard.dirty.value || memberGuard.dirty.value || addGuard.dirty.value || removeGuard.dirty.value || dissolveGuard.dirty.value);
const currentServerID = computed(() => scope.currentServer?.id || 0);

function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T;
}

function setDetail(value: GuildDetail) {
  detail.value = value;
  form.value = {
    version: value.version,
    reason: '',
    name: value.name,
    level: value.level,
    notice: value.notice,
    declaration: value.declaration,
    flag: value.flag,
    coin: value.coin,
    badge: value.badge,
    exp: value.exp,
    donateScore: value.donateScore,
    activeScore: value.activeScore,
    heroStatueOpen: value.heroStatueOpen,
    guardTreeOpen: value.guardTreeOpen
  };
  baseline.value = JSON.stringify(form.value);
  reason.value = '';
  guildGuard.capture();
}

async function load() {
  if (!currentServerID.value) {
    rows.value = [];
    total.value = 0;
    return;
  }
  const data = await run(() => getGuilds(currentServerID.value, q.value.trim(), page.value, pageSize.value));
  if (data) {
    rows.value = data.items;
    total.value = data.total;
  }
}
function search() {
  page.value = 1;
  load();
}

async function show(row: GuildSummary) {
  if (busy.value) return;
  if (!currentServerID.value) return;
  drawer.value = true;
  detail.value = null;
  form.value = null;
  detailError.value = '';
  await loadDetail(row.id);
}
async function loadDetail(id: string) {
  const generation = ++detailGeneration;
  const serverId = currentServerID.value;
  detailLoading.value = true;
  detailError.value = '';
  try {
    const result = await getGuild(currentServerID.value, id);
    if (generation === detailGeneration && serverId === currentServerID.value && drawer.value) setDetail(result);
  } catch (e) {
    if (generation === detailGeneration && drawer.value) detailError.value = errorMessage(e);
  } finally {
    if (generation === detailGeneration) detailLoading.value = false;
  }
}
async function refreshDetail() {
  if (!detail.value || !await guildGuard.canLeave()) return;
  await loadDetail(detail.value.id);
}

function checkClean() {
  if (busy.value) return false;
  if (!guildGuard.dirty.value) return true;
  return window.confirm('公会数值有未保存修改，继续操作会丢弃这些输入。确定继续？');
}

async function saveGuild() {
  if (busy.value) return;
  if (!detail.value || !form.value || !canWrite.value || saving.value) return;
  if (!reason.value.trim()) {
    saveError.value = '请填写本次变更原因';
    return;
  }
  saving.value = true;
  saveError.value = '';
  try {
    const result = await updateGuild(currentServerID.value, detail.value.id, { ...form.value, reason: reason.value.trim() });
    setDetail(result);
    window.$message?.success('公会数值已保存');
    await load();
  } catch (e) {
    saveError.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}

function editMember(row: GuildMember) {
  if (!checkClean()) return;
  memberTarget.value = row;
  memberDraft.value = { contribution: row.contribution, spendableContribution: row.spendableContribution, todayCoin: row.todayCoin, todayBadge: row.todayBadge, role: row.role, reason: '' };
  memberError.value = '';
  memberOpen.value = true;
  memberGuard.capture();
}
async function submitMember() {
  if (busy.value) return;
  if (!detail.value || !memberTarget.value || memberSaving.value || !canWrite.value) return;
  if (!memberDraft.value.reason.trim()) {
    memberError.value = '请填写操作原因';
    return;
  }
  memberSaving.value = true;
  memberError.value = '';
  try {
    const result = await updateGuildMember(currentServerID.value, detail.value.id, memberTarget.value.playerId, {
      version: memberTarget.value.version,
      reason: memberDraft.value.reason.trim(),
      contribution: memberDraft.value.contribution,
      spendableContribution: memberDraft.value.spendableContribution,
      expectedSpendableContribution: memberTarget.value.spendableContribution,
      todayCoin: memberDraft.value.todayCoin,
      todayBadge: memberDraft.value.todayBadge,
      role: memberDraft.value.role
    });
    setDetail(result);
    memberOpen.value = false;
    window.$message?.success('成员数值与职位已保存');
    await load();
  } catch (e) {
    memberError.value = errorMessage(e);
  } finally {
    memberSaving.value = false;
  }
}

function beginAdd() {
  if (!checkClean()) return;
  addDraft.value = { playerId: null, reason: '' };
  addError.value = '';
  playerOptions.value = [];
  addOpen.value = true;
  addGuard.capture();
  searchPlayers();
}
async function submitAdd() {
  if (busy.value) return;
  if (!detail.value || !addDraft.value.playerId || addSaving.value || !canWrite.value) return;
  if (!addDraft.value.reason.trim()) {
    addError.value = '请填写操作原因';
    return;
  }
  addSaving.value = true;
  addError.value = '';
  try {
    const result = await addGuildMember(currentServerID.value, detail.value.id, {
      version: detail.value.version,
      reason: addDraft.value.reason.trim(),
      playerId: addDraft.value.playerId
    });
    setDetail(result);
    addOpen.value = false;
    window.$message?.success('成员已加入公会');
    await load();
  } catch (e) {
    addError.value = errorMessage(e);
  } finally {
    addSaving.value = false;
  }
}

function beginRemove(row: GuildMember) {
  if (!checkClean()) return;
  removeTarget.value = row;
  removeReason.value = '';
  removeError.value = '';
  removeGuard.capture();
}
async function submitRemove() {
  if (busy.value) return;
  if (!detail.value || !removeTarget.value || removing.value || !canWrite.value) return;
  if (!removeReason.value.trim()) {
    removeError.value = '请填写移除原因';
    return;
  }
  removing.value = true;
  removeError.value = '';
  try {
    const result = await removeGuildMember(currentServerID.value, detail.value.id, removeTarget.value.playerId, {
      version: detail.value.version,
      reason: removeReason.value.trim()
    });
    setDetail(result);
    removeTarget.value = null;
    window.$message?.success('成员已移出公会');
    await load();
  } catch (e) {
    removeError.value = errorMessage(e);
  } finally {
    removing.value = false;
  }
}

function beginDissolve() {
  if (!detail.value || !canWrite.value || !checkClean()) return;
  window.$dialog?.warning({
    title: `解散「${detail.value.name}」？`,
    content: '全体成员会离开公会并立即失去公会频道与公会战资格，24小时退会冷却生效；历史回执、祈祷、战争结算和已发奖励保留。下一步还需要输入精确公会名。',
    positiveText: '继续确认',
    negativeText: '取消',
    onPositiveClick: () => {
      dissolveName.value = '';
      dissolveReason.value = '';
      dissolveError.value = '';
      dissolveOpen.value = true;
      dissolveGuard.capture();
    }
  });
}
async function submitDissolve() {
  if (busy.value) return;
  if (!detail.value || dissolving.value || !canWrite.value) return;
  if (dissolveName.value !== detail.value.name) {
    dissolveError.value = '公会名必须与当前名称完全一致';
    return;
  }
  if (!dissolveReason.value.trim()) {
    dissolveError.value = '请填写解散原因';
    return;
  }
  dissolving.value = true;
  dissolveError.value = '';
  try {
    await dissolveGuild(currentServerID.value, detail.value.id, {
      version: detail.value.version,
      confirmName: dissolveName.value,
      reason: dissolveReason.value.trim()
    });
    dissolveOpen.value = false;
    drawer.value = false;
    detail.value = null;
    form.value = null;
    window.$message?.success('公会已解散');
    await load();
  } catch (e) {
    dissolveError.value = errorMessage(e);
  } finally {
    dissolving.value = false;
  }
}

async function canLeave() {
  if (busy.value) return false;
  for (const guard of [memberGuard, addGuard, removeGuard, dissolveGuard, guildGuard]) {
    if (!await guard.canLeave()) return false;
  }
  return true;
}
async function requestCloseDrawer() {
  if (!await canLeave()) return;
  detailGeneration += 1;
  playerGeneration += 1;
  memberOpen.value = false;
  addOpen.value = false;
  removeTarget.value = null;
  dissolveOpen.value = false;
  drawer.value = false;
}

const columns: DataTableColumns<GuildSummary> = [
  {
    title: '公会',
    key: 'name',
    minWidth: 220,
    render: row => h('div', [h('strong', row.name), h('div', { class: 'ops-mono ops-muted' }, `ID ${row.id}`)])
  },
  { title: '等级', key: 'level', width: 80 },
  { title: '成员', key: 'memberCount', width: 80, render: row => `${row.memberCount} 人` },
  { title: '会长', key: 'chairmanName', minWidth: 140, render: row => row.chairmanName || '—' },
  { title: '金币', key: 'coin', width: 120, render: row => row.coin.toLocaleString() },
  { title: '徽章', key: 'badge', width: 90 },
  { title: '活跃度', key: 'activeScore', width: 100 },
  { title: '创建时间', key: 'createdAt', width: 180, render: row => formatTime(row.createdAt) },
  {
    title: '操作',
    key: 'action',
    width: 110,
    render: row => h(NButton, { size: 'small', secondary: true, onClick: () => show(row) }, () => '管理')
  }
];
const memberColumns: DataTableColumns<GuildMember> = [
  {
    title: '角色',
    key: 'playerName',
    minWidth: 170,
    render: row => h('div', [h('strong', row.playerName || `#${row.playerId}`), h('div', { class: 'ops-mono ops-muted' }, `ID ${row.playerId} · Lv.${row.level}`)])
  },
  {
    title: '职位',
    key: 'role',
    width: 100,
    render: row => h('span', { class: row.role === 2 ? 'text-orange-600 font-600' : row.role === 1 ? 'text-blue-600' : '' }, row.role === 2 ? '会长' : row.role === 1 ? '副会长' : '成员')
  },
  { title: '累计贡献', key: 'contribution', width: 100 },
  { title: '可用贡献', key: 'spendableContribution', width: 100 },
  { title: '今日金币', key: 'todayCoin', width: 110, render: row => row.todayCoin.toLocaleString() },
  { title: '今日徽章', key: 'todayBadge', width: 100 },
  { title: '加入时间', key: 'joinedAt', width: 180, render: row => formatTime(row.joinedAt) },
  {
    title: '操作',
    key: 'action',
    width: 150,
    render: row => h(NSpace, { size: 4 }, () => [
      canWrite.value ? h(NButton, { size: 'small', secondary: true, onClick: () => editMember(row) }, () => '编辑') : null,
      canWrite.value && row.role !== 2 ? h(NButton, { size: 'small', quaternary: true, type: 'error', onClick: () => beginRemove(row) }, () => '移除') : null
    ])
  }
];

const removeSwitchGuard = scope.addSwitchGuard(canLeave);
onBeforeUnmount(() => { removeSwitchGuard(); detailGeneration += 1; playerGeneration += 1; });

function beforeUnload(event: BeforeUnloadEvent) {
  if (!busy.value && !drawerDirty.value) return;
  event.preventDefault();
  event.returnValue = '';
}
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));
onBeforeRouteLeave(canLeave);

watch(() => scope.serverId, async () => {
  detailGeneration += 1; playerGeneration += 1;
  memberOpen.value = false; addOpen.value = false; removeTarget.value = null; dissolveOpen.value = false;
  drawer.value = false;
  detail.value = null;
  page.value = 1;
  await load();
});
onMounted(async () => {
  if (!scope.loaded) await scope.loadServers();
  window.addEventListener('beforeunload', beforeUnload);
  await load();
});
</script>

<template>
  <div class="ops-page">
    <NAlert v-if="!scope.currentServer" type="info" title="请先在顶部选择区服；公会列表和写入都只允许当前区服。" />
    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false" class="ops-panel">
      <div class="ops-filters">
        <NInput v-model:value="q" placeholder="公会名称或 ID" clearable class="ops-search" @keydown.enter="search" />
        <NButton type="primary" :disabled="!scope.currentServer" @click="search">搜索</NButton>
        <span class="ops-muted">共 {{ total.toLocaleString() }} 个公会</span>
        <NButton :loading="loading" :disabled="!scope.currentServer" @click="load">刷新</NButton>
      </div>
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :row-key="(row: GuildSummary) => row.id"
        :bordered="false"
        :scroll-x="1250"
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

    <NDrawer :show="drawer" :mask-closable="!busy" :close-on-esc="!busy" width="min(1040px, 100vw)" class="ops-editor-drawer" @update:show="value => !value && requestCloseDrawer()">
      <NDrawerContent :title="detail ? `${detail.name} · 公会管理` : '公会管理'" :closable="!busy">
        <NAlert v-if="detailError" type="error" :title="detailError" class="mb-16px" />
        <NAlert v-if="saveError" type="error" :title="saveError" class="mb-16px" />
        <NSpin :show="detailLoading">
          <template v-if="detail && form">
            <div class="guild-detail-header">
              <div>
                <span class="ops-mono ops-muted">ID {{ detail.id }}</span>
              </div>
              <NSpace>
                <NButton :loading="detailLoading" @click="refreshDetail" :disabled="busy">刷新</NButton>
                <NButton v-if="canWrite" quaternary type="error" @click="beginDissolve">解散公会</NButton>
              </NSpace>
            </div>

            <NCard :bordered="false" class="ops-panel mb-16px">
              <template #header>公会共享数值</template>
              <NForm :label-placement="formLabelPlacement" :label-width="viewportWidth < 600 ? undefined : 140" :disabled="!canWrite || busy">
                <div class="guild-form-grid">
                  <NFormItem label="公会名称"><NInput v-model:value="form.name" maxlength="64" /></NFormItem>
                  <NFormItem label="等级"><NInputNumber v-model:value="form.level" :min="1" :max="20" /></NFormItem>
                  <NFormItem label="公会金币"><NInputNumber v-model:value="form.coin" :min="0" /></NFormItem>
                  <NFormItem label="公会徽章"><NInputNumber v-model:value="form.badge" :min="0" /></NFormItem>
                  <NFormItem label="公会积分"><NInputNumber v-model:value="form.exp" :min="0" /></NFormItem>
                  <NFormItem label="捐献积分"><NInputNumber v-model:value="form.donateScore" :min="0" /></NFormItem>
                  <NFormItem label="活跃度"><NInputNumber v-model:value="form.activeScore" :min="0" /></NFormItem>
                  <NFormItem label="公会旗帜"><NInputNumber v-model:value="form.flag" :min="0" /></NFormItem>
                  <NFormItem label="英雄雕像"><NSwitch v-model:value="form.heroStatueOpen" :checked-value="1" :unchecked-value="0" /></NFormItem>
                  <NFormItem label="守护之树"><NSwitch v-model:value="form.guardTreeOpen" :checked-value="1" :unchecked-value="0" /></NFormItem>
                </div>
                <NFormItem label="公会公告"><NInput v-model:value="form.notice" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" /></NFormItem>
                <NFormItem label="公会宣言"><NInput v-model:value="form.declaration" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" /></NFormItem>
                <NFormItem label="公会战自动报名">
                  <NTag :type="detail.unionFightApply ? 'success' : 'default'" :bordered="false">{{ detail.unionFightApply ? '已开启' : '未开启' }}</NTag>
                </NFormItem>
                <NFormItem label="变更原因" required><NInput v-model:value="reason" maxlength="500" placeholder="用于审计" /></NFormItem>
              </NForm>
              <template #footer>
                <NSpace justify="space-between" align="center">
                  <span class="ops-muted">{{ guildDirty ? '有未保存的公会数值修改' : '公会数值已同步' }}</span>
                  <NButton v-if="canWrite" type="primary" :loading="saving" @click="saveGuild">保存公会数值</NButton>
                </NSpace>
              </template>
            </NCard>

            <NCard :bordered="false" class="ops-panel">
              <template #header>
                <NSpace justify="space-between" align="center" style="width: 100%">
                  <span>公会成员 · {{ detail.members.length }} 人</span>
                  <NButton v-if="canWrite" size="small" type="primary" secondary :disabled="guildDirty || busy" @click="beginAdd">添加已有玩家</NButton>
                </NSpace>
              </template>
              <NAlert v-if="guildDirty" type="info" title="请先保存或放弃公会数值修改，再管理成员，避免版本冲突。" class="mb-12px" />
              <NDataTable :columns="memberColumns" :data="detail.members" :row-key="(row: GuildMember) => row.playerId" :scroll-x="950" size="small" />
            </NCard>
          </template>
        </NSpin>
      </NDrawerContent>
    </NDrawer>

    <NModal :show="memberOpen" :closable="!busy" :mask-closable="!busy" @update:show="!$event && memberGuard.requestClose()" preset="card" title="编辑公会成员" style="width: min(560px, calc(100vw - 32px))">
      <NAlert v-if="memberError" type="error" :title="memberError" class="mb-12px" />
      <NForm label-placement="left" label-width="130" :disabled="busy || !canWrite">
        <NFormItem label="角色"><span class="ops-mono">{{ memberTarget?.playerName }} · #{{ memberTarget?.playerId }}</span></NFormItem>
        <NFormItem label="累计贡献"><NInputNumber v-model:value="memberDraft.contribution" :min="0" /></NFormItem>
        <NFormItem label="可用贡献"><NInputNumber v-model:value="memberDraft.spendableContribution" :min="0" :max="2147483647" :precision="0" /></NFormItem>
        <NFormItem label="今日金币"><NInputNumber v-model:value="memberDraft.todayCoin" :min="0" /></NFormItem>
        <NFormItem label="今日徽章"><NInputNumber v-model:value="memberDraft.todayBadge" :min="0" /></NFormItem>
        <NFormItem label="职位"><NSelect v-model:value="memberDraft.role" :options="roleOptions" /></NFormItem>
        <NFormItem label="变更原因" required><NInput v-model:value="memberDraft.reason" maxlength="500" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end"><NButton :disabled="busy" @click="memberGuard.requestClose()">取消</NButton><NButton type="primary" :loading="memberSaving" :disabled="!canWrite || busy" @click="submitMember">保存成员</NButton></NSpace>
      </template>
    </NModal>

    <NModal :show="addOpen" :closable="!busy" :mask-closable="!busy" @update:show="!$event && addGuard.requestClose()" preset="card" title="添加已有玩家" style="width: min(560px, calc(100vw - 32px))">
      <NAlert v-if="addError" type="error" :title="addError" class="mb-12px" />
      <NForm :label-placement="formLabelPlacement" :label-width="viewportWidth < 600 ? undefined : 110" :disabled="busy || !canWrite">
        <NFormItem label="同区角色" required><NSelect v-model:value="addDraft.playerId" :options="playerOptions" :loading="playerLoading" filterable remote clearable placeholder="搜索角色名称或 ID" @search="searchPlayers" /></NFormItem>
        <NFormItem label="加入原因" required><NInput v-model:value="addDraft.reason" maxlength="500" /></NFormItem>
      </NForm>
      <NAlert type="info" title="仅可添加本区服、达到公会开放等级且未加入公会的角色；退会冷却和人数上限仍生效。" />
      <template #footer>
        <NSpace justify="end"><NButton :disabled="busy" @click="addGuard.requestClose()">取消</NButton><NButton type="primary" :loading="addSaving" :disabled="!canWrite || busy" @click="submitAdd">加入公会</NButton></NSpace>
      </template>
    </NModal>

    <NModal :show="Boolean(removeTarget)" preset="card" title="移除公会成员" style="width: min(540px, calc(100vw - 32px))" :closable="!busy" :mask-closable="!busy" @update:show="!$event && removeGuard.requestClose()">
      <NAlert v-if="removeError" type="error" :title="removeError" class="mb-12px" />
      <p>将 <strong>{{ removeTarget?.playerName }}</strong> 移出公会，24小时退会冷却和房间/公会战撤权立即生效；历史记录与奖励保留。</p>
      <NForm label-placement="left" label-width="100" :disabled="busy || !canWrite">
        <NFormItem label="移除原因" required><NInput v-model:value="removeReason" maxlength="500" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end"><NButton :disabled="busy" @click="removeGuard.requestClose()">取消</NButton><NButton type="error" :loading="removing" :disabled="!canWrite || busy" @click="submitRemove">确认移除</NButton></NSpace>
      </template>
    </NModal>

    <NModal :show="dissolveOpen" :closable="!busy" :mask-closable="!busy" @update:show="!$event && dissolveGuard.requestClose()" preset="card" title="二次确认：解散公会" style="width: min(620px, calc(100vw - 32px))">
      <NAlert type="error" title="解散会移除全部成员并撤销公会频道/公会战资格，且不能撤销。历史回执、祈祷、战争结算和已发奖励保留。" class="mb-12px" />
      <NAlert v-if="dissolveError" type="error" :title="dissolveError" class="mb-12px" />
      <NForm label-placement="left" label-width="120" :disabled="busy || !canWrite">
        <NFormItem label="精确公会名" required><NInput v-model:value="dissolveName" :placeholder="detail?.name" /></NFormItem>
        <NFormItem label="解散原因" required><NInput v-model:value="dissolveReason" maxlength="500" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end"><NButton :disabled="busy" @click="dissolveGuard.requestClose()">取消</NButton><NButton type="error" :loading="dissolving" :disabled="busy || !canWrite || dissolveName !== detail?.name" @click="submitDissolve">确认解散</NButton></NSpace>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.guild-detail-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; margin-bottom: 16px; }
.guild-detail-header h2 { margin: 0 0 4px; font-size: 18px; }
.guild-form-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(260px, 100%), 1fr)); gap: 0 16px; }
@media (max-width: 720px) {
  .guild-detail-header { flex-direction: column; }
}
</style>
