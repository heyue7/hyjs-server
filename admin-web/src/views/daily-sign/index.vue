<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NButton } from 'naive-ui';
import type { DataTableColumns, DialogReactive } from 'naive-ui';
import ContextHelp from '@/components/operations/context-help.vue';
import RewardList from '@/components/operations/reward-list.vue';
import RewardSummary from '@/components/operations/reward-summary.vue';
import { useOperations } from '@/composables/useOperations';
import { getDailySignRules, saveDailySignRules } from '@/service/daily-sign';
import type { DailySignRow, DailySignRules } from '@/service/daily-sign';
import { ApiError, api, errorMessage } from '@/service/operations';
import type { Item, ItemsResponse } from '@/service/operations';

const props = defineProps<{ embedded?: boolean }>();
const { canWrite } = useOperations();
const currentMonth = new Intl.DateTimeFormat('sv-SE', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit' }).format(new Date());
const selectedMonth = ref(currentMonth);
const snapshot = ref<DailySignRules>();
let loadGeneration = 0;
const pastMonth = computed(() => selectedMonth.value < currentMonth);
const monthLabel = computed(() => selectedMonth.value.replace('-', '年') + '月');
const nextMonth = (() => { const [year, month] = currentMonth.split('-').map(Number); return `${year + (month === 12 ? 1 : 0)}-${String(month === 12 ? 1 : month + 1).padStart(2, '0')}`; })();
const visibleRows = computed(() => rows.value.slice(0, snapshot.value?.daysInMonth || 31));
const rows = ref<DailySignRow[]>([]);
const enabled = ref(false);
const original = ref('');
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const conflict = ref(false);
const resources = ref<Item[]>([]);
const resourceError = ref('');
const resourceLoading = ref(false);
const editorOpen = ref(false);
const editor = ref<DailySignRow>({ day: 1, reward: [], doubleVip: 0 });
const editorOriginal = ref('');
const editorError = ref('');
let alive = true;
let resourceGeneration = 0;
const maxAmount = 2147483647;
const content = () => JSON.stringify({ enabled: enabled.value, rows: rows.value });
const dirty = computed(() => Boolean(snapshot.value) && content() !== original.value);
const editorDirty = computed(() => editorOpen.value && JSON.stringify(editor.value) !== editorOriginal.value);
const disabled = computed(() => !canWrite.value || pastMonth.value || saving.value || loading.value || conflict.value);

// Table rows and drafts are Vue proxies; copy the API fields into plain DTOs.
function cloneRow(row: DailySignRow): DailySignRow {
  return {
    day: row.day,
    doubleVip: row.doubleVip,
    reward: row.reward.map(reward => ({ resourceId: reward.resourceId, amount: reward.amount }))
  };
}

async function loadResources() {
  const generation = ++resourceGeneration;
  const ids = [...new Set(rows.value.flatMap(row => row.reward.map(reward => reward.resourceId)).filter(Boolean))];
  resourceLoading.value = true;
  resourceError.value = '';
  try {
    const data = ids.length
      ? await api<ItemsResponse>(`/items?${new URLSearchParams({ ids: ids.join(','), page: '1', pageSize: '100' })}`)
      : { items: [], total: 0 };
    if (!alive || generation !== resourceGeneration) return;
    resources.value = data.items;
    if (data.items.length < ids.length) resourceError.value = '部分奖励名称或图片尚未载入，请重试。';
  } catch (cause) {
    if (alive && generation === resourceGeneration) resourceError.value = errorMessage(cause);
  } finally {
    if (alive && generation === resourceGeneration) resourceLoading.value = false;
  }
}

function apply(data: DailySignRules) {
  snapshot.value = data;
  selectedMonth.value = data.month;
  rows.value = data.rows.map(cloneRow).sort((a, b) => a.day - b.day);
  enabled.value = data.enabled;
  original.value = content();
  conflict.value = false;
}

async function load() {
  if (loading.value || saving.value) return;
  const generation = ++loadGeneration;
  const month = selectedMonth.value;
  loading.value = true;
  error.value = '';
  try {
    const data = await getDailySignRules(month);
    if (!alive || generation !== loadGeneration || month !== selectedMonth.value) return;
    apply(data);
    await loadResources();
  } catch (cause) {
    if (alive) error.value = errorMessage(cause);
  } finally {
    if (alive && generation === loadGeneration) loading.value = false;
  }
}

let pendingDiscard: { promise: Promise<boolean>; dispose: () => void } | null = null;
function confirmDiscard(title: string): Promise<boolean> {
  if (pendingDiscard) return pendingDiscard.promise;
  if (!window.$dialog) return Promise.resolve(false);
  let resolveConfirmation!: (value: boolean) => void;
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
    title,
    content: '尚未保存的修改将丢失。',
    positiveText: '放弃修改',
    negativeText: '继续编辑',
    onPositiveClick: () => dismiss(true),
    onNegativeClick: () => dismiss(false),
    onEsc: () => dismiss(false),
    onClose: () => dismiss(false),
    onMaskClick: () => dismiss(false),
    onAfterLeave: () => settle(accepted)
  });
  pendingDiscard = { promise, dispose: () => { dialog.destroy(); settle(false); } };
  return promise;
}

async function refresh() {
  if (editorOpen.value || loading.value || saving.value) return;
  if (dirty.value && !await confirmDiscard('重新载入签到配置？')) return;
  await load();
}

async function changeMonth(value: string | null) {
  if (!value || value === selectedMonth.value || loading.value || saving.value || editorOpen.value) return;
  if (dirty.value && !await confirmDiscard('切换签到月份？')) return;
  selectedMonth.value = value;
  snapshot.value = undefined;
  rows.value = [];
  conflict.value = false;
  await load();
}

async function copyCurrentMonth() {
  if (disabled.value || editorOpen.value || selectedMonth.value === currentMonth) return;
  if (dirty.value && !await confirmDiscard('用本月奖历替换当前修改？')) return;
  loading.value = true;
  error.value = '';
  try {
    const data = await getDailySignRules(currentMonth);
    if (!alive) return;
    rows.value = data.rows.map(cloneRow);
    enabled.value = data.enabled;
    await loadResources();
  } catch (cause) { if (alive) error.value = errorMessage(cause); }
  finally { if (alive) loading.value = false; }
}

function validateRow(row: DailySignRow) {
  if (!Number.isInteger(row.day) || row.day < 1 || row.day > 31) return '签到天数需为 1–31。';
  if (row.reward.length !== 1 || !row.reward[0].resourceId) return '每档请选择 1 种签到奖励。';
  const amount = row.reward[0].amount;
  if (!Number.isInteger(amount) || amount < 1 || amount > maxAmount) return '奖励数量需为 1–2,147,483,647 的整数。';
  if (!Number.isInteger(row.doubleVip) || row.doubleVip < 0 || row.doubleVip > 12) return '双倍所需 VIP 需为 0–12。';
  return '';
}

function edit(row: DailySignRow) {
  editor.value = cloneRow(row);
  editorOriginal.value = JSON.stringify(editor.value);
  editorError.value = '';
  editorOpen.value = true;
}

async function closeEditor() {
  if (editorDirty.value && !await confirmDiscard('放弃本档奖励修改？')) return;
  editorOpen.value = false;
}

function applyEditor() {
  if (disabled.value) return;
  editorError.value = validateRow(editor.value);
  if (editorError.value) return;
  rows.value = rows.value.map(row => row.day === editor.value.day ? cloneRow(editor.value) : row);
  editorOpen.value = false;
  loadResources();
}

async function save() {
  if (!snapshot.value || disabled.value || (!dirty.value && snapshot.value.configured) || editorOpen.value) return;
  error.value = '';
  if (rows.value.length !== 31 || new Set(rows.value.map(row => row.day)).size !== 31) {
    error.value = '签到配置需完整保留第 1–31 天，请刷新后重试。';
    return;
  }
  for (const row of rows.value) {
    const reason = validateRow(row);
    if (reason) { error.value = `第 ${row.day} 天：${reason}`; return; }
  }
  saving.value = true;
  try {
    const data = await saveDailySignRules({ month: snapshot.value.month, version: snapshot.value.version, enabled: enabled.value, rows: rows.value.map(cloneRow) });
    if (!alive) return;
    apply(data);
    window.$message?.success(`${monthLabel.value}签到安排已保存`);
    await loadResources();
  } catch (cause) {
    if (!alive) return;
    error.value = errorMessage(cause);
    if (cause instanceof ApiError && cause.status === 409) conflict.value = true;
  } finally {
    if (alive) saving.value = false;
  }
}

const columns = computed<DataTableColumns<DailySignRow>>(() => [
  { title: '累计签到', key: 'day', width: 100, render: row => `第 ${row.day} 天` },
  { title: '签到奖励', key: 'reward', minWidth: 300, render: row => h(RewardSummary, { rewards: row.reward, resources: resources.value }) },
  { title: '双倍所需 VIP', key: 'doubleVip', width: 160, render: row => row.doubleVip === 0 ? '不启用双倍' : `VIP ${row.doubleVip}` },
  { title: '操作', key: 'action', width: 90, fixed: 'right', render: row => h(NButton, { size: 'small', disabled: loading.value || saving.value || conflict.value, onClick: () => edit(row) }, () => canWrite.value && !pastMonth.value ? '编辑' : '查看') }
]);

async function beforeLeave() {
  if (saving.value) return false;
  if (canWrite.value && (dirty.value || editorDirty.value)) {
    if (!await confirmDiscard('离开每日签到配置？')) return false;
    const wasConflict = conflict.value;
    if (snapshot.value) apply(snapshot.value);
    conflict.value = wasConflict;
  }
  editorOpen.value = false;
  return true;
}
onBeforeRouteLeave(() => props.embedded ? true : beforeLeave());
const busy = computed(() => loading.value || saving.value);
defineExpose({ beforeLeave, dirty: computed(() => dirty.value || editorDirty.value), busy });
function beforeUnload(event: BeforeUnloadEvent) {
  if (saving.value || canWrite.value && (dirty.value || editorDirty.value)) {
    event.preventDefault();
    event.returnValue = '';
  }
}
onMounted(() => { load(); window.addEventListener('beforeunload', beforeUnload); });
onBeforeUnmount(() => { pendingDiscard?.dispose(); alive = false; loadGeneration += 1; resourceGeneration += 1; window.removeEventListener('beforeunload', beforeUnload); });
</script>

<template>
  <div class="ops-page daily-sign-page">
    <div class="ops-toolbar">
      <NSpace align="center" :size="8" class="daily-sign-month-picker">
        <NDatePicker type="month" :formatted-value="selectedMonth" value-format="yyyy-MM" format="yyyy年MM月" :clearable="false" :disabled="saving || loading || editorOpen" aria-label="签到月份" @update:formatted-value="changeMonth" />
        <NButton :disabled="saving || loading || editorOpen" @click="changeMonth(currentMonth)">本月</NButton>
        <NButton :disabled="saving || loading || editorOpen" @click="changeMonth(nextMonth)">下个月</NButton>
        <ContextHelp title="签到规则" :text="snapshot?.source || '每月按累计签到天数依次领取，每天限领一档。已领取奖励保留原记录。'" />
        <NTag v-if="dirty && canWrite" type="warning" :bordered="false">未保存</NTag>
        <NTag v-if="!canWrite" :bordered="false">只读</NTag>
        <NButton :loading="loading" :disabled="saving || editorOpen" @click="refresh">刷新</NButton>
      </NSpace>
    </div>
    <NAlert v-if="error" type="error" :title="error" />
    <NAlert v-if="conflict" type="warning" title="配置已被其他操作更新。当前输入已保留，请刷新后重新编辑。" />
    <template v-if="snapshot">
      <div class="daily-sign-month-status">
        <strong>{{ monthLabel }}签到</strong>
        <NTag :type="snapshot.configured ? 'success' : 'default'" :bordered="false">{{ snapshot.configured ? (selectedMonth > currentMonth ? '已安排，到月自动生效' : '已单独配置') : '尚未单独安排，沿用默认奖历' }}</NTag>
        <NTag v-if="pastMonth" :bordered="false">历史月份 · 只读</NTag>
        <NButton v-if="canWrite && selectedMonth > currentMonth" size="small" :disabled="disabled || editorOpen" @click="copyCurrentMonth">复制本月奖历</NButton>
      </div>
      <NCard :bordered="false">
        <div class="daily-sign-settings">
          <div class="daily-sign-enabled">
            <span>开放签到</span>
            <NSwitch v-model:value="enabled" :disabled="disabled" aria-label="开放每日签到" />
            <span class="ops-muted">{{ enabled ? '已开启' : '已关闭' }}</span>
          </div>
          <span class="daily-sign-caption">本月累计签到 · 共 {{ snapshot.daysInMonth }} 档 · 双倍门槛 0 表示不启用</span>
        </div>
        <div v-if="resourceError" class="daily-sign-resource-error">
          <span>{{ resourceError }}</span>
          <NButton text size="small" :loading="resourceLoading" @click="loadResources">重试载入资源</NButton>
        </div>
        <NDataTable :columns="columns" :data="visibleRows" :loading="loading" :bordered="false" :row-key="(row: DailySignRow) => row.day" :scroll-x="700" size="small" />
      </NCard>
      <div class="daily-sign-footer">
        <span class="ops-muted">{{ selectedMonth > currentMonth ? '提前保存后，到月自动切换。' : '修改用于本月后续领取，已领取的奖励不变。' }}</span>
        <NButton v-if="canWrite" type="primary" :loading="saving" :disabled="disabled || (!dirty && snapshot.configured) || editorOpen" @click="save">保存安排</NButton>
      </div>
    </template>
    <NSpin v-else-if="loading" class="daily-sign-loading" />
    <NEmpty v-else description="签到配置尚未载入，请刷新重试" />

    <NModal :show="editorOpen" preset="card" :title="`第 ${editor.day} 天签到奖励`" class="daily-sign-editor" @update:show="!$event && closeEditor()">
      <NAlert v-if="editorError" type="error" :title="editorError" class="mb-16px" />
      <NForm label-placement="top" :disabled="disabled">
        <NFormItem label="签到奖励" required>
          <RewardList v-model:value="editor.reward" :max="1" :disabled="disabled" label="奖励" />
        </NFormItem>
        <NFormItem label="双倍所需 VIP" required>
          <div class="daily-sign-vip">
            <NInputNumber v-model:value="editor.doubleVip" :min="0" :max="12" :precision="0" />
            <span class="ops-muted">0 表示不启用双倍奖励</span>
          </div>
        </NFormItem>
      </NForm>
      <template #footer>
        <div class="ops-drawer-actions">
          <NButton @click="closeEditor">{{ canWrite ? '取消' : '关闭' }}</NButton>
          <NButton v-if="canWrite" type="primary" :disabled="disabled" @click="applyEditor">应用到配置</NButton>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.daily-sign-month-status { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin: 0 0 16px; }
.daily-sign-month-picker :deep(.n-date-picker) { width: 172px; }
.daily-sign-page { max-width: 1200px; }
.daily-sign-enabled { display: flex; align-items: center; gap: 10px; }
.daily-sign-settings { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 14px; margin-bottom: 20px; }
.daily-sign-caption { color: rgb(var(--base-text-color) / .6); font-size: 13px; }
.daily-sign-resource-error { display: flex; align-items: center; gap: 12px; margin-bottom: 16px; color: rgb(var(--base-text-color) / .65); }
.daily-sign-footer { position: sticky; z-index: 20; bottom: 0; display: flex; justify-content: space-between; align-items: center; gap: 16px; margin-top: 16px; padding: 16px 20px; border: 1px solid rgb(var(--base-text-color) / .08); border-radius: 8px; background: rgb(var(--container-bg-color)); }
.daily-sign-loading { display: block; margin: 48px auto; }
:global(.n-card.daily-sign-editor) { width: 720px; max-width: calc(100vw - 32px); }
.daily-sign-editor :deep(.reward-list) { width: 100%; }
.daily-sign-vip { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; }
.daily-sign-vip > .n-input-number { width: 140px; }
@media (max-width: 640px) {
  .daily-sign-page .ops-toolbar, .daily-sign-footer { flex-wrap: wrap; }
  .daily-sign-editor :deep(.reward-list-line) { grid-template-columns: minmax(0, 1fr) auto; }
  .daily-sign-editor :deep(.reward-list-line > :first-child) { grid-column: 1 / -1; }
}
</style>
