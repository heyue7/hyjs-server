<script setup lang="ts">
import { useDraftGuard } from '@/composables/useDraftGuard';
import { computed, h, reactive, ref } from 'vue';
import { useRouter } from 'vue-router';
import { NButton, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { ApiError, errorMessage, formatTime } from '@/service/operations';
import { createServer, updateServer } from '@/service/servers';
import type { GameServer } from '@/service/servers';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { useOperations } from '@/composables/useOperations';

const router = useRouter();
const scope = useServerScopeStore();
const { canWrite } = useOperations();
const open = ref(false);
const saving = ref(false);
const conflict = ref(false);
const error = ref('');
const target = ref<GameServer | null>(null);
const draft = reactive({ name: '', enabled: false, openAt: Date.now() });
const editorGuard = useDraftGuard({ active: () => open.value, value: () => draft, busy: () => saving.value, writable: () => canWrite.value, label: '区服修改', close: () => { open.value = false; } });
const search = ref('');
const rows = computed(() =>
  scope.servers.filter(row => `${row.id} ${row.name}`.toLowerCase().includes(search.value.toLowerCase()))
);
function edit(row?: GameServer) {
  target.value = row ? { ...row } : null;
  Object.assign(
    draft,
    row
      ? { name: row.name, enabled: row.enabled, openAt: row.openAt }
      : { name: '', enabled: false, openAt: Date.now() }
  );
  error.value = '';
  conflict.value = false;
  editorGuard.capture();
  open.value = true;
}
async function save() {
  if (saving.value || conflict.value || !canWrite.value) return;
  if (!draft.name.trim() || draft.name.trim().length > 40 || !Number.isFinite(draft.openAt)) {
    error.value = '请填写 1～40 字的区服名称和开服日期';
    return;
  }
  saving.value = true;
  error.value = '';
  try {
    if (target.value)
      await updateServer(target.value.id, { ...draft, name: draft.name.trim(), version: target.value.version });
    else await createServer({ name: draft.name.trim(), openAt: draft.openAt });
    await scope.loadServers();
    open.value = false;
    window.$message?.success(target.value ? '区服设置已保存' : '区服已创建，可开始编排活动');
  } catch (e) {
    error.value = errorMessage(e);
    conflict.value = e instanceof ApiError && e.status === 409;
  } finally {
    saving.value = false;
  }
}
function operate(row: GameServer) {
  scope.setServer(row.id);
  router.push('/activity-schedules');
}
function serverStatus(row: GameServer): { label: string; type: 'success' | 'warning' | 'default' } {
  if (!row.runtimeBound) return { label: '待部署', type: 'default' };
  return row.enabled ? { label: '运行中', type: 'success' } : { label: '已关闭', type: 'warning' };
}
const columns: DataTableColumns<GameServer> = [
  {
    title: '区服',
    key: 'name',
    minWidth: 220,
    render: row => h('div', [h('strong', row.name), h('div', { class: 'ops-muted' }, `${row.id} 区`)])
  },
  { title: '开服日期（北京）', key: 'openAt', minWidth: 190, render: row => formatTime(row.openAt) },
  {
    title: '运行状态',
    key: 'enabled',
    width: 150,
    render: row =>
      h(NTag, { size: 'small', bordered: false, type: serverStatus(row).type }, () => serverStatus(row).label)
  },
  {
    title: '操作',
    key: 'actions',
    minWidth: 220,
    render: row =>
      h('div', { class: 'flex gap-8px' }, [
        h(NButton, { size: 'small', secondary: true, onClick: () => operate(row) }, () => '活动排期'),
        h(NButton, { size: 'small', disabled: !canWrite.value, onClick: () => edit(row) }, () => '编辑区服')
      ])
  }
];
</script>

<template>
  <div class="ops-page">

    <NAlert v-if="scope.error" type="error" :title="scope.error" />
    <p class="ops-inline-note">待部署区服可提前安排活动，绑定游戏服务后才能开放登录。</p>
    <NCard :bordered="false" class="ops-panel">
      <div class="ops-filters">
        <NInput v-model:value="search" placeholder="搜索区服名称或编号" clearable class="ops-search" />
        <NSpace>
          <NButton :loading="scope.loading" @click="scope.loadServers">刷新</NButton>
          <NButton v-if="canWrite" type="primary" @click="edit()">新建区服</NButton>
        </NSpace>
      </div>
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="scope.loading"
        :row-key="(row: GameServer) => row.id"
        :bordered="false"
        :scroll-x="850"
      />
    </NCard>
    <NModal
      :show="open"
      @update:show="!$event && editorGuard.requestClose()"
      preset="card"
      class="ops-modal"
      :title="target ? `编辑 ${target.id} 区` : '新建区服'"
      :mask-closable="!saving"
      :closable="!saving"
    >
      <NAlert v-if="error" type="error" :title="error" class="mb-20px" />
      <NAlert
        v-if="conflict"
        type="warning"
        title="该区服已被其他操作更新。当前填写内容已保留，请关闭后刷新再编辑。"
        class="mb-20px"
      />
      <NForm label-placement="top" :disabled="saving || !canWrite">
        <NFormItem label="区服名称" required>
          <NInput v-model:value="draft.name" :maxlength="40" placeholder="例如：灵魂之花 1 区" />
        </NFormItem>
        <NFormItem label="开服日期" required>
          <NDatePicker v-model:value="draft.openAt" type="datetime" class="w-full" />
        </NFormItem>
        <NFormItem v-if="target?.runtimeBound" label="玩家可进入"><NSwitch v-model:value="draft.enabled" /></NFormItem>
        <p v-else class="ops-muted">尚未绑定游戏运行服务，创建后为待部署状态。</p>
      </NForm>
      <template #footer>
        <div class="ops-drawer-actions">
          <NButton :disabled="saving" @click="editorGuard.requestClose()">取消</NButton>
          <NButton type="primary" :loading="saving" :disabled="conflict || !canWrite" @click="save">保存区服</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>
