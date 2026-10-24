<script setup lang="ts">
import { useDraftGuard } from '@/composables/useDraftGuard';
import { computed, h, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { useWindowSize } from '@vueuse/core';
import { NButton, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { useAuthStore } from '@/store/modules/auth';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { useOperations } from '@/composables/useOperations';
import { ApiError, errorMessage, formatTime } from '@/service/operations';
import { createGameAccount, getGameAccounts, resetGameAccountPassword, updateGameAccount } from '@/service/game-accounts';
import type { AccountPlayer, GameAccount } from '@/service/game-accounts';
const { width: viewportWidth } = useWindowSize();

const router = useRouter();
const route = useRoute();
const locatedId = computed(() => { const id = Number(route.query.accountId); return Number.isSafeInteger(id) && id > 0 ? id : 0; });
function rowClass(row: GameAccount) { return row.id === locatedId.value ? 'account-located-row' : ''; }
function applyLocation() { q.value = typeof route.query.q === 'string' ? route.query.q : ''; page.value = 1; load(); }
const scope = useServerScopeStore();
const auth = useAuthStore();
const { canWrite } = useOperations();
const isAdmin = computed(() => auth.userInfo.roles.includes('admin'));
const rows = ref<GameAccount[]>([]);
const q = ref('');
const page = ref(1);
const pageSize = ref(25);
const total = ref(0);
const loading = ref(false);
const error = ref('');
let generation = 0;
async function load() {
  const current = ++generation;
  loading.value = true;
  error.value = '';
  try {
    const result = await getGameAccounts(q.value.trim(), page.value, pageSize.value, locatedId.value);
    if (current !== generation) return;
    rows.value = (result.items || []).map(row => ({ ...row, players: row.players || [] }));
    total.value = result.total;
  } catch (e) {
    if (current === generation) error.value = errorMessage(e);
  } finally {
    if (current === generation) loading.value = false;
  }
}
function search() {
  page.value = 1;
  if (locatedId.value) { router.replace({ query: q.value.trim() ? { q: q.value.trim() } : {} }); return; }
  load();
}
const modal = ref<'' | 'create' | 'edit' | 'password'>('');
const target = ref<GameAccount | null>(null);
const saving = ref(false);
const conflict = ref(false);
const saveError = ref('');
const draft = reactive({ username: '', nickname: '冒险者', password: '', confirmPassword: '', enabled: true, reason: '' });
const editorGuard = useDraftGuard({ active: () => Boolean(modal.value), value: () => draft, busy: () => saving.value, writable: () => canWrite.value, label: '账号修改', close: () => { modal.value = ''; draft.password = ''; draft.confirmPassword = ''; } });
const title = computed(() => ({ create: '新建 Brain Burst 2039 账号', edit: '编辑 Brain Burst 2039 账号', password: '重置 Brain Burst 2039 密码' })[modal.value || 'edit']);
function edit(kind: 'create' | 'edit' | 'password', row: GameAccount | null = null) {
  if (!canWrite.value || row?.protected || (kind === 'password' && !isAdmin.value)) return;
  target.value = row ? { ...row } : null;
  Object.assign(draft, { username: row?.username || '', nickname: row?.nickname || '冒险者', password: '', confirmPassword: '', enabled: row?.enabled ?? true, reason: '' });
  saveError.value = '';
  conflict.value = false;
  editorGuard.capture();
  modal.value = kind;
}
function close() { return editorGuard.requestClose(); }

function validate() {
  if (modal.value === 'create' && !/^[A-Za-z0-9_-]{3,32}$/.test(draft.username)) return '账号需为 3–32 位字母、数字、下划线或连字符';
  if (modal.value !== 'password' && !draft.nickname.trim()) return '请填写账号名称';
  if (modal.value === 'create' || modal.value === 'password') {
    if (Array.from(draft.password).length < 6 || new TextEncoder().encode(draft.password).length > 72) return '密码至少 6 个字符，中文和符号合计不超过 72 字节';
    if (draft.password !== draft.confirmPassword) return '两次密码输入不一致';
  }
  if (!draft.reason.trim()) return '请填写操作原因';
  return '';
}
async function save() {
  if (saving.value || conflict.value || !canWrite.value || target.value?.protected) return;
  if (modal.value === 'password' && !isAdmin.value) return;
  saveError.value = validate();
  if (saveError.value) return;
  saving.value = true;
  try {
    const reason = draft.reason.trim();
    if (modal.value === 'create') await createGameAccount({ username: draft.username, nickname: draft.nickname.trim(), password: draft.password, confirmPassword: draft.confirmPassword, reason });
    else if (modal.value === 'password') await resetGameAccountPassword(target.value!.id, { password: draft.password, confirmPassword: draft.confirmPassword, version: target.value!.version, reason });
    else await updateGameAccount(target.value!.id, { nickname: draft.nickname.trim(), enabled: draft.enabled, version: target.value!.version, reason });
    window.$message?.success(modal.value === 'password' ? '密码已重置' : modal.value === 'create' ? 'Brain Burst 2039 账号已创建' : '账号已保存');
    modal.value = '';
    editorGuard.clear();
    draft.password = '';
    draft.confirmPassword = '';
    await load();
  } catch (e) {
    saveError.value = errorMessage(e);
    conflict.value = e instanceof ApiError && e.status === 409;
  } finally {
    saving.value = false;
  }
}
const navigating = ref(false);
async function openPlayers(row: AccountPlayer, wallet = false) {
  if (!row.name || !row.playerId || navigating.value) return;
  navigating.value = true;
  try {
    if (!await scope.requestServerChange(row.serverId) || scope.serverId !== row.serverId) return;
    await router.push({ path: '/players', query: { serverId: row.serverId, q: row.name, ...(wallet ? { wallet: row.playerId } : { playerId: row.playerId }) } });
  } catch (e) { window.$message?.error(errorMessage(e)); }
  finally { navigating.value = false; }
}
const serverName = (id: number) => scope.servers.find(row => row.id === id)?.name || `${id} 区`;
const selected = ref<GameAccount | null>(null);
const playerColumns: DataTableColumns<AccountPlayer> = [
  { title: '区服', key: 'serverId', minWidth: 170, render: row => serverName(row.serverId) },
  { title: '角色', key: 'name', minWidth: 160, render: row => row.name || '未创建角色' },
  { title: '等级', key: 'level', width: 90, render: row => row.name ? `Lv.${row.level}` : '—' },
  { title: '操作', key: 'actions', width: 220, render: row => h(NSpace, { size: 6 }, () => [
    h(NButton, { size: 'small', secondary: true, disabled: !row.name || !row.playerId || navigating.value, onClick: () => openPlayers(row) }, () => '查看角色'),
    h(NButton, { size: 'small', secondary: true, type: 'primary', disabled: !row.name || !row.playerId || selected.value?.protected || navigating.value, onClick: () => openPlayers(row, true) }, () => '蓝绿终端')
  ]) }
];
const playersOpen = ref(false);
function viewPlayers(row: GameAccount) { selected.value = row; playersOpen.value = true; }
const columns: DataTableColumns<GameAccount> = [
  { title: '登录账号', key: 'username', minWidth: 190, render: row => h('div', [h('strong', row.username), row.protected ? h(NTag, { size: 'small', bordered: false, style: 'margin-left:8px' }, () => '受保护') : null]) },
  { title: '账号名称', key: 'nickname', minWidth: 140 },
  { title: '角色数量', key: 'players', width: 110, render: row => h(NButton, { text: true, type: 'primary', onClick: () => viewPlayers(row) }, () => `${row.players.length} 个角色`) },
  { title: '登录状态', key: 'enabled', width: 110, render: row => h(NTag, { size: 'small', bordered: false, type: row.enabled ? 'success' : 'default' }, () => row.enabled ? '允许登录' : '已停用') },
  { title: '注册时间', key: 'createdAt', minWidth: 180, render: row => formatTime(row.createdAt) },
  { title: '操作', key: 'actions', minWidth: 260, render: row => h(NSpace, { size: 6 }, () => [
    h(NButton, { size: 'small', secondary: true, onClick: () => viewPlayers(row) }, () => '查看角色列表'),
    ...(canWrite.value ? [h(NButton, { size: 'small', secondary: true, disabled: row.protected, onClick: () => edit('edit', row) }, () => '编辑')] : []),
    ...(isAdmin.value ? [h(NButton, { size: 'small', quaternary: true, disabled: row.protected, onClick: () => edit('password', row) }, () => '重置密码')] : [])
  ]) }
];
onMounted(applyLocation);
watch(() => [route.query.accountId, route.query.q], applyLocation);
onBeforeUnmount(() => { generation += 1; draft.password = ''; draft.confirmPassword = ''; });
</script>

<template>
  <div class="ops-page">

    <div class="account-heading"><img src="/media/brain-burst-2039.png" alt="" /><div><h1>Brain Burst 2039 账号</h1></div></div>
    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false" class="ops-panel">
      <p v-if="locatedId" class="ops-inline-note">账号 #{{ locatedId }} <NButton text type="primary" @click="q = ''; search()">查看全部账号</NButton></p>
      <div class="ops-filters">
        <NInput v-model:value="q" clearable class="ops-search" placeholder="搜索登录账号或账号名称" @keydown.enter="search" />
        <NButton type="primary" @click="search">搜索</NButton>
        <span class="ops-muted">共 {{ total.toLocaleString() }} 个账号</span>
        <NSpace>
          <NButton :loading="loading" @click="load">刷新</NButton>
          <NButton v-if="canWrite" type="primary" @click="edit('create')">新建账号</NButton>
        </NSpace>
      </div>
      <NDataTable :columns="columns" :data="rows" :loading="loading" :row-key="(row: GameAccount) => row.id" :row-class-name="rowClass" :bordered="false" :scroll-x="1100" />
      <div class="ops-pagination"><NPagination v-model:page="page" v-model:page-size="pageSize" :item-count="total" :page-sizes="[25, 50, 100]" show-size-picker @update:page="load" @update:page-size="search" /></div>
    </NCard>
    <NModal :show="Boolean(modal)" preset="card" :title="title" class="ops-modal account-editor" :mask-closable="!saving" :closable="!saving" @update:show="!$event && close()">
      <div v-if="target" class="account-context"><strong>{{ target.username }}</strong><span class="ops-muted">账号编号 {{ target.id }}</span></div>
      <NAlert v-if="saveError" type="error" :title="saveError" class="mb-16px" />
      <p v-if="conflict" class="ops-inline-note">请关闭并刷新列表，再重新编辑。当前输入仍保留。</p>
      <NForm label-placement="top" :disabled="saving || conflict" @submit.prevent="save">
        <NFormItem v-if="modal === 'create'" label="登录账号" required><NInput v-model:value="draft.username" :maxlength="32" autocomplete="username" placeholder="3–32 位字母、数字、_ 或 -" /></NFormItem>
        <NFormItem v-if="modal !== 'password'" label="账号名称" required><NInput v-model:value="draft.nickname" :maxlength="40" placeholder="与各区服的角色名分别设置" /></NFormItem>
        <template v-if="modal === 'create' || modal === 'password'">
          <NFormItem :label="modal === 'create' ? '登录密码' : '新密码'" required><NInput v-model:value="draft.password" type="password" show-password-on="click" autocomplete="new-password" placeholder="至少 6 个字符" /></NFormItem>
          <NFormItem label="确认密码" required><NInput v-model:value="draft.confirmPassword" type="password" show-password-on="click" autocomplete="new-password" placeholder="再次输入密码" /></NFormItem>
        </template>
        <NFormItem v-if="modal === 'edit'" label="允许登录"><NSwitch v-model:value="draft.enabled" /><template #feedback>停用后，该账号下所有区服的角色将无法登录。</template></NFormItem>
        <NFormItem label="操作原因" required><NInput v-model:value="draft.reason" type="textarea" :maxlength="300" :autosize="{ minRows: 2, maxRows: 4 }" placeholder="记录本次调整用途" /></NFormItem>
        <p v-if="modal === 'password'" class="ops-inline-note">新密码用于该账号下所有区服，不会显示历史密码。</p>
      </NForm>
      <template #footer><div class="ops-drawer-actions"><NButton :disabled="saving" @click="close">取消</NButton><NButton type="primary" :loading="saving" :disabled="conflict" @click="save">{{ modal === 'create' ? '创建账号' : modal === 'password' ? '重置密码' : '保存' }}</NButton></div></template>
    </NModal>
    <NDrawer v-model:show="playersOpen" :width="Math.min(820, viewportWidth)"><NDrawerContent :title="`${selected?.nickname || selected?.username || ''} · 区服角色`" closable><p class="ops-inline-note">登录账号：{{ selected?.username }}。每个账号在每服可创建 1 个角色；角色进度与蓝绿终端余额按区服独立。</p><NDataTable :columns="playerColumns" :data="selected?.players || []" :bordered="false" :row-key="(row: AccountPlayer) => `${row.serverId}:${row.playerId}`" :scroll-x="680" /><template #footer><NButton @click="playersOpen = false">关闭</NButton></template></NDrawerContent></NDrawer>
  </div>
</template>

<style scoped>
:deep(.account-located-row td){background:rgb(var(--primary-color) / .07)!important}:deep(.account-located-row td:first-child){box-shadow:inset 3px 0 rgb(var(--primary-color))}
.account-heading{display:flex;align-items:center;gap:12px}.account-heading img{width:40px;height:40px;object-fit:contain;border-radius:6px}.account-heading h1{font-size:16px;font-weight:600;margin:0;overflow-wrap:anywhere}
.account-editor{width:540px;max-width:calc(100vw - 32px)}.account-context{display:flex;align-items:center;justify-content:space-between;gap:12px;margin-bottom:16px}
@media(max-width:600px){.account-context{align-items:flex-start;flex-direction:column;gap:4px}}
</style>
