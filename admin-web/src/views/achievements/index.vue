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
  { key: 'publish', label: '状态' }
];
const dirty = computed(() => editing.value && original.value !== JSON.stringify(draft.value));
const kindLabel = (value: string) =>
  kinds.value.find((row) => row.id === value)?.label ||
  ({ collection: '图鉴收集', warrior_training: '历史强者培养', 'login-days': '累计登录', 'copy-stages': '主线通关', 'formation-count': '上阵人数', 're-os-growth': 'OS角色成长' } as Record<string, string>)[value] ||
  '未识别条件';
const targetUnit = (value: string) =>
  ({ 'login-days': '天', collection: '种角色', 'copy-stages': '关', 'formation-count': '人', 're-os-growth': '项', level: '级', 'level-advanced': '级', 'copy-stars': '颗星', 'copy-stars-advanced': '颗星' } as Record<string, string>)[value] || '';
const isOSGrowth = computed(() => draft.value.kind === 're-os-growth');
const osRequirements = computed(() => draft.value.requirements);
function goalSummary(row: Achievement) {
  if (row.kind === 're-os-growth') {
    const goal = row.requirements;
    return goal ? `登录 ${goal.loginDays} 天 · 等级 ${goal.level} 级 · 通关 ${goal.stages} 关` : '请配置登录天数、角色等级和主线关卡数';
  }
  return `${kindLabel(row.kind)} · ${row.target.toLocaleString()}${targetUnit(row.kind)}`;
}
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
const fixedTarget = computed(() => isOSGrowth.value ? 3 : kinds.value.find(row => row.id === draft.value.kind)?.fixedTarget);
// Display rules reviewed against AdminAchievementKinds; API notes remain available for unknown conditions.
const conditionRules: Record<string, string> = {
  vip: '会员等级达到目标。',
  level: '主角等级达到目标。',
  'level-advanced': '主角等级达到目标，分段目标可单独配置。',
  'copy-stars': '按各主线关卡的历史最高星数合计，同一关卡不重复计数。',
  'copy-stars-advanced': '与副本总星数（一）使用相同统计范围，分段目标可单独配置。',
  fight: '阵容战斗力达到目标。',
  'equipped-heroes': '统计穿齐四件装备的上阵角色人数。',
  'treasure-heroes': '统计穿齐两件宝物的上阵角色人数。',
  trial: '按试炼历史最高总星数计算，层数不计入。',
  'formation-level': '至少四名角色上阵，四人等级都须达到目标。',
  'expedition-hours': '累计已接受远征任务的已完成整小时，每个任务最多计入开始时的计划时长，不乘伙伴人数。',
  'wanted-kills': '仅统计本人完成的首领击杀，参与伤害不计为击杀。',
  'formation-breach': '至少四名角色上阵，四人的突破等级都须达到目标。',
  'friend-disaster-help': '统计本人成功帮助其他玩家解决厄运的次数，重复请求不重复计数。',
  'pet-count': '累计获得的精灵逐只去重，含已回收及成长消耗记录；缺失的历史记录不补计。',
  'pet-elements': '按相同模型的精灵分组，每组元素去重，取元素最多的一组。',
  'pet-level': '取当前精灵及保留的历史记录中的最高等级。',
  'pet-breach': '取精灵最高突破等级。',
  'pet-talents': '按单只精灵已解锁的不同天赋技能计数，取最多的一只；解锁条件为突破等级达到技能要求。',
  'pet-intimacy': '取精灵最高亲密度星级，亲密经验不计为星级。',
  'pet-recruit': '累计普通精灵招募次数，引导首抽不计入。',
  'awake-1-1': '完整探索碧绿草原难度1；四个宝箱各5分、怪物80分，全部完成为100分。',
  'login-days': '按北京时间累计登录日期，同日重连不重复计数。',
  collection: '按已获得的不同角色种类计数，回收后保留图鉴记录。',
  'copy-stages': '按已通关的不同主线关卡计数，同一关卡不重复。',
  'formation-count': '统计上阵且仍在背包中的不同角色，不计空位和重复角色。',
  're-os-growth': '累计登录、角色等级和主线通关数三项须同时达成。',
  warrior_training: '历史培养条件，仅保留已有配置，不支持新建。'
};
const unavailableExplorationKinds = new Set([
  'awake-1-2', 'awake-1-3', 'awake-2-1', 'awake-2-2', 'awake-2-3',
  'awake-3-1', 'awake-3-2', 'awake-3-3', 'awake-4-1', 'awake-4-2',
  'awake-4-3', 'awake-5-1', 'awake-5-2', 'awake-5-3'
]);
function conditionNote(row?: { id?: string; kind?: string; notes?: string }) {
  if (!row) return '';
  const id = row.kind || row.id || '';
  if (unavailableExplorationKinds.has(id)) return '此地图难度的探索尚未开放，暂不能启用；完整探索目标为100%。';
  return conditionRules[id] || row.notes || '';
}
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
  { title: '说明', key: 'notes', minWidth: 360, render: (row) => conditionNote(row) || '按所选进度条件配置' }
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
  }
}
function edit(row: Achievement | null) {
  selected.value = row;
  draft.value = row ? JSON.parse(JSON.stringify(row)) : fresh();
  if (isOSGrowth.value && !draft.value.requirements) draft.value.requirements = { loginDays: 1, level: 1, stages: 1 };
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
  if (value === 're-os-growth' && !draft.value.requirements) draft.value.requirements = { loginDays: 1, level: 1, stages: 1 };
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
      ? `此条件暂不能启用：${conditionNote(condition.value) || '对应玩法尚未恢复'}`
      : !draft.value.kind
        ? '请选择成就条件'
        : !Number.isInteger(draft.value.target) || draft.value.target < 1
          ? '目标需为正整数'
          : isOSGrowth.value && (!osRequirements.value || [osRequirements.value.loginDays, osRequirements.value.level, osRequirements.value.stages].some(value => !Number.isSafeInteger(value) || value < 1 || value > 2147483647))
            ? '请为OS角色成长填写正整数的登录天数、角色等级和主线关卡数'
          : !draft.value.rewards.length ||
              draft.value.rewards.some((r) => !r.resourceId || !Number.isInteger(r.amount) || r.amount < 1)
            ? '请填写完整的完成奖励'
            : '';
  if (saveError.value) return;
  saving.value = true;
  try {
    await api(
      `/achievements${selected.value ? `/${encodeURIComponent(selected.value.id)}` : ''}`,
      json(selected.value ? 'PUT' : 'POST', { ...draft.value, requirements: isOSGrowth.value ? draft.value.requirements : undefined })
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
          ? [h('div', { class: 'ops-muted mt-5px' }, `暂不可用：${conditionNote(row) || '对应玩法尚未恢复'}`)]
          : [])
      ])
  },
  {
    title: '目标',
    key: 'target',
    minWidth: 250,
    render: goalSummary
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
    <template #title-extra><ContextHelp title="进度记录" text="仅记录已观察到的进度，缺失的历史进度不补计。" /></template>
    <template v-if="!editing" #actions>
      <NButton :loading="loading" @click="load">刷新</NButton>
      <NButton v-if="canWrite" type="primary" @click="edit(null)">新建成就</NButton>
    </template>
    <template #status>修改用于之后首次接收此成就的玩家；已有目标、奖励和领取记录保留。</template>
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
          filterable
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
          <NFormItem label="后台名称" required><NInput v-model:value="draft.name" placeholder="成就名称" /></NFormItem>
        </div>
        <NFormItem label="进度条件" required>
          <template #label><span class="achievement-condition-label">进度条件<ContextHelp v-if="conditionNote(condition) && condition?.supported !== false" title="成就条件说明" :text="conditionNote(condition)" /></span></template>
          <NSelect :value="draft.kind" :options="kindOptions" filterable placeholder="搜索或选择触发条件" @update:value="setKind" />
        </NFormItem>
        <NAlert
          v-if="conditionNote(condition) && condition?.supported === false"
          type="warning"
          :title="conditionNote(condition)"
          class="mb-20px"
        />
        <div v-if="isOSGrowth && draft.requirements" class="achievement-requirements">
          <NFormItem label="累计登录天数" required><NInputNumber v-model:value="draft.requirements.loginDays" :min="1" :max="2147483647" :precision="0"><template #suffix>天</template></NInputNumber></NFormItem>
          <NFormItem label="角色等级" required><NInputNumber v-model:value="draft.requirements.level" :min="1" :max="2147483647" :precision="0"><template #suffix>级</template></NInputNumber></NFormItem>
          <NFormItem label="通关主线关卡数" required><NInputNumber v-model:value="draft.requirements.stages" :min="1" :max="2147483647" :precision="0"><template #suffix>关</template></NInputNumber></NFormItem>
        </div>
        <p v-if="isOSGrowth" class="ops-inline-note mb-16px">以上三项需同时满足；主线按不同已通关关卡计数。</p>
        <NFormItem :label="isOSGrowth ? '需达成条件数' : '达成目标'" required>
          <NInputNumber v-model:value="draft.target" :disabled="fixedTarget !== undefined || saving || !canWrite" :min="1" :max="2147483647" :precision="0"><template v-if="targetUnit(draft.kind)" #suffix>{{ targetUnit(draft.kind) }}</template></NInputNumber>
        </NFormItem>
      </section>
      <section v-show="section === 'rewards'">
        <h3 class="mb-20px">完成奖励</h3>
        <RewardList v-model:value="draft.rewards" :disabled="saving || !canWrite" />
      </section>
      <section v-show="section === 'publish'">
        <h3 class="mb-20px">状态</h3>
        <NFormItem label="启用成就">
          <NSwitch
            v-model:value="draft.enabled"
            :disabled="saving || !canWrite || (condition?.supported === false && !draft.enabled)"
            aria-label="启用成就"
          />
        </NFormItem>
      </section>
    </NForm>
  </ManagementWorkspace>
</template>

<style scoped>
.achievement-condition-label { display: inline-flex; align-items: center; gap: 8px; }
.achievement-requirements { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
.achievement-requirements :deep(.n-input-number) { width: 100%; }
@media (max-width: 650px) { .achievement-requirements { grid-template-columns: minmax(0, 1fr); gap: 0; } }
</style>
