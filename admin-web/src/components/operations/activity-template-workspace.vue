<script setup lang="ts">
import ContextHelp from './context-help.vue';
import { computed, h, onMounted, onUnmounted, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NButton, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns, DialogReactive } from 'naive-ui';
import { api, errorMessage, json } from '@/service/operations';
import type { ActivityDefinition } from '@/service/activity-schedules';
import { activityCopyAlias, activityFormName, activityRepeatLabel, activityRepeatPolicy, deleteActivityTemplate, newActivityForm, normalizeActivityForm } from '@/service/activity-operations';
import type { ActivityForm, ActivityMode } from '@/service/activity-operations';
import { useOperations } from '@/composables/useOperations';
import ActivityProducts from './activity-products.vue';
import ActivityMilestones from './activity-milestones.vue';
import ActivityPrizes from './activity-prizes.vue';
import KingPrizes from './king-prizes.vue';
import ActivityOptions from './activity-options.vue';
import ActivitySummary from './activity-summary.vue';

const props = defineProps<{ inline?: boolean }>();
const emit = defineEmits<{ saved: [definition: ActivityDefinition]; deleted: [id: string]; closed: [] }>();
const { canWrite, loading, error, run } = useOperations();
const definitions = ref<ActivityDefinition[]>([]);
const modes = ref<ActivityMode[]>([]);
const search = ref('');
const modeFilter = ref<string | null>(null);
const availability = ref('usable');
const selected = ref<ActivityDefinition | null>(null);
const editorOpen = ref(false);
const readOnly = ref(false);
const form = ref<ActivityForm | null>(null);
const defaultDays = ref(14);
const claimDays = ref(0);
const enabled = ref(false);
const activeSection = ref('basics');
const saving = ref(false);
const deletingId = ref('');
const busy = computed(() => saving.value || Boolean(deletingId.value));
const saveError = ref('');
const baseline = ref('');
const draft = computed(() => ({
  form: form.value,
  defaultDays: defaultDays.value,
  claimDays: claimDays.value,
  enabled: enabled.value
}));
const dirty = computed(() => editorOpen.value && !readOnly.value && JSON.stringify(draft.value) !== baseline.value);
const modeMap = computed(() => new Map(modes.value.map(mode => [mode.mode, mode])));
const selectedMode = computed(() => modeMap.value.get(form.value?.mode || ''));
const editorName = computed(() => (form.value ? activityFormName(form.value, selectedMode.value) : ''));
const modeOptions = computed(() => modes.value.map(mode => ({ label: mode.label, value: mode.mode })));
const repeatOptions = [{ label: '一次性', value: 'once' }, { label: '可重复', value: 'repeat' }];
const availableCount = computed(() => definitions.value.filter(row => row.availability.usable).length);
const availabilityOptions = computed(() => [
  { label: `可使用模板 ${availableCount.value}`, value: 'usable' },
  { label: `暂不可用模板 ${definitions.value.length - availableCount.value}`, value: 'unavailable' }
]);
function definitionName(row: ActivityDefinition): string {
  return row.mode === 'king-recruit' ? '王者招募' : row.name;
}
function definitionAlias(row: ActivityDefinition): string {
  const alias = (row.form?.alias || '').trim();
  if (alias) return alias;
  if (row.mode === 'king-recruit' && row.name.startsWith('王者招募') && row.name !== '王者招募')
    return row.name.slice('王者招募'.length).replace(/^[·\s-]+/, '');
  return '';
}
const filtered = computed(() =>
  definitions.value.filter(
    row =>
      row.availability.usable === (availability.value === 'usable') &&
      (!modeFilter.value || row.mode === modeFilter.value) &&
      (!search.value ||
        `${row.name} ${row.form?.alias || ''} ${modeMap.value.get(row.mode)?.label || ''}`
          .toLowerCase()
          .includes(search.value.toLowerCase()))
  )
);
const vipOptions = computed(
  () => selectedMode.value?.conditions.find(condition => condition.key === 'minVip')?.options || []
);
const sections = [
  { key: 'basics', name: '基本设置' },
  { key: 'content', name: '奖励与资格' },
  { key: 'settings', name: '玩法规则' },
  { key: 'review', name: '检查配置' }
];
async function load() {
  const result = await run(() =>
    Promise.all([
      api<{ items: ActivityDefinition[] }>('/activity-templates'),
      api<{ items: ActivityMode[] }>('/activity-modes')
    ])
  );
  if (!result) return;
  definitions.value = result[0].items;
  modes.value = [
    ...result[1].items,
    {
      mode: 'opening-14',
      label: '开服狂欢',
      templates: [{ id: 'opening-14-original', name: '原开服任务页面', prefab: '' }],
      conditions: [],
      supportsProducts: false,
      supportsMilestones: false,
      supportsPrizes: false,
      notes: '任务按区服开服自然日解锁，角色保留完整配置时长。'
    }
  ];
}
function openEditor(row?: ActivityDefinition, view = false, copy = false) {
  if (busy.value) return;
  selected.value = copy ? null : row || null;
  readOnly.value = view || !canWrite.value || Boolean(row && !modeMap.value.has(row.mode));
  if (!row && !modes.value[0]) return;
  form.value = row ? JSON.parse(JSON.stringify(row.form)) : newActivityForm(modes.value[0]);
  if (form.value) {
    normalizeActivityForm(form.value, modeMap.value.get(form.value.mode));
    form.value.repeatPolicy = activityRepeatPolicy(form.value);
    // 复制只改后台别名，游戏内固定标题保持服务端给出的名称。
    if (copy) form.value.alias = activityCopyAlias(form.value.alias);
  }
  defaultDays.value = row?.defaultDays || 14;
  claimDays.value = row?.claimDays || 0;
  enabled.value = copy ? false : Boolean(row?.enabled);
  activeSection.value = 'basics';
  saveError.value = '';
  editorOpen.value = true;
  baseline.value = JSON.stringify(draft.value);
}
function changeMode(mode: string) {
  const definition = modeMap.value.get(mode);
  if (!definition) return;
  form.value = newActivityForm(definition);
  if (mode === 'opening-14') {
    defaultDays.value = 14;
    claimDays.value = 1;
  }
}
let pendingDiscard: { promise: Promise<boolean>; dispose: () => void } | null = null;
function confirmDiscard(): Promise<boolean> {
  if (busy.value) return Promise.resolve(false);
  if (!dirty.value) return Promise.resolve(true);
  if (pendingDiscard) return pendingDiscard.promise;
  if (!window.$dialog) return Promise.resolve(false);
  let resolveConfirmation!: (accepted: boolean) => void;
  const promise = new Promise<boolean>(resolve => { resolveConfirmation = resolve; });
  let accepted = false;
  let decided = false;
  let settled = false;
  let dialog: DialogReactive;
  const settle = (value: boolean) => {
    if (settled) return;
    settled = true;
    pendingDiscard = null;
    resolveConfirmation(value);
  };
  const dismiss = (value: boolean) => {
    if (!decided) { accepted = value; decided = true; }
    dialog.destroy();
    return false;
  };
  dialog = window.$dialog.warning({
    title: '放弃未保存的模板修改？',
    content: '已发布的区服排期不会受到影响。',
    positiveText: '放弃修改',
    negativeText: '继续编辑',
    onPositiveClick: () => dismiss(true),
    onNegativeClick: () => dismiss(false),
    onClose: () => dismiss(false),
    onMaskClick: () => dismiss(false),
    onEsc: () => dismiss(false),
    // Release the editor/route only after this dialog has left the provider.
    onAfterLeave: () => settle(accepted)
  });
  pendingDiscard = { promise, dispose: () => { dialog.destroy(); settle(false); } };
  return promise;
}
async function close() {
  if (!busy.value && (await confirmDiscard())) {
    editorOpen.value = false;
    emit('closed');
  }
}
async function beforeTabLeave() {
  if (busy.value) return false;
  const hadDraft = dirty.value;
  if (!(await confirmDiscard())) return false;
  if (hadDraft && editorOpen.value) {
    editorOpen.value = false;
    selected.value = null;
    form.value = null;
    baseline.value = '';
    emit('closed');
  }
  return true;
}
async function save() {
  if (!form.value || readOnly.value || busy.value) return;
  const name = activityFormName(form.value, selectedMode.value).trim();
  if (!name) {
    saveError.value = '当前活动模式缺少客户端名称';
    activeSection.value = 'basics';
    return;
  }
  saving.value = true;
  saveError.value = '';
  try {
    // 名称由客户端固定，后台别名单独保存，避免写回时相互覆盖。
    form.value.name = name;
    form.value.alias = (form.value.alias || '').trim();
    const saved = await api<ActivityDefinition>(
      selected.value ? `/activity-templates/${encodeURIComponent(selected.value.id)}` : '/activity-templates',
      json(selected.value ? 'PUT' : 'POST', {
        name,
        form: { ...form.value, startAt: 0, endAt: 0, claimEndAt: 0 },
        defaultDays: defaultDays.value,
        claimDays: claimDays.value,
        enabled: enabled.value,
        version: selected.value?.version || ''
      })
    );
    window.$message?.success('活动设置已保存');
    emit('saved', saved);
    emit('closed');
    editorOpen.value = false;
    await load();
  } catch (e) {
    saveError.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
function requestDelete(row: ActivityDefinition) {
  if (!canWrite.value || busy.value || !window.$dialog) return;
  deletingId.value = row.id;
  let submitting = false;
  const dialog = window.$dialog.warning({
    title: `删除模板“${row.name}”？`,
    content: `删除后将从模板库移除，不能再用于新排期。已发布活动、参与进度和领奖记录保留。${selected.value?.id === row.id && dirty.value ? '本次未保存的修改也将丢弃。' : ''}`,
    positiveText: '删除模板',
    negativeText: '取消',
    positiveButtonProps: { type: 'error' },
    maskClosable: false,
    closeOnEsc: false,
    closable: false,
    onPositiveClick: async () => {
      if (submitting) return false;
      submitting = true;
      dialog.loading = true;
      dialog.negativeButtonProps = { disabled: true };
      try {
        await deleteActivityTemplate(row.id, row.version);
        definitions.value = definitions.value.filter(item => item.id !== row.id);
        if (selected.value?.id === row.id) {
          editorOpen.value = false;
          selected.value = null;
          form.value = null;
          baseline.value = '';
          emit('closed');
        }
        emit('deleted', row.id);
        window.$message?.success('模板已删除，已发布活动保持不变');
        return true;
      } catch (e) {
        window.$message?.error(errorMessage(e));
        return false;
      } finally {
        submitting = false;
        dialog.loading = false;
        dialog.negativeButtonProps = { disabled: false };
      }
    },
    onAfterLeave: () => { deletingId.value = ''; }
  });
}
const columns = computed<DataTableColumns<ActivityDefinition>>(() => [
  {
    title: '活动',
    key: 'name',
    minWidth: 260,
    render: row => {
      const alias = definitionAlias(row);
      return h('div', [
        h('strong', definitionName(row)),
        alias
          ? h('div', { class: 'ops-muted', style: 'margin-top:2px;font-size:12px' }, `后台别名：${alias}`)
          : null
      ]);
    }
  },
  { title: '模式', key: 'mode', width: 155, render: row => modeMap.value.get(row.mode)?.label || '暂未适配' },
  { title: '参与方式', key: 'repeatPolicy', width: 110, render: row => activityRepeatLabel(row.form) },
  {
    title: '可排期',
    key: 'enabled',
    width: 110,
    render: row =>
      h(NTag, { size: 'small', bordered: false, type: row.enabled ? 'success' : 'default' }, () =>
        row.enabled ? '可使用' : '已停用'
      )
  },
  {
    title: availability.value === 'usable' ? '默认持续' : '待补齐内容',
    key: 'duration',
    minWidth: 200,
    render: row =>
      availability.value === 'usable'
        ? `${row.defaultDays}天${row.claimDays ? ` · 另留${row.claimDays}天领奖` : ''}`
        : row.availability.reason || '尚无完整原消费者'
  },
  {
    title: '操作',
    key: 'actions',
    width: 300,
    fixed: 'right',
    render: row =>
      h(NSpace, { size: 6 }, () => [
        h(NButton, { size: 'small', disabled: busy.value, 'aria-label': `查看${definitionName(row)}`, onClick: () => openEditor(row, true) }, () => '查看'),
        canWrite.value && modeMap.value.has(row.mode)
          ? h(NButton, { size: 'small', disabled: busy.value, 'aria-label': `编辑${definitionName(row)}`, onClick: () => openEditor(row) }, () => '编辑')
          : null,
        canWrite.value && row.availability.usable
          ? h(NButton, { size: 'small', disabled: busy.value, quaternary: true, 'aria-label': `复制${definitionName(row)}`, onClick: () => openEditor(row, false, true) }, () => '复制')
          : null,
        canWrite.value
          ? h(NButton, { size: 'small', quaternary: true, type: 'error', disabled: busy.value, loading: deletingId.value === row.id, 'aria-label': `删除${definitionName(row)}`, onClick: () => requestDelete(row) }, () => '删除')
          : null
      ])
  }
]);
function beforeUnload(event: BeforeUnloadEvent) {
  if (dirty.value || busy.value) {
    event.preventDefault();
    event.returnValue = '';
  }
}
async function openFromSchedule(row: ActivityDefinition, view = false) {
  if (!modes.value.length) await load();
  openEditor(row, view);
}
defineExpose({ openFromSchedule, confirmDiscard, beforeTabLeave, saving: busy, dirty });
onBeforeRouteLeave(() => busy.value ? false : props.inline ? true : confirmDiscard());
onMounted(() => {
  window.addEventListener('beforeunload', beforeUnload);
  load();
});
onUnmounted(() => {
  pendingDiscard?.dispose();
  window.removeEventListener('beforeunload', beforeUnload);
});
</script>

<template>
  <div class="ops-page template-workspace" :class="{ 'template-inline': inline }">
    <template v-if="!editorOpen && !inline">
      <NAlert v-if="error" type="error" :title="error" />
      <NCard :bordered="false" class="ops-panel">
        <div class="ops-filters">
          <NInput v-model:value="search" clearable placeholder="搜索活动名称、别名或模式" class="ops-search" aria-label="搜索活动名称、别名或模式" />
          <NSelect
            v-model:value="availability"
            :options="availabilityOptions"
            class="ops-filter-select"
            aria-label="按可用性筛选"
          />
          <NSelect
            v-model:value="modeFilter"
            :options="modeOptions"
            filterable
            clearable
            placeholder="全部模式"
            class="ops-filter-select"
            aria-label="按活动模式筛选"
          />
          <span class="ops-muted template-count">{{ filtered.length }} / {{ definitions.length }} 个</span>
          <NSpace align="center">
            <ContextHelp title="排期设置" text="开放日期和区服在活动排期中设置。" />
            <NButton :loading="loading" :disabled="busy" @click="load">刷新</NButton>
            <NButton v-if="canWrite" type="primary" :disabled="busy || !modes.length" @click="openEditor()">新建活动模板</NButton>
          </NSpace>
        </div>
        <NDataTable
          :columns="columns"
          :data="filtered"
          :loading="loading"
          :row-key="(row: ActivityDefinition) => row.id"
          :bordered="false"
          :scroll-x="1160"
          :pagination="{ pageSize: 20 }"
        >
          <template #empty>
            <NEmpty :description="definitions.length ? '没有符合筛选条件的模板' : '暂无活动模板'" />
          </template>
        </NDataTable>
      </NCard>
    </template>
    <template v-else-if="form">
      <div class="template-editor-actions">
        <NButton v-if="!inline" quaternary :disabled="busy" @click="close">← 返回模板库</NButton>
        <ContextHelp title="生效范围" text="修改用于新排期，已发布活动不变。开放日期和区服在活动排期中设置。" />
        <span v-if="readOnly" class="ops-muted">只读查看</span>
      </div>
      <NAlert v-if="saveError" type="error" :title="saveError" class="mb-16px" />
      <div class="template-editor">
        <nav aria-label="活动模板编辑分区">
          <button
            v-for="section in sections"
            :key="section.key"
            :class="{ active: activeSection === section.key }"
            @click="activeSection = section.key"
          >
            {{ section.name }}
          </button>
        </nav>
        <NCard :bordered="false">
          <NForm v-if="activeSection !== 'review'" label-placement="top" :disabled="busy || readOnly">
            <template v-if="activeSection === 'basics'">
              <NFormItem required>
                <template #label><span class="ops-field-label">活动名称<ContextHelp title="正式名称" text="游戏内名称由客户端固定；需要区分批次请填写后台别名。" /></span></template>
                <NInput :value="editorName" disabled />
              </NFormItem>
              <NFormItem label="后台别名">
                <NInput v-model:value="form.alias" :maxlength="80" clearable placeholder="仅后台列表与排期显示，不改游戏内名称" />
              </NFormItem>
              <NFormItem required>
                <template #label><span class="ops-field-label">活动模式<ContextHelp v-if="selectedMode?.notes" title="活动模式说明" :text="selectedMode.notes" /></span></template>
                <NSelect
                  :value="form.mode"
                  :options="modeOptions"
                  :disabled="Boolean(selected) || readOnly"
                  @update:value="changeMode"
                />
              </NFormItem>
              <NFormItem label="游戏界面模板" required>
                <NSelect
                  v-model:value="form.templateId"
                  :options="selectedMode?.templates.map(item => ({ label: item.name, value: item.id })) || []"
                />
              </NFormItem>
              <NFormItem label="参与方式" required>
                <NSelect v-model:value="form.repeatPolicy" :options="repeatOptions" />
              </NFormItem>
              <p class="template-help">{{ form.repeatPolicy === 'once' ? '每个角色只能参与一期，再次排期不会重新参与。' : '每次新建排期可重新参与，同一期内的修改或停启保留进度。' }}</p>
              <div v-if="selectedMode?.templates.find(item => item.id === form?.templateId)?.preview" class="mb-20px">
                <NImage
                  :src="selectedMode.templates.find(item => item.id === form?.templateId)?.preview"
                  :width="240"
                  object-fit="contain"
                />
              </div>
              <div class="template-durations">
                <NFormItem label="默认活动天数">
                  <NInputNumber v-model:value="defaultDays" :min="1" :max="366" />
                </NFormItem>
                <NFormItem label="额外领奖宽限天数">
                  <NInputNumber v-model:value="claimDays" :min="0" :max="90" />
                </NFormItem>
              </div>
              <NFormItem label="允许新增排期">
                <NSwitch v-model:value="enabled" />
                <span class="ops-muted ml-12px">关闭后不能新增排期，已发布活动不变</span>
              </NFormItem>
              <NFormItem label="运营备注">
                <NInput v-model:value="form.source" type="textarea" :maxlength="1000" />
              </NFormItem>
            </template>
            <template v-if="activeSection === 'content'">
              <ActivityProducts
                v-if="selectedMode?.supportsProducts"
                v-model="form.products"
                :vip-options="vipOptions"
                :disabled="busy || readOnly"
              />
              <ActivityMilestones
                v-else-if="selectedMode?.supportsMilestones"
                v-model="form.milestones"
                :mode="form.mode"
                :disabled="busy || readOnly"
                :single="selectedMode.maxMilestones === 1"
                :max="selectedMode.maxMilestones"
                :day-based="['login-rewards', 'limited-sign'].includes(form.mode)"
                :choice="form.mode === 'login-rewards'"
              />
              <KingPrizes
                v-model:world="form.options.kingWorld"
                v-else-if="form.mode === 'king-recruit' && selectedMode?.supportsPrizes"
                v-model="form.prizes"
                :disabled="busy || readOnly"
              />
              <ActivityPrizes
                v-else-if="selectedMode?.supportsPrizes"
                v-model="form.prizes"
                :disabled="busy || readOnly"
              />
              <NAlert
                v-else-if="form.mode === 'opening-14'"
                type="info"
                title="沿用客户端249项任务与奖励。"
              />
              <ActivityOptions
                v-else
                v-model="form.options"
                :mode="form.mode"
                :fixed-values="selectedMode?.fixedValues"
                :disabled="busy || readOnly"
              />
            </template>
            <template v-if="activeSection === 'settings'">
              <NAlert
                v-if="form.mode === 'opening-14'"
                type="info"
                title="任务按区服开服自然日解锁，有效期按角色起算；每一期独立保留领取记录。"
              />
              <ActivityOptions
                v-else
                v-model="form.options"
                :mode="form.mode"
                :fixed-values="selectedMode?.fixedValues"
                :disabled="busy || readOnly"
              />
            </template>
          </NForm>
          <ActivitySummary v-else :form="form" :mode="selectedMode" hide-schedule />
        </NCard>
      </div>
      <div class="template-footer">
        <span class="ops-muted">{{ dirty ? '未保存' : '开放日期在活动排期中设置' }}</span>
        <NSpace>
          <NButton v-if="canWrite && selected" quaternary type="error" :disabled="busy" :loading="deletingId === selected.id" @click="requestDelete(selected)">删除模板</NButton>
          <NButton :disabled="busy" @click="close">{{ inline ? '取消' : '返回' }}</NButton>
          <NButton v-if="!readOnly" type="primary" :loading="saving" :disabled="busy" @click="save">保存活动设置</NButton>
        </NSpace>
      </div>
    </template>
  </div>
</template>

<style scoped>
.template-inline { padding: 0; }
.template-editor-actions { display: flex; align-items: center; gap: 12px; margin-bottom: 12px; flex-wrap: wrap; }
.template-count { margin-left: auto; font-variant-numeric: tabular-nums; }
.template-help { color: rgb(var(--base-text-color) / .65); font-size: 13px; line-height: 1.7; margin: 0 0 12px; }
.template-editor nav button:hover { background: rgb(var(--base-text-color) / .04); }
.template-editor nav button:focus-visible { outline: 2px solid rgb(var(--primary-color)); outline-offset: 2px; }

.template-editor {
  display: grid;
  grid-template-columns: 160px minmax(0, 1fr);
  gap: 16px;
}
.template-editor nav {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.template-editor nav button {
  border: 0;
  background: transparent;
  appearance: none;
  padding: 12px 14px;
  text-align: left;
  color: inherit;
  cursor: pointer;
  border-radius: 8px;
}
.template-editor nav button.active {
  color: rgb(var(--primary-color));
  background: rgb(var(--primary-color) / 0.08);
}
.template-durations {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 20px;
}
.template-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  position: sticky;
  bottom: 0;
  padding: 18px 20px;
  background: rgb(var(--container-bg-color));
  margin-top: 20px;
  border-top: 1px solid rgb(var(--base-text-color) / 0.08);
  z-index: 3;
}
@media (max-width: 760px) {
  .template-count {
    margin-left: 0;
  }
  .template-editor {
    display: block;
  }
  .template-editor nav {
    flex-direction: row;
    overflow: auto;
    margin-bottom: 16px;
  }
  .template-editor nav button {
    white-space: nowrap;
  }
  .template-durations {
    display: block;
  }
  .template-footer {
    flex-wrap: wrap;
  }
}
</style>
