<script setup lang="ts">
import ContextHelp from './context-help.vue';
import { useDraftGuard } from '@/composables/useDraftGuard';
import { h, onMounted, reactive, ref } from 'vue';
import { NButton, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, errorMessage, json } from '@/service/operations';
import { money } from '@/service/wallet-operations';
import type { WalletReason } from '@/service/wallet-operations';
import { useOperations } from '@/composables/useOperations';
const { canWrite, loading, error, run } = useOperations();
const rows = ref<WalletReason[]>([]);
const open = ref(false);
const saving = ref(false);
const saveError = ref('');
const draft = reactive<WalletReason>({
  id: '',
  label: '',
  minCents: 0,
  maxCents: 0,
  weight: 0,
  enabled: false,
  version: ''
});
const editorGuard = useDraftGuard({ active: () => open.value, value: () => draft, busy: () => saving.value, writable: () => canWrite.value, label: '入账规则修改', close: () => { open.value = false; } });
async function load() {
  const result = await run(() => api<{ items: WalletReason[] }>('/wallet-reasons'));
  if (result) rows.value = result.items;
}
function edit(row: WalletReason) {
  Object.assign(draft, row);
  saveError.value = '';
  editorGuard.capture();
  open.value = true;
}
async function save() {
  if (!canWrite.value || saving.value) return;
  if (!draft.label.trim() || draft.minCents <= 0 || draft.maxCents < draft.minCents || draft.weight < 1) {
    saveError.value = '请填写理由、有效金额范围及正整数权重';
    return;
  }
  saving.value = true;
  saveError.value = '';
  try {
    await api(`/wallet-reasons/${encodeURIComponent(draft.id)}`, json('PUT', { ...draft }));
    window.$message?.success('入账理由已保存');
    open.value = false;
    await load();
  } catch (e) {
    saveError.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
const columns: DataTableColumns<WalletReason> = [
  { title: '入账理由', key: 'label', minWidth: 240 },
  {
    title: '金额范围（元）',
    key: 'minCents',
    width: 200,
    render: row => `${money(row.minCents)} ～ ${money(row.maxCents)}`
  },
  { title: '相对权重', key: 'weight', width: 110 },
  {
    title: '状态',
    key: 'enabled',
    width: 100,
    render: row =>
      h(NTag, { size: 'small', bordered: false, type: row.enabled ? 'success' : 'default' }, () =>
        row.enabled ? '已启用' : '已关闭'
      )
  },
  {
    title: '操作',
    key: 'action',
    width: 90,
    render: row =>
      h(NButton, { size: 'small', secondary: true, onClick: () => edit(row) }, () => (canWrite.value ? '编辑' : '查看'))
  }
];
onMounted(load);
</script>

<template>
  <div>
    <div class="ops-filters">
      <ContextHelp title="入账规则" text="添加余额时，在对应金额范围内随机选取理由。" />
      <NButton :loading="loading" @click="load">刷新</NButton>
    </div>
    <NAlert v-if="error" type="error" :title="error" class="mb-20px" />
    <NDataTable
      :columns="columns"
      :data="rows"
      :loading="loading"
      :row-key="(row: WalletReason) => row.id"
      :bordered="false"
      :scroll-x="750"
      :pagination="{ pageSize: 15 }"
    />
    <NModal
      :show="open"
      @update:show="!$event && editorGuard.requestClose()"
      preset="card"
      title="入账理由"
      class="ops-modal"
      :mask-closable="!saving"
      :closable="!saving"
    >
      <NAlert v-if="saveError" type="error" :title="saveError" class="mb-20px" />
      <NForm label-placement="top" :disabled="!canWrite || saving">
        <NFormItem label="理由名称" required>
          <NInput v-model:value="draft.label" :maxlength="100" placeholder="可用 {amount} 显示实际入账金额" />
        </NFormItem>
        <div class="ops-date-grid">
          <NFormItem label="最低金额（元）" required>
            <NInputNumber
              :value="draft.minCents / 100"
              :min="0.01"
              :precision="2"
              class="w-full"
              @update:value="draft.minCents = Math.round(($event || 0) * 100)"
            />
          </NFormItem>
          <NFormItem label="最高金额（元）" required>
            <NInputNumber
              :value="draft.maxCents / 100"
              :min="0.01"
              :precision="2"
              class="w-full"
              @update:value="draft.maxCents = Math.round(($event || 0) * 100)"
            />
          </NFormItem>
        </div>
        <NFormItem label="相对权重" required>
          <NInputNumber v-model:value="draft.weight" :min="1" :precision="0" class="w-full" />
        </NFormItem>
        <NFormItem label="参与抽取"><NSwitch v-model:value="draft.enabled" /></NFormItem>
      </NForm>
      <template #footer>
        <div class="ops-drawer-actions">
          <NButton :disabled="saving" @click="editorGuard.requestClose()">关闭</NButton>
          <NButton v-if="canWrite" type="primary" :loading="saving" @click="save">保存</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>
