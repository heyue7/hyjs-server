<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue';
import { NButton, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, errorMessage, json, setCsrfToken } from '@/service/operations';
import { useAuthStore } from '@/store/modules/auth';
import { clearAuthStorage } from '@/store/modules/auth/shared';
import { useOperations } from '@/composables/useOperations';
interface User {
  id: number;
  username: string;
  role: 'viewer' | 'operator' | 'admin';
}
const auth = useAuthStore();
const { loading, error, run } = useOperations();
const rows = ref<User[]>([]);
const roles = [
  { label: '只读查看', value: 'viewer' },
  { label: '运营操作', value: 'operator' },
  { label: '管理员', value: 'admin' }
];
const roleLabels: Record<string, string> = { viewer: '只读查看', operator: '运营操作', admin: '管理员' };
const roleDescriptions: Record<string, string> = {
  viewer: '查看运营数据，无法修改配置或发放道具。',
  operator: '配置活动、公告和发放道具，无法管理后台账号。',
  admin: '拥有运营权限，并可管理后台账号与角色。'
};
async function load() {
  const data = await run(() => api<{ items: User[] } | User[]>('/users'));
  if (data) rows.value = Array.isArray(data) ? data : data.items;
}
const modal = ref<'create' | 'role' | 'password' | ''>('');
const target = ref<User | null>(null);
const saving = ref(false);
const saveError = ref('');
const draft = reactive({ username: '', password: '', confirmPassword: '', currentPassword: '', role: 'viewer' });
const isSelf = computed(() => target.value?.username === auth.userInfo.userName);
const title = computed(() => {
  if (modal.value === 'password') return isSelf.value ? '修改我的密码' : '重置账号密码';
  if (modal.value === 'create') return '新建后台账号';
  if (modal.value === 'role') return '调整权限';
  return '';
});
function edit(kind: 'create' | 'role' | 'password', user: User | null = null) {
  target.value = user;
  Object.assign(draft, {
    username: '',
    password: '',
    confirmPassword: '',
    currentPassword: '',
    role: user?.role || 'viewer'
  });
  saveError.value = '';
  modal.value = kind;
}
function close() {
  modal.value = '';
  draft.password = '';
  draft.confirmPassword = '';
  draft.currentPassword = '';
}
function relogin() {
  clearAuthStorage();
  setCsrfToken('');
  window.location.assign('/admin/login');
}
function validationMessage() {
  if (modal.value === 'create' && !/^[a-z0-9][a-z0-9_.-]{2,63}$/.test(draft.username))
    return '账号需为3–64位小写字母、数字、下划线、点或短横线';
  if (modal.value === 'role') return '';
  const bytes = new TextEncoder().encode(draft.password).length;
  if (bytes < 12 || bytes > 72) return '密码需为12–72字节（中文占多个字节）';
  if (draft.password !== draft.confirmPassword) return '两次新密码不一致';
  if (modal.value === 'password' && isSelf.value && !draft.currentPassword) return '请填写当前密码';
  return '';
}
async function save() {
  if (saving.value) return;
  saveError.value = validationMessage();
  if (saveError.value) return;
  saving.value = true;
  saveError.value = '';
  try {
    if (modal.value === 'create')
      await api('/users', json('POST', { username: draft.username, password: draft.password, role: draft.role }));
    if (modal.value === 'role') await api(`/users/${target.value!.id}/role`, json('PUT', { role: draft.role }));
    if (modal.value === 'password')
      await api(
        `/users/${target.value!.id}/password`,
        json('PUT', { currentPassword: draft.currentPassword, newPassword: draft.password })
      );
    const needsRelogin =
      isSelf.value && (modal.value === 'password' || (modal.value === 'role' && target.value?.role !== draft.role));
    window.$message?.success(needsRelogin ? '修改成功，请重新登录' : '账号已保存');
    close();
    if (needsRelogin) relogin();
    else await load();
  } catch (e) {
    saveError.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
const columns: DataTableColumns<User> = [
  {
    title: '账号',
    key: 'username',
    minWidth: 240,
    render: user =>
      h('div', [
        h('strong', user.username),
        user.username === auth.userInfo.userName
          ? h(NTag, { size: 'small', bordered: false, style: { marginLeft: '10px' } }, () => '当前账号')
          : null
      ])
  },
  { title: '权限', key: 'role', minWidth: 180, render: user => roleLabels[user.role] || user.role },
  { title: 'ID', key: 'id', width: 100 },
  {
    title: '操作',
    key: 'action',
    width: 200,
    render: user =>
      h(NSpace, { size: 8 }, () => [
        h(NButton, { size: 'small', secondary: true, onClick: () => edit('role', user) }, () => '调整权限'),
        h(NButton, { size: 'small', quaternary: true, onClick: () => edit('password', user) }, () =>
          user.username === auth.userInfo.userName ? '修改密码' : '重置密码'
        )
      ])
  }
];
onMounted(load);
</script>

<template>
  <div class="ops-page">
    <div class="ops-toolbar">
      <div class="ops-page-label">账号管理</div>
      <NSpace>
        <NButton :loading="loading" @click="load">刷新</NButton>
        <NButton type="primary" @click="edit('create')">新建账号</NButton>
      </NSpace>
    </div>
    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false" class="ops-panel">
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :row-key="(row: User) => row.id"
        :bordered="false"
        :scroll-x="700"
      />
    </NCard>
    <NModal
      :show="Boolean(modal)"
      preset="card"
      :title="title"
      class="ops-modal"
      :mask-closable="!saving"
      :closable="!saving"
      @update:show="!$event && close()"
    >
      <div v-if="target" class="grant-object">
        <strong>{{ target.username }}</strong>
        <span class="ops-muted">后台账号 · ID {{ target.id }}</span>
      </div>
      <NAlert v-if="saveError" type="error" :title="saveError" class="mb-20px" />
      <NForm :disabled="saving" label-placement="top">
        <NFormItem v-if="modal === 'create'" label="账号" required>
          <NInput
            v-model:value="draft.username"
            placeholder="3–64位小写字母、数字或 _. -"
            :maxlength="64"
            autocomplete="off"
          />
        </NFormItem>
        <template v-if="modal === 'create' || modal === 'role'">
          <NFormItem label="权限" required><NSelect v-model:value="draft.role" :options="roles" /></NFormItem>
          <p class="ops-muted mb-20px">{{ roleDescriptions[draft.role] }}</p>
        </template>
        <template v-if="modal !== 'role'">
          <NFormItem v-if="modal === 'password' && isSelf" label="当前密码" required>
            <NInput
              v-model:value="draft.currentPassword"
              type="password"
              show-password-on="click"
              autocomplete="current-password"
              placeholder="请输入当前密码"
            />
          </NFormItem>
          <NFormItem :label="modal === 'create' ? '密码' : '新密码'" required>
            <NInput
              v-model:value="draft.password"
              type="password"
              show-password-on="click"
              autocomplete="new-password"
              placeholder="12–72字节"
            />
          </NFormItem>
          <NFormItem label="确认密码" required>
            <NInput
              v-model:value="draft.confirmPassword"
              type="password"
              show-password-on="click"
              autocomplete="new-password"
              placeholder="再输入一次新密码"
            />
          </NFormItem>
        </template>
      </NForm>
      <p v-if="modal === 'role' || modal === 'password'" class="ops-muted">
        修改后，该账号已有登录会话将失效，需要重新登录。
      </p>
      <template #footer>
        <div class="ops-drawer-actions">
          <NButton :disabled="saving" @click="close">取消</NButton>
          <NButton type="primary" :loading="saving" @click="save">保存</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>
