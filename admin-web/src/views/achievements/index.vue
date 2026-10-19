<script setup lang="ts">
import ContextHelp from '@/components/operations/context-help.vue';
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue';
import { NButton, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { onBeforeRouteLeave } from 'vue-router';
import { api, errorMessage, json } from '@/service/operations';
import type { Achievement, AchievementKind } from '@/service/management-operations';
import { useOperations } from '@/composables/useOperations';
import ManagementWorkspace from '@/components/operations/management-workspace.vue';
import RewardList from '@/components/operations/reward-list.vue';
import RewardSummary from '@/components/operations/reward-summary.vue';
const notes = ref('');
const { canWrite, loading, error, run } = useOperations();
const rows = ref<Achievement[]>([]);
const kinds = ref<AchievementKind[]>([]);
const q = ref('');
const kind = ref<string | null>(null);
const state = ref<string | null>(null);
const editing = ref(false);
const selected = ref<Achievement | null>(null);
const fresh = (): Achievement => ({
  id: '',
  name: '',
  kind: '',
  type: 0,
  target: 1,
  rewards: [],
  source: '用户运营配置',
  enabled: false,
  version: ''
});
const draft = ref(fresh());
const original = ref('');
const saving = ref(false);
const saveError = ref('');
const section = ref('goal');
const sections = [
  { key: 'goal', label: '成就目标' },
  { key: 'rewards', label: '完成奖励' },
  { key: 'publish', label: '状态与备注' }
];
const dirty = computed(() => editing.value && original.value !== JSON.stringify(draft.value));
const kindLabel = (value: string) =>
  kinds.value.find((row) => row.id === value)?.label ||
  ({ collection: '历史伙伴收集', warrior_training: '历史强者培养' } as Record<string, string>)[value] ||
  value;
const kindFilterOptions = computed(() =>
  [...new Set([...kinds.value.map((row) => row.id), ...rows.value.map((row) => row.kind)])].map((value) => ({
    value,
    label: kindLabel(value)
  }))
);
const kindOptions = computed(() => {
  const options = kinds.value.map((row) => ({
    value: row.id,
    label: row.supported === false ? `${row.label}（暂不可用）` : row.label,
    disabled: row.supported === false
  }));
  if (selected.value && !options.some((row) => row.value === selected.value!.kind))
    options.unshift({ value: selected.value.kind, label: kindLabel(selected.value.kind), disabled: true });
  return options;
});
const condition = computed(
  () =>
    kinds.value.find((row) => row.id === draft.value.kind) ||
    (selected.value?.kind === draft.value.kind ? selected.value : undefined)
);
const fixedTarget = computed(() => kinds.value.find(row => row.id === draft.value.kind)?.fixedTarget);
const conditionColumns: DataTableColumns<AchievementKind> = [
  { title: '触发条件', key: 'label', minWidth: 230 },
  {
    title: '可用性',
    key: 'supported',
    width: 100,
    render: (row) =>
      h(NTag, { size: 'small', bordered: false, type: row.supported === false ? 'warning' : 'success' }, () =>
        row.supported === false ? '暂不可用' : '可配置'
      )
  },
  { title: '说明', key: 'notes', minWidth: 360, render: (row) => row.notes || '按原进度条件配置' }
];
const visible = computed(() =>
  rows.value.filter(
    (row) =>
      (!kind.value || row.kind === kind.value) &&
      (state.value === null || row.enabled === (state.value === 'enabled')) &&
      `${row.id} ${row.name} ${kindLabel(row.kind)}`.toLowerCase().includes(q.value.trim().toLowerCase())
  )
);
async function load() {
  const data = await run(() =>
    api<{ items: Achievement[]; kinds: AchievementKind[]; notes?: string }>('/achievements')
  );
  if (data) {
    rows.value = (data.items || []).map((row) => ({
      ...row,
      rewards: row.rewards || []
    }));
    kinds.value = data.kinds || [];
    notes.value = data.notes || '';
  }
}
function edit(row: Achievement | null) {
  selected.value = row;
  draft.value = row ? JSON.parse(JSON.stringify(row)) : fresh();
  if (fixedTarget.value !== undefined) draft.value.target = fixedTarget.value;
  original.value = JSON.stringify(draft.value);
  section.value = 'goal';
  saveError.value = '';
  editing.value = true;
}
function setKind(value: string) {
  if (kinds.value.find((row) => row.id === value)?.supported === false) return;
  draft.value.kind = value;
  draft.value.type = kinds.value.find((row) => row.id === value)?.type ?? 0;
  if (fixedTarget.value !== undefined) draft.value.target = fixedTarget.value;
}
function back() {
  if (!dirty.value || !canWrite.value) {
    editing.value = false;
    return;
  }
  window.$dialog?.warning({
    title: '放弃未保存的成就修改？',
    positiveText: '放弃',
    negativeText: '继续编辑',
    onPositiveClick: () => {
      editing.value = false;
    }
  });
}
async function save() {
  if (saving.value || !canWrite.value) return;
  saveError.value = !draft.value.name.trim()
    ? '请填写成就名称'
    : draft.value.enabled && condition.value?.supported === false
      ? `此条件暂不能启用：${condition.value.notes || '对应玩法尚未恢复'}`
      : !draft.value.kind
        ? '请选择成就条件'
        : !Number.isInteger(draft.value.target) || draft.value.target < 1
          ? '目标需为正整数'
          : !draft.value.rewards.length ||
              draft.value.rewards.some((r) => !r.resourceId || !Number.isInteger(r.amount) || r.amount < 1)
            ? '请填写完整的完成奖励'
            : '';
  if (saveError.value) return;
  saving.value = true;
  try {
    await api(
      `/achievements${selected.value ? `/${encodeURIComponent(selected.value.id)}` : ''}`,
      json(selected.value ? 'PUT' : 'POST', draft.value)
    );
    original.value = JSON.stringify(draft.value);
    editing.value = false;
    window.$message?.success('成就已保存');
    await load();
  } catch (e) {
    saveError.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
function remove(row: Achievement) {
  window.$dialog?.warning({
    title: '停用并移除成就？',
    content: `${row.name} 将停止发放新奖励，已领取的奖励不受影响。`,
    positiveText: '移除',
    negativeText: '取消',
    onPositiveClick: async () => {
      await run(async () => {
        await api(`/achievements/${encodeURIComponent(row.id)}`, json('DELETE', { version: row.version }));
        await load();
      });
    }
  });
}
const columns: DataTableColumns<Achievement> = [
  {
    title: '成就',
    key: 'name',
    minWidth: 210,
    render: (row) =>
      h('div', [
        h('strong', row.name || row.id),
        ...(row.supported === false
          ? [h('div', { class: 'ops-muted mt-5px' }, `暂不可用：${row.notes || '对应玩法尚未恢复'}`)]
          : [])
      ])
  },
  {
    title: '目标',
    key: 'target',
    minWidth: 190,
    render: (row) => `${kindLabel(row.kind)} · ${row.target.toLocaleString()}`
  },
  {
    title: '奖励',
    key: 'rewards',
    minWidth: 250,
    render: (row) => h(RewardSummary, { rewards: row.rewards })
  },
  {
    title: '状态',
    key: 'enabled',
    width: 90,
    render: (row) =>
      h(
        NTag,
        {
          size: 'small',
          bordered: false,
          type: row.enabled ? 'success' : 'default'
        },
        () => (row.enabled ? '启用' : '关闭')
      )
  },
  {
    title: '操作',
    key: 'action',
    width: 150,
    render: (row) =>
      h(NSpace, { size: 5 }, () => [
        h(NButton, { size: 'small', secondary: true, onClick: () => edit(row) }, () =>
          canWrite.value ? '编辑' : '查看'
        ),
        ...(canWrite.value
          ? [
              h(
                NButton,
                {
                  size: 'small',
                  quaternary: true,
                  type: 'error',
                  onClick: () => remove(row)
                },
                () => '移除'
              )
            ]
          : [])
      ])
  }
];
onBeforeRouteLeave(() => {
  if (!dirty.value || !canWrite.value) return true;
  return new Promise<boolean>((resolve) =>
    window.$dialog?.warning({
      title: '离开成就编辑？',
      content: '未保存的修改将丢失。',
      positiveText: '离开',
      negativeText: '继续编辑',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false)
    })
  );
});
function beforeUnload(event: BeforeUnloadEvent) {
  if (dirty.value && canWrite.value) {
    event.preventDefault();
    event.returnValue = '';
  }
}
window.addEventListener('beforeunload', beforeUnload);
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));
onMounted(load);
</script>

<template>
  <ManagementWorkspace
    :title="editing ? `${selected ? (canWrite ? '编辑' : '查看') : '新建'}成就` : '成就管理'"
    :editing="editing"
    :saving="saving"
    :readonly="!canWrite"
    :dirty="dirty"
    :sections="sections"
    v-model:section="section"
    @back="back"
    @save="save"
  >
    <template #title-extra><ContextHelp v-if="notes" title="成就配置说明" :text="notes" /></template>
    <template v-if="!editing" #actions>
      <NButton :loading="loading" @click="load">刷新</NButton>
      <NButton v-if="canWrite" type="primary" @click="edit(null)">新建成就</NButton>
    </template>
    <template #alert>
      <NAlert v-if="error || saveError" type="error" :title="saveError || error" />
    </template>
    <NCard v-if="!editing" :bordered="false" class="ops-panel">
      <NCollapse class="mb-24px">
        <NCollapseItem title="条件目录" name="conditions">
          <NDataTable
            :columns="conditionColumns"
            :data="kinds"
            :row-key="(row: AchievementKind) => row.id"
            :bordered="false"
            :scroll-x="720"
            :max-height="420"
          />
        </NCollapseItem>
      </NCollapse>
      <div class="ops-filters">
        <NInput v-model:value="q" clearable class="ops-search" placeholder="搜索成就名称或 ID" />
        <NSelect
          v-model:value="kind"
          :options="kindFilterOptions"
          clearable
          class="ops-filter-select"
          placeholder="全部类型"
        />
        <NSelect
          v-model:value="state"
          clearable
          :options="[
            { label: '启用', value: 'enabled' },
            { label: '关闭', value: 'disabled' }
          ]"
          class="ops-filter-select"
          placeholder="全部状态"
        />
      </div>
      <NDataTable
        :columns="columns"
        :data="visible"
        :row-key="(row: Achievement) => row.id"
        :loading="loading"
        :bordered="false"
        :pagination="{ pageSize: 20 }"
        :scroll-x="900"
      />
    </NCard>
    <NForm v-else label-placement="top" :disabled="saving || !canWrite">
      <section v-show="section === 'goal'">
        <h3 class="mb-20px">成就目标</h3>
        <div class="ops-date-grid">
          <NFormItem v-if="selected" label="成就编号"><span class="ops-mono ops-muted">{{ draft.id }}</span></NFormItem>
          <NFormItem label="显示名称" required><NInput v-model:value="draft.name" placeholder="成就名称" /></NFormItem>
        </div>
        <NFormItem label="进度条件" required>
          <NSelect :value="draft.kind" :options="kindOptions" placeholder="选择触发条件" @update:value="setKind" />
        </NFormItem>
        <NAlert
          v-if="condition?.notes"
          :type="condition.supported === false ? 'warning' : 'info'"
          :title="condition.notes"
          class="mb-20px"
        />
        <NFormItem label="达成目标" required>
          <NInputNumber v-model:value="draft.target" :disabled="fixedTarget !== undefined || saving || !canWrite" :min="1" :precision="0" />
        </NFormItem>
      </section>
      <section v-show="section === 'rewards'">
        <h3 class="mb-20px">完成奖励</h3>
        <RewardList v-model:value="draft.rewards" :disabled="saving || !canWrite" />
      </section>
      <section v-show="section === 'publish'">
        <h3 class="mb-20px">状态与备注</h3>
        <NFormItem label="启用成就">
          <NSwitch
            v-model:value="draft.enabled"
            :disabled="saving || !canWrite || (condition?.supported === false && !draft.enabled)"
            aria-label="启用成就"
          />
        </NFormItem>
        <NFormItem label="配置备注">
          <NInput v-model:value="draft.source" type="textarea" :maxlength="1000" />
        </NFormItem>
        <p class="ops-inline-note">修改仅影响之后的新奖励，已领取的记录不变。</p>
      </section>
    </NForm>
  </ManagementWorkspace>
</template>
