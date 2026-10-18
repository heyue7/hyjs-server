<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NButton, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, errorMessage, json } from '@/service/operations';
import type { ActivityDefinition } from '@/service/activity-schedules';
import { newActivityForm } from '@/service/activity-operations';
import type { ActivityForm, ActivityMode } from '@/service/activity-operations';
import { useOperations } from '@/composables/useOperations';
import ActivityProducts from './activity-products.vue';
import ActivityMilestones from './activity-milestones.vue';
import ActivityPrizes from './activity-prizes.vue';
import ActivityOptions from './activity-options.vue';
import ActivitySummary from './activity-summary.vue';

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
const modeOptions = computed(() => modes.value.map(mode => ({ label: mode.label, value: mode.mode })));
const availableCount = computed(() => definitions.value.filter(row => row.availability.usable).length);
const filtered = computed(() =>
  definitions.value.filter(
    row =>
      row.availability.usable === (availability.value === 'usable') &&
      (!modeFilter.value || row.mode === modeFilter.value) &&
      (!search.value ||
        `${row.name} ${modeMap.value.get(row.mode)?.label || ''}`.toLowerCase().includes(search.value.toLowerCase()))
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
      label: '开服14天原版任务',
      templates: [{ id: 'opening-14-original', name: '原开服任务页面', prefab: '' }],
      conditions: [],
      supportsProducts: false,
      supportsMilestones: false,
      supportsPrizes: false,
      notes: '复用原249项开服任务及奖励。每个角色仍从自己的第1天开始；区服排期只控制入口开放，不重置已有领取记录。'
    }
  ];
}
function openEditor(row?: ActivityDefinition, view = false, copy = false) {
  selected.value = copy ? null : row || null;
  readOnly.value = view || !canWrite.value || Boolean(row && !modeMap.value.has(row.mode));
  if (!row && !modes.value[0]) return;
  form.value = row ? JSON.parse(JSON.stringify(row.form)) : newActivityForm(modes.value[0]);
  if (copy && form.value) form.value.name = `${row!.name}（副本）`;
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
function confirmDiscard(): Promise<boolean> {
  if (!dirty.value) return Promise.resolve(true);
  return new Promise(resolve => {
    if (!window.$dialog) {
      resolve(false);
      return;
    }
    window.$dialog.warning({
      title: '放弃未保存的模板修改？',
      content: '已发布的区服排期不会受到影响。',
      positiveText: '放弃修改',
      negativeText: '继续编辑',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false)
    });
  });
}
async function close() {
  if (!saving.value && (await confirmDiscard())) editorOpen.value = false;
}
async function save() {
  if (!form.value || readOnly.value || saving.value) return;
  if (!form.value.name.trim()) {
    saveError.value = '请填写活动名称';
    activeSection.value = 'basics';
    return;
  }
  saving.value = true;
  saveError.value = '';
  try {
    await api<ActivityDefinition>(
      selected.value ? `/activity-templates/${selected.value.id}` : '/activity-templates',
      json(selected.value ? 'PUT' : 'POST', {
        name: form.value.name.trim(),
        form: { ...form.value, startAt: 0, endAt: 0, claimEndAt: 0 },
        defaultDays: defaultDays.value,
        claimDays: claimDays.value,
        enabled: enabled.value,
        version: selected.value?.version || ''
      })
    );
    window.$message?.success('全局活动设置已保存；已发布排期保持原规则');
    editorOpen.value = false;
    await load();
  } catch (e) {
    saveError.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
const columns = computed<DataTableColumns<ActivityDefinition>>(() => [
  { title: '全局活动', key: 'name', minWidth: 240, render: row => h('strong', row.name) },
  { title: '模式', key: 'mode', width: 155, render: row => modeMap.value.get(row.mode)?.label || '暂未适配' },
  {
    title: '可排期',
    key: 'enabled',
    width: 110,
    render: row =>
      h(NTag, { size: 'small', bordered: false, type: row.enabled ? 'success' : 'default' }, () =>
        row.enabled ? '可使用' : '草稿 / 停用'
      )
  },
  {
    title: availability.value === 'usable' ? '默认持续' : '待补齐内容',
    key: 'duration',
    minWidth: 200,
    render: row =>
      availability.value === 'usable'
        ? `${row.defaultDays}天 · 领奖宽限${row.claimDays}天`
        : row.availability.reason || '尚无完整原消费者'
  },
  {
    title: '操作',
    key: 'actions',
    width: 220,
    render: row =>
      h(NSpace, { size: 6 }, () => [
        h(NButton, { size: 'small', onClick: () => openEditor(row, true) }, () => '查看'),
        canWrite.value && modeMap.value.has(row.mode)
          ? h(NButton, { size: 'small', onClick: () => openEditor(row) }, () => '编辑')
          : null,
        canWrite.value && row.availability.usable
          ? h(NButton, { size: 'small', quaternary: true, onClick: () => openEditor(row, false, true) }, () => '复制')
          : null
      ])
  }
]);
function beforeUnload(event: BeforeUnloadEvent) {
  if (dirty.value || saving.value) {
    event.preventDefault();
    event.returnValue = '';
  }
}
onBeforeRouteLeave(() => (saving.value ? false : confirmDiscard()));
onMounted(() => {
  window.addEventListener('beforeunload', beforeUnload);
  load();
});
onUnmounted(() => window.removeEventListener('beforeunload', beforeUnload));
</script>

<template>
  <div class="ops-page template-workspace">
    <template v-if="!editorOpen">
      <div class="ops-toolbar mb-20px">
        <div>
          <h1>活动设置</h1>
          <p class="ops-muted">全局模板库 · 奖励、资格和原美术资源统一设置；开放日期在区服活动排期中管理。</p>
        </div>
        <NSpace>
          <NButton :loading="loading" @click="load">刷新</NButton>
          <NButton v-if="canWrite" type="primary" :disabled="!modes.length" @click="openEditor()">新建活动模板</NButton>
        </NSpace>
      </div>
      <NAlert v-if="error" type="error" :title="error" class="mb-16px" />
      <NCard :bordered="false">
        <NTabs v-model:value="availability" type="line">
          <NTab name="usable">可用模板 {{ availableCount }}</NTab>
          <NTab name="unavailable">暂不可用 {{ definitions.length - availableCount }}</NTab>
        </NTabs>
        <div class="ops-filters mt-16px">
          <NInput v-model:value="search" clearable placeholder="搜索活动名称或模式" class="ops-search" />
          <NSelect
            v-model:value="modeFilter"
            :options="modeOptions"
            filterable
            clearable
            placeholder="全部模式"
            class="ops-filter-select"
          />
        </div>
        <NDataTable
          :columns="columns"
          :data="filtered"
          :loading="loading"
          :row-key="(row: ActivityDefinition) => row.id"
          :bordered="false"
          :scroll-x="950"
          :pagination="{ pageSize: 20 }"
        />
      </NCard>
    </template>
    <template v-else-if="form">
      <div class="ops-toolbar mb-20px">
        <div>
          <NButton quaternary @click="close">← 返回模板库</NButton>
          <h1>{{ selected?.name || '新建活动模板' }}</h1>
        </div>
        <NTag :bordered="false">全局设置</NTag>
      </div>
      <NAlert v-if="saveError" type="error" :title="saveError" class="mb-16px" />
      <NAlert
        type="info"
        title="模板供所有区服复用。保存仅影响今后新增的排期，已发布和已领取内容保留。"
        class="mb-20px"
      />
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
          <NForm v-if="activeSection !== 'review'" label-placement="top" :disabled="saving || readOnly">
            <template v-if="activeSection === 'basics'">
              <NFormItem label="活动名称" required><NInput v-model:value="form.name" :maxlength="80" /></NFormItem>
              <NFormItem label="活动模式" required>
                <NSelect
                  :value="form.mode"
                  :options="modeOptions"
                  :disabled="Boolean(selected) || readOnly"
                  @update:value="changeMode"
                />
              </NFormItem>
              <NFormItem label="对应原美术资源" required>
                <NSelect
                  v-model:value="form.templateId"
                  :options="selectedMode?.templates.map(item => ({ label: item.name, value: item.id })) || []"
                />
              </NFormItem>
              <div v-if="selectedMode?.templates.find(item => item.id === form?.templateId)?.preview" class="mb-20px">
                <NImage
                  :src="selectedMode.templates.find(item => item.id === form?.templateId)?.preview"
                  :width="240"
                  object-fit="contain"
                />
              </div>
              <NAlert v-if="selectedMode?.notes" type="info" :title="selectedMode.notes" class="mb-20px" />
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
                <span class="ops-muted ml-12px">关闭后保留草稿；现有区服排期不追改</span>
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
                :disabled="saving || readOnly"
              />
              <ActivityMilestones
                v-else-if="selectedMode?.supportsMilestones"
                v-model="form.milestones"
                :disabled="saving || readOnly"
                :single="selectedMode.maxMilestones === 1"
                :max="selectedMode.maxMilestones"
                :day-based="['login-rewards', 'limited-sign'].includes(form.mode)"
                :choice="form.mode === 'login-rewards'"
              />
              <ActivityPrizes
                v-else-if="selectedMode?.supportsPrizes"
                v-model="form.prizes"
                :disabled="saving || readOnly"
              />
              <NAlert
                v-else-if="form.mode === 'opening-14'"
                type="info"
                title="沿用原249项开服任务及原奖励、前7天与14天阶段，不生成新的奖励获取渠道。"
              />
              <ActivityOptions
                v-else
                v-model="form.options"
                :mode="form.mode"
                :fixed-values="selectedMode?.fixedValues"
                :disabled="saving || readOnly"
              />
            </template>
            <template v-if="activeSection === 'settings'">
              <NAlert
                v-if="form.mode === 'opening-14'"
                type="info"
                title="每角色从自己的第1天开始，服排期控制入口开放。重复排期开服任务不会重置已领取记录。"
              />
              <ActivityOptions
                v-else
                v-model="form.options"
                :mode="form.mode"
                :fixed-values="selectedMode?.fixedValues"
                :disabled="saving || readOnly"
              />
            </template>
          </NForm>
          <ActivitySummary v-else :form="form" :mode="selectedMode" hide-schedule />
        </NCard>
      </div>
      <div class="template-footer">
        <span class="ops-muted">{{ dirty ? '有未保存的修改' : '全局模板 · 日期独立在各服排期设置' }}</span>
        <NSpace>
          <NButton :disabled="saving" @click="close">返回</NButton>
          <NButton v-if="!readOnly" type="primary" :loading="saving" @click="save">保存活动设置</NButton>
        </NSpace>
      </div>
    </template>
  </div>
</template>

<style scoped>
h1 {
  margin: 4px 0 8px;
  font-size: 22px;
  font-weight: 600;
}
.template-editor {
  display: grid;
  grid-template-columns: 160px minmax(0, 1fr);
  gap: 24px;
}
.template-editor nav {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.template-editor nav button {
  border: 0;
  background: transparent;
  padding: 14px;
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
