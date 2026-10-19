<script setup lang="ts">
import ContextHelp from './context-help.vue';
import { computed, h, nextTick, onMounted, ref, watch } from 'vue';
import { onBeforeRouteLeave, useRoute, useRouter } from 'vue-router';
import { NButton, NDropdown, NImage, NMenu, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns, MenuOption, SelectOption } from 'naive-ui';
import { api, errorMessage, formatTime, json } from '@/service/operations';
import { deleteActivityInstance, newActivityForm } from '@/service/activity-operations';
import type { ActivityForm, ActivityInstance, ActivityMode } from '@/service/activity-operations';
import { useOperations } from '@/composables/useOperations';
import ActivityProducts from '@/components/operations/activity-products.vue';
import ActivityMilestones from '@/components/operations/activity-milestones.vue';
import KingPrizes from '@/components/operations/king-prizes.vue';
import ActivityPrizes from '@/components/operations/activity-prizes.vue';
import ActivityOptions from '@/components/operations/activity-options.vue';
import ActivitySummary from '@/components/operations/activity-summary.vue';
const { canWrite, loading, error, run } = useOperations();
const router = useRouter();
const route = useRoute();
const rows = ref<ActivityInstance[]>([]);
const modes = ref<ActivityMode[]>([]);
const q = ref('');
const modeFilter = ref<string | null>(null);
const stateFilter = ref<string | null>(null);
const modeMap = computed(() => new Map(modes.value.map(mode => [mode.mode, mode])));
const modeOptions = computed(() => modes.value.map(mode => ({ label: mode.label, value: mode.mode })));
const listPage = ref(1);
const listHeading = ref<HTMLElement | null>(null);
const editorHeading = ref<HTMLElement | null>(null);
const availabilitySection = computed(() => route.query.availability === 'unavailable' ? 'unavailable' : 'usable');
const unavailableOnly = computed(() => availabilitySection.value === 'unavailable');
const availabilityMenu = computed<MenuOption[]>(() => [
  { key: 'usable', label: `可用活动（${rows.value.filter(row => row.availability?.usable).length}）` },
  { key: 'unavailable', label: `暂不可用（${rows.value.filter(row => !row.availability?.usable).length}）` }
]);
const scopedRows = computed(() => rows.value.filter(row => Boolean(row.availability?.usable) !== unavailableOnly.value));
async function selectAvailability(key: string) {
  await router.replace({ query: { ...route.query, availability: key === 'unavailable' ? 'unavailable' : undefined } });
}
watch(availabilitySection, () => {
  listPage.value = 1;
  stateFilter.value = null;
});
const stateOptions = [
  { label: '已开启', value: 'on' },
  { label: '已关闭', value: 'off' },
  { label: '已冻结', value: 'frozen' }
];
const filtered = computed(() =>
  scopedRows.value.filter(row => {
    if (
      q.value &&
      !`${row.name} ${row.id} ${modeMap.value.get(row.mode)?.label || row.mode}`
        .toLowerCase()
        .includes(q.value.toLowerCase())
    )
      return false;
    if (modeFilter.value && row.mode !== modeFilter.value) return false;
    if (stateFilter.value === 'on' && !row.enabled) return false;
    if (stateFilter.value === 'off' && row.enabled) return false;
    if (stateFilter.value === 'frozen' && !row.frozen) return false;
    return true;
  })
);
async function load() {
  const result = await run(() =>
    Promise.all([
      api<{ items: ActivityInstance[] }>('/activity-instances'),
      api<{ items: ActivityMode[] }>('/activity-modes')
    ])
  );
  if (result) {
    rows.value = result[0].items;
    modes.value = result[1].items;
  }
}
const open = ref(false);
const saving = ref(false);
const deletingId = ref('');
const busy = computed(() => saving.value || Boolean(deletingId.value));
const saveError = ref('');
const selected = ref<ActivityInstance | null>(null);
const editorKind = ref<'create' | 'edit' | 'view' | 'copy'>('create');
const form = ref<ActivityForm | null>(null);
const activeTab = ref('basics');
const baseline = ref('');
const dirty = computed(() => editable.value && JSON.stringify(form.value) !== baseline.value);
const sectionOptions = computed(() => [
  { key: 'basics', title: '基本信息', detail: '模式、名称与游戏模板' },
  { key: 'content', title: '奖励内容', detail: '玩家获得什么、如何领取' },
  { key: 'schedule', title: '活动时间', detail: '开放时间与领奖期限' },
  ...(showSettings.value ? [{ key: 'settings', title: '规则设置', detail: '排名、折扣与额外规则' }] : []),
  { key: 'review', title: '检查配置', detail: '按玩家看到的内容核对' }
]);
const currentSection = computed(() => sectionOptions.value.find(section => section.key === activeTab.value));
const chosenTemplate = computed(() => selectedMode.value?.templates.find(template => template.id === form.value?.templateId));
function leave() { open.value = false; nextTick(() => listHeading.value?.scrollIntoView({ block: 'start' })); }
function requestClose() {
  if (busy.value) return;
  if (!dirty.value) { leave(); return; }
  window.$dialog?.warning({ title: '放弃未保存的修改？', content: '返回列表后，本次修改不会保存。', positiveText: '放弃修改', negativeText: '继续编辑', onPositiveClick: leave });
}
const selectedMode = computed(() => (form.value ? modeMap.value.get(form.value.mode) : undefined));
const editable = computed(
  () =>
    canWrite.value && editorKind.value !== 'view' && (editorKind.value !== 'edit' || Boolean(selected.value?.editable))
);
const editorTitle = computed(() => {
  if (editorKind.value === 'create') return '新建活动';
  if (editorKind.value === 'copy') return '复制为新活动';
  return selected.value?.name || '活动详情';
});
const templateOptions = computed(
  () =>
    selectedMode.value?.templates.map(template => ({
      label: template.name,
      value: template.id,
      preview: template.preview
    })) || []
);
const vipOptions = computed(
  () => selectedMode.value?.conditions.find(condition => condition.key === 'minVip')?.options || []
);
const showSettings = computed(() =>
  Boolean(
    form.value &&
    (['turntable', 'mystic-shop', 'continued-spend'].includes(form.value.mode) || Array.isArray(form.value.options.discounts))
  )
);
function show(row?: ActivityInstance, copy = false, view = false) {
  if (busy.value) return;
  if (copy && !row?.availability?.usable) return;
  selected.value = row || null;
  saveError.value = '';
  activeTab.value = 'basics';
  editorKind.value = 'create';
  if (row) {
    form.value = JSON.parse(JSON.stringify(row.form));
    editorKind.value = copy ? 'copy' : 'view';
    if (!copy && !view && canWrite.value && row.editable) editorKind.value = 'edit';
    if (copy) form.value!.name = `${row.name}（副本）`;
  } else {
    const first = modes.value[0];
    if (!first) {
      window.$message?.error('暂未取得支持的活动模式，请刷新后重试');
      return;
    }
    form.value = newActivityForm(first);
  }
  baseline.value = JSON.stringify(form.value);
  open.value = true;
  nextTick(() => editorHeading.value?.scrollIntoView({ block: 'start' }));
}
function changeMode(mode: string) {
  const definition = modeMap.value.get(mode);
  if (!definition) return;
  form.value = newActivityForm(definition);
}
function renderTemplate(option: SelectOption) {
  const template = option as SelectOption & { preview?: string };
  const preview = template.preview && /^(\/admin\/|https:\/\/)/.test(template.preview) ? template.preview : '';
  return h('div', { class: 'activity-template-option' }, [
    preview ? h(NImage, { src: preview, width: 64, height: 40, previewDisabled: true }) : null,
    h('span', String(template.label))
  ]);
}
function validateForm() {
  if (!form.value) return '尚未选择活动';
  if (!form.value.mode || !form.value.templateId) return '请选择活动模式和对应界面模板';
  if (!form.value.name.trim()) { activeTab.value = 'basics'; return '请填写活动名称'; }
  if (!form.value.startAt || !form.value.endAt || !form.value.claimEndAt) {
    activeTab.value = 'schedule';
    return '请设置开始、结束和领奖截止时间';
  }
  if (form.value.products.some(product => !product.cost.resourceId && product.cost.amount > 0)) {
    activeTab.value = 'content';
    return '请选择消耗资源并填写数量';
  }
  if (form.value.startAt && form.value.endAt && form.value.endAt <= form.value.startAt) { activeTab.value = 'schedule'; return '结束时间需要晚于开始时间'; }
  if (form.value.endAt && form.value.claimEndAt && form.value.claimEndAt < form.value.endAt) { activeTab.value = 'schedule'; return '领奖截止不能早于结束时间'; }
  return '';
}
async function save() {
  if (!editable.value || !form.value || busy.value) return;
  saveError.value = validateForm();
  if (saveError.value) return;
  saving.value = true;
  try {
    const updating = editorKind.value === 'edit' && selected.value;
    const saved = await api<ActivityInstance>(
      updating ? `/activity-instances/${encodeURIComponent(selected.value!.id)}` : '/activity-instances',
      json(updating ? 'PUT' : 'POST', {
        ...form.value,
        enabled: updating ? selected.value!.enabled : false,
        makeCurrent: false,
        ...(updating ? { version: selected.value!.version } : {})
      })
    );
    window.$message?.success(updating ? '活动配置已保存' : '活动草稿已创建');
    baseline.value = JSON.stringify(form.value);
    open.value = false;
    await load();
    await selectAvailability(saved.availability?.usable ? 'usable' : 'unavailable');
  } catch (e) {
    saveError.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
async function changeState(row: ActivityInstance, enabled: boolean, makeCurrent = false) {
  if (busy.value) return;
  if (enabled && !row.availability?.usable) return;
  saving.value = true;
  try {
    await api(
      `/activity-instances/${encodeURIComponent(row.id)}/state`,
      json('PUT', { version: row.version, enabled, makeCurrent })
    );
    window.$message?.success(enabled ? '活动已开启' : '活动已关闭');
    await load();
  } catch (e) {
    window.$message?.error(errorMessage(e));
  } finally {
    saving.value = false;
  }
}
function requestDelete(row: ActivityInstance) {
  if (!canWrite.value || busy.value || !window.$dialog) return;
  deletingId.value = row.id;
  let submitting = false;
  const dialog = window.$dialog.warning({
    title: `删除活动“${row.name}”？`,
    content: `此活动将停用并从列表移除，原参与进度和领奖记录保留；已发布的排期快照不变。${open.value && selected.value?.id === row.id && dirty.value ? '本次未保存的修改也将丢弃。' : ''}`,
    positiveText: '删除活动',
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
        await deleteActivityInstance(row.id, row.version);
        rows.value = rows.value.filter(item => item.id !== row.id);
        if (open.value && selected.value?.id === row.id) {
          baseline.value = JSON.stringify(form.value);
          leave();
        }
        window.$message?.success('活动已删除，参与和领奖记录已保留');
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
function activityActions(row: ActivityInstance) {
  if (!canWrite.value || (!row.availability?.usable && !row.editable)) return [];
  const options = [];
  if (row.availability?.usable) {
    options.push({ key: 'copy', label: '复制为新一期' });
    options.push({ key: 'state', label: row.enabled ? '关闭活动' : '开启活动' });
    if (row.mode === 'turntable') options.push({ key: 'current', label: '设为当前转盘' });
  } else if (row.enabled) options.push({ key: 'state', label: '关闭活动' });
  return options;
}
function selectAction(key: string, row: ActivityInstance) {
  if (busy.value) return;
  if (key === 'copy') { show(row, true); return; }
  const label = key === 'current' ? '设为当前转盘' : row.enabled ? '关闭活动' : '开启活动';
  const content = key === 'current' ? '启用此转盘供后续活动期使用，已有活动期和领奖历史保持。' : `${label}“${row.name}”？`;
  window.$dialog?.warning({ title: label, content, positiveText: '确认', negativeText: '取消', onPositiveClick: () => changeState(row, key === 'current' || !row.enabled, key === 'current') });
}
const columns = computed<DataTableColumns<ActivityInstance>>(() => [
  { title: '活动', key: 'name', minWidth: 250, render: row => h('strong', row.name) },
  { title: '模式', key: 'mode', width: 135, render: row => modeMap.value.get(row.mode)?.label || '暂未支持' },
  { title: unavailableOnly.value ? '不可用原因' : '状态', key: 'enabled', minWidth: unavailableOnly.value ? 230 : 150,
    render: row => unavailableOnly.value ? h('div', [h('div', row.availability?.reason || '暂不可运营'), h('span', { class: 'ops-muted' }, row.editable ? '可补齐配置' : '仅供查看')]) : h(NSpace, { size: 6 }, () => [h(NTag, { type: row.enabled ? 'success' : 'default', size: 'small', bordered: false }, () => row.enabled ? '已开启' : '已关闭'), row.frozen ? h(NTag, { size: 'small', bordered: false }, () => '已冻结') : null]) },
  { title: '结束时间', key: 'endAt', width: 185, render: row => formatTime(row.form.endAt) },
  { title: '操作', key: 'action', width: 310, render: row => h(NSpace, { size: 6 }, () => [
    h(NButton, { size: 'small', disabled: busy.value, secondary: true, onClick: () => show(row, false, true) }, () => '查看'),
    canWrite.value && row.editable ? h(NButton, { size: 'small', disabled: busy.value, onClick: () => show(row) }, () => row.availability?.usable ? '编辑' : '补齐配置') : null,
    activityActions(row).length ? h(NDropdown, { trigger: 'click', options: activityActions(row), onSelect: (key: string) => selectAction(key, row) }, { default: () => h(NButton, { size: 'small', disabled: busy.value, quaternary: true }, () => '更多') }) : null,
    canWrite.value ? h(NButton, { size: 'small', quaternary: true, type: 'error', disabled: busy.value, loading: deletingId.value === row.id, onClick: () => requestDelete(row) }, () => '删除') : null
  ]) }
]);
onBeforeRouteLeave(() => {
  if (busy.value) return false;
  if (!open.value || !dirty.value) return true;
  return new Promise<boolean>(resolve => {
    if (!window.$dialog) { resolve(false); return; }
    window.$dialog.warning({ title: '离开前放弃修改？', content: '此活动有未保存的修改。', positiveText: '放弃并离开', negativeText: '继续编辑', onPositiveClick: () => resolve(true), onNegativeClick: () => resolve(false), onClose: () => resolve(false), onMaskClick: () => resolve(false) });
  });
});
onMounted(load);
</script>

<template>
  <div class="ops-page activity-workspace">
    <div v-show="!open" class="activity-list">
      <div ref="listHeading" class="ops-toolbar mb-20px">
        <div class="ops-page-label">活动配置<span class="ops-count">{{ rows.length }}</span></div>
        <NSpace><NButton :loading="loading" :disabled="busy" @click="load">刷新</NButton><NButton v-if="canWrite" type="primary" :disabled="busy || !modes.length" @click="show()">新建活动</NButton></NSpace>
      </div>
      <NAlert v-if="error" type="error" :title="error" class="mb-18px" />
      <NCard :bordered="false" class="ops-panel">
        <NMenu mode="horizontal" :value="availabilitySection" :options="availabilityMenu" class="mb-18px" aria-label="活动可用性分类" @update:value="selectAvailability" />

        <div class="ops-filters">
          <NInput v-model:value="q" clearable placeholder="活动名称或 ID" class="ops-search" @update:value="listPage = 1" />
          <NSelect v-model:value="modeFilter" :options="modeOptions" clearable filterable placeholder="全部活动模式" class="ops-filter-select" @update:value="listPage = 1" />
          <NSelect v-if="!unavailableOnly" v-model:value="stateFilter" :options="stateOptions" clearable placeholder="全部状态" class="ops-filter-select" @update:value="listPage = 1" />
          <span v-if="q || modeFilter || stateFilter" class="ops-muted">找到 {{ filtered.length }} 项</span>
        </div>
        <NDataTable :columns="columns" :data="filtered" :loading="loading" :row-key="(row: ActivityInstance) => row.id" :scroll-x="1215" :bordered="false" :pagination="{ page: listPage, pageSize: 20, onChange: (value: number) => listPage = value }" />
      </NCard>
    </div>
    <template v-if="open && form">
      <div ref="editorHeading" class="activity-editor-header">
        <NButton quaternary :disabled="busy" @click="requestClose">← 返回活动列表</NButton>
        <div class="activity-editor-title"><h1>{{ editorTitle }}</h1><NSpace :size="8"><NTag size="small" :bordered="false">{{ editorKind === 'create' || editorKind === 'copy' ? '新草稿' : selected?.enabled ? '已开启' : '已关闭' }}</NTag><NTag v-if="selected?.frozen && editorKind !== 'copy'" size="small" :bordered="false">已冻结</NTag><NTag v-if="!editable" size="small" :bordered="false">只读查看</NTag></NSpace></div>

      </div>
      <NAlert v-if="saveError" type="error" :title="saveError" />
      <NAlert v-if="selected && !selected.availability?.usable" type="warning" :title="selected.availability?.reason || '此活动暂不可运营'">
        <ul v-if="selected.availability?.missing?.length" class="activity-missing"><li v-for="reason in selected.availability.missing" :key="reason">{{ reason }}</li></ul>
      </NAlert>
      <NAlert v-if="selected?.frozen && editorKind !== 'copy'" type="info" title="已冻结的规则保留历史，可复制为新一期再调整。" />
      <div class="activity-editor-layout" :class="{ 'activity-view-layout': !editable }">
        <nav v-if="editable" class="activity-sections" aria-label="活动编辑分区">
          <button v-for="section in sectionOptions" :key="section.key" type="button" :class="{ selected: activeTab === section.key }" :aria-current="activeTab === section.key ? 'step' : undefined" @click="activeTab = section.key">
            <strong>{{ section.title }}</strong>
          </button>
        </nav>
        <NCard :bordered="false" class="ops-panel activity-editor-body">
          <ActivitySummary v-if="!editable || activeTab === 'review'" :form="form" :mode="selectedMode" />
          <template v-else>
            <div class="activity-section-title"><h2>{{ currentSection?.title }}</h2><p>{{ currentSection?.detail }}</p></div>
            <NForm label-placement="top" :disabled="busy">
              <template v-if="activeTab === 'basics'">
                <NFormItem label="活动名称" required><NInput v-model:value="form.name" :maxlength="80" placeholder="在游戏中显示的活动名称" :disabled="busy" /></NFormItem>
                <NFormItem required><template #label><span class="ops-field-label">活动模式<ContextHelp v-if="selectedMode?.notes" title="活动模式说明" :text="selectedMode.notes" /></span></template><NSelect :value="form.mode" :options="modeOptions" :disabled="busy || editorKind === 'edit'" @update:value="changeMode" /></NFormItem>
                <NFormItem label="游戏界面模板" required><NSelect v-model:value="form.templateId" :options="templateOptions" :render-label="renderTemplate" :disabled="busy" /></NFormItem>
                <div v-if="chosenTemplate?.preview" class="activity-template-preview"><NImage :src="chosenTemplate.preview" :width="180" :height="110" object-fit="contain" /><span class="ops-muted">界面预览</span></div>
              </template>
              <template v-if="activeTab === 'content'">
                <ActivityProducts v-if="selectedMode?.supportsProducts" v-model="form.products" :vip-options="vipOptions" :disabled="busy" />
                <ActivityMilestones v-if="selectedMode?.supportsMilestones" v-model="form.milestones" :mode="form.mode" :disabled="busy" :single="selectedMode?.maxMilestones === 1" :max="selectedMode?.maxMilestones" :day-based="['login-rewards', 'limited-sign'].includes(form.mode)" :choice="form.mode === 'login-rewards'" />
                <KingPrizes v-if="form.mode === 'king-recruit' && selectedMode?.supportsPrizes" v-model="form.prizes" :disabled="busy" />
                <ActivityPrizes v-else-if="selectedMode?.supportsPrizes" v-model="form.prizes" :disabled="busy" />
                <ActivityOptions v-if="!selectedMode?.supportsProducts && !selectedMode?.supportsMilestones && !selectedMode?.supportsPrizes" v-model="form.options" :mode="form.mode" :fixed-values="selectedMode?.fixedValues" :disabled="busy" />
              </template>
              <template v-if="activeTab === 'schedule'">
                <NFormItem label="开始时间" required><NDatePicker :value="form.startAt || null" type="datetime" class="w-full" :disabled="busy" @update:value="form.startAt = $event || 0" /></NFormItem>
                <NFormItem label="结束时间" required><NDatePicker :value="form.endAt || null" type="datetime" class="w-full" :disabled="busy" @update:value="form.endAt = $event || 0" /></NFormItem>
                <NFormItem label="领奖截止时间" required><NDatePicker :value="form.claimEndAt || null" type="datetime" class="w-full" :disabled="busy" @update:value="form.claimEndAt = $event || 0" /></NFormItem>
                <p class="ops-muted mb-24px">时间采用北京时间，领奖截止不能早于活动结束。</p>
                <NFormItem label="运营备注"><NInput v-model:value="form.source" type="textarea" :maxlength="255" :disabled="busy" /></NFormItem>
              </template>
              <ActivityOptions v-if="activeTab === 'settings'" v-model="form.options" :mode="form.mode" :fixed-values="selectedMode?.fixedValues" :disabled="busy" />
            </NForm>
          </template>
        </NCard>

      </div>
      <div class="activity-editor-footer">
        <span class="ops-muted">{{ dirty ? '有未保存的修改' : editable ? editorKind === 'create' || editorKind === 'copy' ? '草稿尚未创建' : '当前配置已保存' : '只读' }}</span>
        <NSpace><NButton v-if="canWrite && selected && (editorKind === 'edit' || editorKind === 'view')" quaternary type="error" :disabled="busy" :loading="deletingId === selected.id" @click="requestDelete(selected)">删除活动</NButton><NButton :disabled="busy" @click="requestClose">{{ editable ? '取消并返回' : '返回列表' }}</NButton><NButton v-if="editable && activeTab !== 'review'" :disabled="busy" @click="activeTab = 'review'">检查配置</NButton><NButton v-if="editable" type="primary" :loading="saving" :disabled="busy" @click="save">{{ editorKind === 'edit' ? '保存配置' : '保存草稿' }}</NButton><NButton v-else-if="canWrite && selected?.availability?.usable" type="primary" :disabled="busy" @click="show(selected, true)">复制为新一期</NButton></NSpace>
      </div>
    </template>
  </div>
</template>

<style scoped>
.activity-editor-header { scroll-margin-top: 80px; }
.activity-editor-header > .n-button { margin-left: -12px; }
.activity-editor-title { display: flex; gap: 20px; align-items: center; margin: 12px 0 6px; }
.activity-editor-title h1 { margin: 0; font-size: 24px; font-weight: 600; line-height: 1.4; }
.activity-editor-layout { display: grid; grid-template-columns: 160px minmax(0, 1fr); gap: 24px; align-items: start; }
.activity-view-layout { grid-template-columns: minmax(0, 1fr); }
.activity-sections { display: flex; flex-direction: column; gap: 5px; position: sticky; top: 20px; }
.activity-sections button { display: flex; flex-direction: column; gap: 6px; padding: 13px 14px; border: 0; border-radius: 7px; color: rgb(var(--base-text-color) / .65); background: transparent; appearance: none; text-align: left; cursor: pointer; }
.activity-sections button:hover { background: rgb(var(--base-text-color) / .035); }
.activity-sections button.selected { background: rgb(var(--primary-color) / .08); color: rgb(var(--primary-color)); }
.activity-sections button:focus-visible { outline: 2px solid rgb(var(--primary-color) / .55); outline-offset: 2px; }
.activity-sections strong { font-size: 14px; font-weight: 500; }
.activity-sections span { font-size: 12px; line-height: 1.5; color: rgb(var(--base-text-color) / .46); }
.activity-section-title { margin-bottom: 28px; }
.activity-section-title h2 { margin: 0 0 8px; font-size: 20px; font-weight: 600; }
.activity-section-title p { margin: 0; color: rgb(var(--base-text-color) / .5); font-size: 13px; }
.activity-editor-body { min-width: 0; }
.activity-context { padding: 10px 0 10px 10px; }
.activity-context h3 { margin: 0 0 24px; font-size: 14px; font-weight: 500; }
.activity-context dl { margin: 0; font-size: 13px; }
.activity-context dt { margin: 20px 0 6px; color: rgb(var(--base-text-color) / .5); }
.activity-context dd { margin: 0; line-height: 1.7; overflow-wrap: anywhere; }
.activity-context p { margin: 28px 0 0; padding-top: 20px; border-top: 1px solid rgb(var(--base-text-color) / .09); font-size: 12px; line-height: 1.8; color: rgb(var(--base-text-color) / .5); }
.activity-editor-footer { position: sticky; bottom: 0; display: flex; justify-content: space-between; align-items: center; gap: 18px; padding: 17px 22px; background: rgb(var(--container-bg-color)); border-radius: 9px; border-top: 1px solid rgb(var(--base-text-color) / .08); z-index: 4; }
.activity-template-preview { display: flex; align-items: center; gap: 20px; padding: 18px; border-radius: 8px; background: rgb(var(--base-text-color) / .025); }
.activity-missing { margin: 8px 0 0; padding-left: 18px; }
@media (max-width: 1300px) { .activity-editor-layout { grid-template-columns: 165px minmax(0, 1fr); } .activity-view-layout { grid-template-columns: minmax(0, 1fr); } .activity-context { display: none; } }
@media (max-width: 760px) { .activity-editor-title { align-items: flex-start; flex-direction: column; gap: 10px; } .activity-editor-title h1 { font-size: 21px; } .activity-editor-layout { display: block; } .activity-sections { flex-direction: row; position: static; overflow-x: auto; margin-bottom: 18px; } .activity-sections button { white-space: nowrap; padding: 10px 13px; } .activity-sections span { display: none; } .activity-editor-footer { padding: 14px; flex-wrap: wrap; } .activity-editor-footer > .ops-muted { width: 100%; } .activity-template-preview { flex-direction: column; align-items: flex-start; } }
</style>
