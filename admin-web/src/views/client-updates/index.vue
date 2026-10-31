<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import type { Directive } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NButton, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { ApiError, errorMessage, formatTime } from '@/service/operations';
import { actClientUpdate, getClientUpdateHistory, getClientUpdates, saveClientUpdate } from '@/service/client-updates';
import type { ClientRelease, ClientUpdateCommand, ClientUpdateDraft, ClientUpdateOverview, UpdateAction, UpdateMode } from '@/service/client-updates';
import { createRequestId } from '@/utils/request-id';
import VersionSegment from './VersionSegment.vue';

const overview = ref<ClientUpdateOverview | null>(null);
const loading = ref(false); const busy = ref(false); const error = ref(''); const conflict = ref(false);
const history = ref<ClientRelease[]>([]); const total = ref(0); const page = ref(1); const historyError = ref('');
const editor = ref(false); const preview = ref(false); const formError = ref(''); const baseline = ref('');
const editorConflict = ref(false); const recoveryNotice = ref('');
const pending = ref<ClientUpdateCommand | null>(null);
const draft = reactive<ClientUpdateDraft>(emptyDraft());
const versionParts = reactive({ major: '', minor: '', patch: '' });
const unparsedVersion = ref('');
const startInput = ref<HTMLInputElement | null>(null); const endInput = ref<HTMLInputElement | null>(null);
let dateInputEdited = false;
function markDateInputEdited() { dateInputEdited = true; }
type ServerTimeRange = { earliest: number; latest: number };
type ServerClock = { observedAt: number; receivedAt: number; roundTrip: number };
const maintenanceNow = ref<ServerTimeRange | null>(null);
let serverClock: ServerClock | null = null;
const beijingFormatter = new Intl.DateTimeFormat('en-GB', {
  timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit',
  hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23'
});
function beijingParts(time: number): number[] {
  const parts = Object.fromEntries(beijingFormatter.formatToParts(new Date(time)).map(item => [item.type, item.value]));
  return ['year', 'month', 'day', 'hour', 'minute', 'second'].map(key => Number(parts[key]));
}
function wallTime(parts: number[]): number {
  const [year, month, day, hour, minute, second] = parts;
  const date = new Date(0);
  date.setUTCFullYear(year, month - 1, day);
  date.setUTCHours(hour, minute, second, 0);
  return date.getTime();
}
function beijingInputValue(time: number): string {
  if (!time || !Number.isFinite(time)) return '';
  const [year, month, day, hour, minute, second] = beijingParts(time);
  const two = (value: number) => String(value).padStart(2, '0');
  const base = `${String(year).padStart(4, '0')}-${two(month)}-${two(day)}T${two(hour)}:${two(minute)}`;
  return second ? `${base}:${two(second)}` : base;
}
const vBeijingDateValue: Directive<HTMLInputElement, number> = {
  mounted(element, binding) { element.value = beijingInputValue(binding.value); },
  updated(element, binding) {
    if (binding.value !== binding.oldValue && document.activeElement !== element) element.value = beijingInputValue(binding.value);
  }
};
function parseBeijingInput(value: string): number | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/.exec(value);
  if (!match) return null;
  const parts = [Number(match[1]), Number(match[2]), Number(match[3]), Number(match[4]), Number(match[5]), Number(match[6] || 0)];
  const [year, month, day, hour, minute, second] = parts;
  if (!year || month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || second > 59) return null;
  const target = wallTime(parts); const check = new Date(target);
  if (check.getUTCFullYear() !== year || check.getUTCMonth() + 1 !== month || check.getUTCDate() !== day) return null;
  let result = target;
  for (let i = 0; i < 4; i += 1) {
    const delta = target - wallTime(beijingParts(result));
    if (!delta) break;
    result += delta;
  }
  if (result <= 0 || wallTime(beijingParts(result)) !== target) return null;
  if (wallTime(beijingParts(result - 3600000)) === target || wallTime(beijingParts(result + 3600000)) === target) return null;
  return result;
}
function updateMaintenanceInput(input: HTMLInputElement | null, key: 'maintenanceStartAt' | 'maintenanceEndAt') {
  if (input && input.value !== beijingInputValue(draft[key])) draft[key] = parseBeijingInput(input.value) ?? 0;
}
function syncMaintenanceInputs() {
  if (draft.mode !== 'maintenance' || preview.value) return;
  updateMaintenanceInput(startInput.value, 'maintenanceStartAt');
  updateMaintenanceInput(endInput.value, 'maintenanceEndAt');
}
watch(() => draft.mode, mode => {
  if (editor.value && mode !== 'maintenance') { draft.maintenanceStartAt = 0; draft.maintenanceEndAt = 0; }
}, { flush: 'sync' });
function emptyDraft(): ClientUpdateDraft { return { id: 0, version: '', buildCode: 0, versionPolicy: true, versionName: '', title: '', content: '', mode: 'notice', minLoginBuild: 0, minRunBuild: 0, maintenanceStartAt: 0, maintenanceEndAt: 0, reason: '' }; }
function parseVersion(value: string): [number, number, number] | null {
  const match = /^(0|[1-9]\d{0,2})\.(0|[1-9]\d{0,2})\.(0|[1-9]\d{0,2})$/.exec(value);
  return match ? [Number(match[1]), Number(match[2]), Number(match[3])] : null;
}
function compareVersion(left: string, right: string): number | null {
  const a = parseVersion(left); const b = parseVersion(right);
  if (!a || !b) return null;
  for (let i = 0; i < 3; i += 1) if (a[i] !== b[i]) return a[i] - b[i];
  return 0;
}
const versionName = computed(() => `${versionParts.major}.${versionParts.minor}.${versionParts.patch}`);
const publishVersionProblem = (value: string) => {
  const policy = overview.value?.policy;
  if (!policy || !parseVersion(value)) return '';
  const limits = [
    { value: policy.releaseId ? policy.versionName : '', label: '当前已发布版本' },
    { value: policy.minLoginVersion || '', label: '当前登录版本要求' },
    { value: policy.minRunVersion || '', label: '当前运行版本要求' }
  ];
  const limit = limits.find(item => {
    const comparison = compareVersion(value, item.value);
    return comparison !== null && comparison < 0;
  });
  return limit ? `版本 ${value} 低于${limit.label} ${limit.value}，可以保存草稿，发布前须调整版本。` : '';
};
const draftVersionProblem = computed(() => publishVersionProblem(versionName.value));
const editorModeEffect = computed(() => {
  const version = parseVersion(versionName.value) ? versionName.value : '目标版本';
  if (draft.mode === 'maintenance') return `发布后向所有客户端预告；维护开始时在线玩家将退出，登录及业务暂停。若开始时间已过，发布时立即维护。预计结束时间不会自动开服，须手动结束维护；结束后低于 ${version} 或版本无法识别的客户端需要更新。`;
  return modeEffect(draft.mode, version);
});
const dirty = computed(() => editor.value && JSON.stringify({ draft, versionParts }) !== baseline.value);
const hasUnsavedChanges = () => dirty.value || (editor.value && dateInputEdited);
const canEdit = computed(() => Boolean(overview.value?.canEdit));
const canPublish = computed(() => Boolean(overview.value?.canPublish));
const blocked = computed(() => busy.value || loading.value || conflict.value || Boolean(pending.value));
const modes = [{ label: '仅提醒更新', value: 'notice' }, { label: '下次登录要求更新', value: 'next_login' }, { label: '维护更新', value: 'maintenance' }];
const modeName = (mode: UpdateMode) => modes.find(item => item.value === mode)?.label || '未发布';
const buildThreshold = (value: number) => value > 0 ? String(value) : '不限';
function modeEffect(mode: UpdateMode, version: string, schedule?: { start: number; end: number }): string {
  if (mode === 'next_login') return `发布后，低于 ${version} 或版本无法识别的客户端会收到提醒，下次登录时需要先更新。符合当前运行要求的在线会话可继续运行；已生效的限制继续保留。`;
  if (mode === 'maintenance') {
    const start = schedule?.start || 0; const end = schedule?.end || 0;
    const phase = maintenancePhase(start, maintenanceNow.value);
    let startEffect = `计划北京时间 ${formatTime(start)} 开始维护；服务端时间未到时先向所有客户端预告，已到时立即维护。`;
    if (phase === 'future') startEffect = `按当前服务端时间，发布后先向所有客户端预告，${formatTime(start)}（北京时间）开始维护；若服务端时间已到则发布后立即维护。`;
    if (phase === 'due') startEffect = '服务端时间已到开始时间，发布后立即维护。';
    return `${startEffect}所有在线玩家将退出，登录及业务暂停。预计 ${formatTime(end)}（北京时间）结束，须手动结束维护才会开放；结束后低于 ${version} 或版本无法识别的客户端需要更新。`;
  }
  return `发布后，低于 ${version} 或版本无法识别的客户端会收到提醒；已生效的登录和运行限制继续保留。`;
}
function legacyEffect(row: ClientRelease): string {
  if (row.mode === 'maintenance') return `计划 ${formatTime(row.maintenanceStartAt)} 开始维护。开始时间已到时，发布后立即进入维护；预计 ${formatTime(row.maintenanceEndAt)} 结束，须手动结束维护才会开放。结束后启用该记录的登录构建门槛 ${buildThreshold(row.minLoginBuild)}、运行构建门槛 ${buildThreshold(row.minRunBuild)}。`;
  if (row.mode === 'next_login') return `发布后提醒适用的旧版客户端；低于登录构建门槛 ${buildThreshold(row.minLoginBuild)} 的客户端下次登录需要更新，当前运行构建门槛为 ${buildThreshold(row.minRunBuild)}。`;
  return '发布后提醒适用的旧版客户端，已生效的登录和运行限制继续保留。';
}
function auditReason(verb: string, version: string, extra: string) {
  return `${verb} ${version || '未记录版本'}${extra.trim() ? `；补充：${extra.trim()}` : ''}`;
}
const deliveryMetrics = computed(() => {
  if (!overview.value) return [];
  const { policy, stats } = overview.value;
  return [
    { label: '在线连接', value: stats.online },
    { label: '已展示', value: policy.noticeEnabled ? stats.shown : '—' },
    { label: '已确认', value: policy.noticeEnabled ? stats.acknowledged : '—' },
    { label: '尚未展示', value: policy.noticeEnabled ? stats.pending : '—' }
  ];
});
const stateName = (state: string) => ({ draft: '草稿', published: '已发布', withdrawn: '已撤回提醒', cancelled: '维护已取消' })[state] || state;
function maintenancePhase(start: number, now: ServerTimeRange | null): 'future' | 'due' | 'uncertain' {
  if (!start || !now) return 'uncertain';
  if (start <= now.earliest) return 'due';
  if (start > now.latest) return 'future';
  return 'uncertain';
}
function estimateServerTime(): ServerTimeRange | null {
  if (!serverClock) return null;
  const elapsed = performance.now() - serverClock.receivedAt;
  if (elapsed < 0 || elapsed > 60000) return null;
  const earliest = serverClock.observedAt + elapsed;
  return { earliest, latest: earliest + serverClock.roundTrip };
}
function commitOverview(result: ClientUpdateOverview, requestStartedAt: number) {
  const receivedAt = performance.now();
  const roundTrip = receivedAt - requestStartedAt;
  const observedAt = result.stats?.observedAt;
  serverClock = typeof observedAt === 'number' && Number.isFinite(observedAt) && observedAt >= 1000000000000 &&
    observedAt <= 8640000000000000 && roundTrip >= 0 && roundTrip <= 30000
    ? { observedAt, receivedAt, roundTrip } : null;
  overview.value = result;
  refreshMaintenanceClock();
}
async function loadHistory() {
  historyError.value = '';
  try { const result = await getClientUpdateHistory(page.value); history.value = result.items || []; total.value = result.total; }
  catch (e) { historyError.value = errorMessage(e); }
}
async function load() {
  if (busy.value || pending.value) return;
  loading.value = true; error.value = '';
  try { const startedAt = performance.now(); commitOverview(await getClientUpdates(), startedAt); conflict.value = false; await loadHistory(); }
  catch (e) { error.value = errorMessage(e); }
  finally { loading.value = false; }
}
function edit(row?: ClientRelease) {
  if (!canEdit.value || blocked.value) return;
  const values = emptyDraft();
  for (const key of Object.keys(values) as (keyof ClientUpdateDraft)[]) {
    if (row && key in row) Object.assign(values, { [key]: row[key as keyof ClientRelease] });
  }
  const parts = row ? parseVersion(row.versionName) : null;
  Object.assign(versionParts, { major: parts ? String(parts[0]) : '', minor: parts ? String(parts[1]) : '', patch: parts ? String(parts[2]) : '' });
  unparsedVersion.value = row && !parts ? row.versionName || '（空）' : '';
  Object.assign(draft, values, { version: overview.value!.version, versionPolicy: true, buildCode: 0, minLoginBuild: 0, minRunBuild: 0, reason: '' });
  dateInputEdited = false; baseline.value = JSON.stringify({ draft, versionParts }); preview.value = false; formError.value = ''; editorConflict.value = false; recoveryNotice.value = ''; editor.value = true;
}
function validate() {
  if (!parseVersion(versionName.value)) return '请完整填写三段版本号，每段须为 0 至 999 的整数，且不含前导零';
  if (!draft.title.trim() || !draft.content.trim()) return '请填写提醒标题和更新说明';
  if (draft.mode === 'maintenance' && (!draft.maintenanceStartAt || !draft.maintenanceEndAt || draft.maintenanceEndAt <= draft.maintenanceStartAt)) return '请填写维护开始及预计结束时间，结束时间须晚于开始时间';
  return '';
}
function showPreview() { syncMaintenanceInputs(); formError.value = validate(); if (!formError.value) preview.value = true; }
async function save() {
  if (busy.value || !canEdit.value || conflict.value) return;
  formError.value = validate(); if (formError.value) return;
  busy.value = true;
  try {
    const startedAt = performance.now();
    commitOverview(await saveClientUpdate({ ...draft, versionPolicy: true, buildCode: 0, minLoginBuild: 0, minRunBuild: 0, title: draft.title.trim(), content: draft.content.trim(), versionName: versionName.value, reason: auditReason(`${draft.id ? '保存' : '新建'}${modeName(draft.mode)}草稿`, versionName.value, draft.reason) }), startedAt);
    editor.value = false; window.$message?.success('草稿已保存，尚未发布'); await loadHistory();
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) {
      editorConflict.value = true; conflict.value = true; recoveryNotice.value = '';
      formError.value = '更新策略已变化，输入仍保留。请刷新状态后重新预览。';
    } else formError.value = errorMessage(e);
  }
  finally { busy.value = false; }
}
async function refreshDraftState() {
  if (busy.value || !editorConflict.value) return;
  busy.value = true; formError.value = '';
  try {
    const startedAt = performance.now();
    const latest = await getClientUpdates();
    const latestHistory = await getClientUpdateHistory(page.value);
    const wasExisting = draft.id > 0;
    commitOverview(latest, startedAt);
    history.value = latestHistory.items || []; total.value = latestHistory.total; historyError.value = '';
    draft.version = latest.version;
    if (wasExisting) draft.id = 0;
    preview.value = false; editorConflict.value = false; conflict.value = false;
    recoveryNotice.value = wasExisting
      ? '原草稿可能已被其他人修改，本次输入将另存为新草稿，请重新预览后保存。'
      : '已刷新更新策略，输入仍保留，请重新预览后保存。';
  } catch {
    formError.value = '刷新状态失败，输入仍保留，请重试。';
  } finally { busy.value = false; }
}
function discard(title: string, content: string): Promise<boolean> {
  return new Promise(resolve => {
    if (!window.$dialog) { resolve(false); return; }
    window.$dialog.warning({ title, content, positiveText: '放弃并继续', negativeText: '返回', onPositiveClick: () => resolve(true), onNegativeClick: () => resolve(false), onClose: () => resolve(false), onMaskClick: () => resolve(false) });
  });
}
async function closeEditor() {
  if (busy.value) return;
  if (hasUnsavedChanges() && !await discard('放弃未保存的修改？', '当前输入尚未保存，关闭后将丢失。')) return;
  editor.value = false; editorConflict.value = false; recoveryNotice.value = '';
}
const inspecting = ref<ClientRelease | null>(null);
function publishControl(row: ClientRelease) {
  if (row.state !== 'draft' || !canPublish.value) return null;
  if (!row.versionPolicy) return h('span', { class: 'ops-muted legacy-draft-hint' }, '请先编辑并保存');
  return h(NButton, { size: 'small', type: 'primary', disabled: blocked.value, onClick: () => prepareAction('publish', row) }, () => '预览发布');
}
const historyColumns = computed<DataTableColumns<ClientRelease>>(() => [
  { title: '版本 / 提醒标题', key: 'title', minWidth: 230, render: row => h('div', { class: 'release-identity' }, [h('strong', row.versionName), h('span', { class: 'ops-muted' }, row.title)]) },
  { title: '更新方式', key: 'mode', width: 165, render: row => modeName(row.mode) },
  { title: '状态', key: 'state', width: 110, render: row => h(NTag, { size: 'small', bordered: false, type: row.state === 'draft' ? 'default' : 'info' }, () => stateName(row.state)) },
  { title: '更新时间（北京时间）', key: 'updatedAt', width: 185, render: row => formatTime(row.publishedAt || row.updatedAt) },
  { title: '操作', key: 'actions', width: 240, render: row => h(NSpace, { size: 8, wrap: false }, () => [
    h(NButton, { size: 'small', quaternary: true, onClick: () => { inspecting.value = row; } }, () => '详情'),
    row.state === 'draft' && canEdit.value ? h(NButton, { size: 'small', disabled: blocked.value, onClick: () => edit(row) }, () => '编辑') : null,
    publishControl(row)
  ]) }
]);
const actionOpen = ref(false); const actionReason = ref(''); const actionError = ref(''); const confirmed = ref(false);
const selected = ref<ClientRelease | null>(null); const action = ref<UpdateAction>('publish');
const currentRelease = computed(() => [...(overview.value?.items || []), ...history.value].find(item => item.id === overview.value?.policy.releaseId) || null);
const currentPlanPhase = computed(() => maintenancePhase(overview.value?.policy.maintenanceStartAt || 0, maintenanceNow.value));
let maintenanceTimer: number | undefined;
function refreshMaintenanceClock() {
  const now = estimateServerTime();
  const start = selected.value?.maintenanceStartAt || 0;
  if (actionOpen.value && (action.value === 'publish' || action.value === 'cancel_maintenance') && selected.value?.mode === 'maintenance' && start && !pending.value &&
      maintenancePhase(start, maintenanceNow.value) !== maintenancePhase(start, now) && confirmed.value) {
    confirmed.value = false;
    actionError.value = '维护开始状态已变化，请重新核对操作影响。';
  }
  maintenanceNow.value = now;
}
watch(
  () => (editor.value && preview.value && draft.mode === 'maintenance') ||
    (actionOpen.value && (action.value === 'publish' || action.value === 'cancel_maintenance') && selected.value?.mode === 'maintenance') ||
    Boolean(overview.value?.policy.maintenanceStartAt && !overview.value?.policy.maintenance),
  active => {
    if (maintenanceTimer !== undefined) window.clearInterval(maintenanceTimer);
    maintenanceTimer = undefined;
    if (active) {
      refreshMaintenanceClock();
      maintenanceTimer = window.setInterval(refreshMaintenanceClock, 1000);
    }
  },
  { flush: 'sync' }
);
const actionNames: Record<UpdateAction, string> = { publish: '发布并提醒', remind: '再次提醒', withdraw: '撤回提醒', start_maintenance: '立即开始维护', end_maintenance: '结束维护', cancel_maintenance: '取消维护计划' };
const actionVersionProblem = computed(() => {
  if (action.value === 'cancel_maintenance' && !pending.value && maintenancePhase(selected.value?.maintenanceStartAt || 0, maintenanceNow.value) !== 'future') return '无法确认维护尚未开始，请关闭窗口并刷新状态。';
  if (action.value !== 'publish' || !selected.value) return '';
  if (!selected.value.versionPolicy) return '旧格式草稿需要先编辑，填写三段版本号并保存，才可以发布。';
  return parseVersion(selected.value.versionName) ? publishVersionProblem(selected.value.versionName) : '此草稿的版本号不完整或无效，请先编辑草稿并重新填写版本。';
});
const actionEffect = computed(() => {
  const row = selected.value;
  if (action.value === 'withdraw') return '停止当前提醒。已生效的登录和运行门槛保持，维护计划和维护状态也保持。';
  if (action.value === 'remind') return '向符合当前提醒条件的客户端发送新一批提醒，已看过上一批提醒的玩家也会再次收到。';
  if (action.value === 'start_maintenance') return '当前接入运行区服立即进入维护，在线玩家将退出，业务请求受到拦截。';
  if (action.value === 'cancel_maintenance') return '取消维护计划并关闭维护预告。已看过预告的玩家不会自动收到取消通知，已生效的版本门槛保持不变。';
  if (action.value === 'end_maintenance') return row?.versionPolicy ? `当前接入运行区服恢复开放，低于 ${row.versionName} 或版本无法识别的客户端需要先更新；原有生效限制继续保留。预计结束时间不会自动开放。` : `当前接入运行区服恢复开放，并启用该记录的登录构建门槛 ${buildThreshold(row?.minLoginBuild || 0)}、运行构建门槛 ${buildThreshold(row?.minRunBuild || 0)}。预计结束时间不会自动开放。`;
  if (!row) return '';
  return row.versionPolicy ? modeEffect(row.mode, row.versionName, { start: row.maintenanceStartAt, end: row.maintenanceEndAt }) : legacyEffect(row);
});
function prepareAction(value: UpdateAction, row?: ClientRelease) {
  if (!canPublish.value || blocked.value) return;
  selected.value = row || currentRelease.value;
  if (!selected.value) { error.value = '当前发布记录未载入，请刷新后重试'; return; }
  if (selected.value.state === 'cancelled') { error.value = '维护计划已取消，请刷新查看当前状态'; return; }
  if (value === 'cancel_maintenance') {
    refreshMaintenanceClock();
    if (currentPlanPhase.value !== 'future') { error.value = '无法确认维护尚未开始，请刷新状态'; return; }
  }
  action.value = value; actionReason.value = ''; actionError.value = ''; confirmed.value = false; actionOpen.value = true;
}
function handleActionFailure(e: unknown) {
  if (e instanceof ApiError && e.status === 409) {
    actionError.value = '更新策略已变化，请关闭窗口并刷新状态后重新核对操作。';
    pending.value = null; conflict.value = true;
  } else {
    actionError.value = errorMessage(e);
    if (e instanceof ApiError && e.status >= 400 && e.status < 500) pending.value = null;
  }
}
async function execute() {
  if ((action.value === 'publish' || action.value === 'cancel_maintenance') && selected.value?.mode === 'maintenance' && !pending.value) refreshMaintenanceClock();
  if (busy.value || !canPublish.value || !selected.value || !confirmed.value || actionVersionProblem.value) return;
  const retrying = Boolean(pending.value);
  pending.value ||= { id: selected.value.id, version: overview.value!.version, action: action.value, reason: auditReason(`${actionNames[action.value]}（${modeName(selected.value.mode)}）`, selected.value.versionName, actionReason.value), requestId: createRequestId() };
  busy.value = true; actionError.value = '';
  try {
    const startedAt = performance.now();
    const result = await actClientUpdate(pending.value);
    pending.value = null; actionOpen.value = false;
    if (retrying) {
      overview.value = result;
      serverClock = null;
      refreshMaintenanceClock();
      try {
        const refreshStartedAt = performance.now();
        commitOverview(await getClientUpdates(), refreshStartedAt);
      } catch {
        conflict.value = true;
        error.value = '操作已完成，但当前状态和服务器时间未能刷新，请点击刷新。';
      }
    } else {
      commitOverview(result, startedAt);
    }
    window.$message?.success(`${actionNames[action.value]}已完成`); await loadHistory();
  } catch (e) { handleActionFailure(e); }
  finally { busy.value = false; }
}
function closeAction() { if (!busy.value && !pending.value) actionOpen.value = false; }
async function leave() {
  if (busy.value || pending.value) { window.$message?.warning('操作结果尚未确认，请使用原请求重试'); return false; }
  return !hasUnsavedChanges() || await discard('离开客户端更新？', '未保存的修改将丢失。');
}
onBeforeRouteLeave(leave);
function beforeUnload(event: BeforeUnloadEvent) { if (hasUnsavedChanges() || busy.value || pending.value) { event.preventDefault(); event.returnValue = ''; } }
function onVisibilityChange() { if (document.hidden) { serverClock = null; refreshMaintenanceClock(); } }
onMounted(() => { load(); window.addEventListener('beforeunload', beforeUnload); document.addEventListener('visibilitychange', onVisibilityChange); });
onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', beforeUnload);
  document.removeEventListener('visibilitychange', onVisibilityChange);
  if (maintenanceTimer !== undefined) window.clearInterval(maintenanceTimer);
});
</script>

<template>
  <div class="ops-page update-page">
    <div class="update-toolbar">
      <NSpace :wrap="false">
        <NButton :loading="loading" :disabled="busy || !!pending" @click="load">刷新</NButton>
        <NButton v-if="canEdit" type="primary" :disabled="blocked" @click="edit()">新建草稿</NButton>
      </NSpace>
    </div>
    <NAlert v-if="error" type="error" :title="error" />
    <NAlert v-if="conflict" type="warning" :title="editorConflict ? '更新策略已变化，请在当前窗口刷新状态并保留输入。' : '当前状态需要刷新后才能继续。'" />
    <NSpin :show="loading">
      <div v-if="overview" class="update-content">
        <section class="update-panel" aria-labelledby="current-policy-title">
          <div class="section-heading">
            <h2 id="current-policy-title">当前更新策略</h2>
            <NSpace size="small">
              <NTag size="small" :type="overview.policy.maintenance ? 'error' : overview.policy.maintenanceStartAt && currentPlanPhase !== 'future' ? 'warning' : 'success'">{{ overview.policy.maintenance ? '维护中' : overview.policy.maintenanceStartAt && currentPlanPhase !== 'future' ? '状态待确认' : '开放中' }}</NTag>
              <NTag size="small" :type="overview.policy.noticeEnabled ? 'info' : 'default'">{{ overview.policy.noticeEnabled ? '提醒已开启' : '提醒已关闭' }}</NTag>
              <NTag v-if="currentRelease?.state === 'cancelled'" size="small">维护已取消</NTag>
            </NSpace>
          </div>
          <div class="policy-heading">
            <div class="policy-version">
              <strong class="version-name">{{ overview.policy.versionName || '尚未发布更新' }}</strong>
              <p v-if="overview.policy.releaseId" class="ops-muted">{{ modeName(overview.policy.mode) }}</p>
            </div>
            <NSpace v-if="canPublish && currentRelease && currentRelease.state !== 'cancelled'" class="policy-actions">
              <NButton v-if="!overview.policy.maintenance" :disabled="blocked || (!!overview.policy.maintenanceStartAt && currentPlanPhase !== 'future')" @click="prepareAction('remind')">再次提醒</NButton>
              <NButton v-if="!overview.policy.maintenance" :disabled="blocked || !overview.policy.noticeEnabled || (!!overview.policy.maintenanceStartAt && currentPlanPhase !== 'future')" @click="prepareAction('withdraw')">撤回提醒</NButton>
              <NButton v-if="!overview.policy.maintenance && overview.policy.mode === 'maintenance' && overview.policy.maintenanceStartAt && currentPlanPhase === 'future'" type="warning" :disabled="blocked" @click="prepareAction('start_maintenance')">开始维护</NButton>
              <NButton v-if="!overview.policy.maintenance && overview.policy.mode === 'maintenance' && overview.policy.maintenanceStartAt && currentPlanPhase === 'future'" :disabled="blocked" @click="prepareAction('cancel_maintenance')">取消维护计划</NButton>
              <NButton v-if="overview.policy.maintenance" type="warning" :disabled="blocked" @click="prepareAction('end_maintenance')">结束维护</NButton>
            </NSpace>
          </div>
          <dl class="policy-summary">
            <div><dt>最低登录版本</dt><dd>{{ overview.policy.minLoginVersion || '未设置版本要求' }}</dd></div>
            <div><dt>最低运行版本</dt><dd>{{ overview.policy.minRunVersion || '未设置版本要求' }}</dd></div>
          </dl>
          <details v-if="overview.policy.minLoginBuild || overview.policy.minRunBuild" class="legacy-policy">
            <summary>查看仍生效的历史构建限制</summary>
            <p>最低登录构建：{{ buildThreshold(overview.policy.minLoginBuild) }}；最低运行构建：{{ buildThreshold(overview.policy.minRunBuild) }}。这些限制继续生效。</p>
          </details>
          <p v-if="overview.policy.maintenanceStartAt" class="maintenance-note">维护时间（北京时间）：{{ formatTime(overview.policy.maintenanceStartAt) }} 开始，预计 {{ formatTime(overview.policy.maintenanceEndAt) }} 结束。结束维护后才会开放。<span v-if="!overview.policy.maintenance && currentPlanPhase !== 'future'"> 开始状态待确认，请刷新。</span></p>
          <p v-if="!canPublish" class="ops-muted permission-note">发布、提醒和维护操作需要管理员权限。</p>
        </section>
        <section class="update-panel delivery-panel" aria-labelledby="delivery-title">
          <div class="section-heading delivery-heading"><h2 id="delivery-title">提醒送达</h2><span class="ops-muted">{{ overview.policy.noticeEnabled ? '当前提醒批次' : '提醒未开启' }}</span></div>
          <dl class="delivery-grid"><div v-for="metric in deliveryMetrics" :key="metric.label"><dt>{{ metric.label }}</dt><dd>{{ metric.value }}</dd></div></dl>
          <div class="delivery-footer"><span>{{ overview.policy.versionPolicy ? '低于当前版本' : '低于当前构建' }} {{ overview.policy.releaseId ? overview.stats.older : '—' }} · {{ overview.policy.versionPolicy ? '版本未知' : '构建未知' }} {{ overview.stats.unknown }}</span><span>本服连接快照 · {{ formatTime(overview.stats.observedAt) }}（北京时间）</span><span v-if="overview.policy.noticeEnabled">尚未展示包括无需提醒的在线连接。</span></div>
        </section>
        <section class="update-panel history-panel" aria-labelledby="history-title">
          <div class="section-heading"><h2 id="history-title">草稿与发布记录 <span v-if="total" class="record-count">{{ total }}</span></h2></div>
          <NAlert v-if="historyError" type="error" :title="historyError" class="mb-16px"><NButton size="small" @click="loadHistory">重试加载历史</NButton></NAlert>
          <div class="desktop-records">
            <NDataTable :columns="historyColumns" :data="history" :row-key="(row: ClientRelease) => row.id" :bordered="false" :scroll-x="930">
              <template #empty>暂无草稿或发布记录</template>
            </NDataTable>
          </div>
          <div class="mobile-records">
            <p v-if="!history.length" class="ops-muted">暂无草稿或发布记录</p>
            <article v-for="row in history" :key="row.id" class="record-card">
              <div class="record-card-heading"><strong>{{ row.versionName || '未记录版本' }}</strong><NTag size="small" :bordered="false" :type="row.state === 'draft' || row.state === 'cancelled' ? 'default' : 'info'">{{ stateName(row.state) }}</NTag></div>
              <p>{{ row.title }}</p>
              <p class="ops-muted">{{ modeName(row.mode) }} · {{ formatTime(row.publishedAt || row.updatedAt) }}（北京时间）</p>
              <div class="record-card-actions">
                <NButton size="small" @click="inspecting = row">详情</NButton>
                <NButton v-if="row.state === 'draft' && canEdit" size="small" :disabled="blocked" @click="edit(row)">编辑</NButton>
                <NButton v-if="row.state === 'draft' && canPublish && row.versionPolicy" size="small" type="primary" :disabled="blocked" @click="prepareAction('publish', row)">预览发布</NButton>
                <span v-else-if="row.state === 'draft' && canPublish" class="ops-muted legacy-draft-hint">请先编辑并保存</span>
              </div>
            </article>
          </div>
          <div v-if="total > 20" class="ops-pagination"><NPagination v-model:page="page" :page-size="20" :item-count="total" simple @update:page="loadHistory" /></div>
        </section>
      </div>
    </NSpin>
    <NModal :show="!!inspecting" preset="card" title="更新记录详情" class="update-modal" @update:show="!$event && (inspecting = null)">
      <template v-if="inspecting">
        <NDescriptions :column="1" label-placement="left" class="mb-20px">
          <NDescriptionsItem label="版本">{{ inspecting.versionName || '未记录' }}</NDescriptionsItem>
          <NDescriptionsItem label="状态">{{ stateName(inspecting.state) }}</NDescriptionsItem>
          <NDescriptionsItem label="更新方式">{{ modeName(inspecting.mode) }}</NDescriptionsItem>
          <NDescriptionsItem v-if="!inspecting.versionPolicy" label="历史构建号">{{ inspecting.buildCode || '未记录' }}</NDescriptionsItem>
          <NDescriptionsItem v-if="!inspecting.versionPolicy" label="历史登录构建门槛">{{ buildThreshold(inspecting.minLoginBuild) }}</NDescriptionsItem>
          <NDescriptionsItem v-if="!inspecting.versionPolicy" label="历史运行构建门槛">{{ buildThreshold(inspecting.minRunBuild) }}</NDescriptionsItem>
          <NDescriptionsItem label="更新时间（北京时间）">{{ formatTime(inspecting.publishedAt || inspecting.updatedAt) }}</NDescriptionsItem>
          <NDescriptionsItem v-if="inspecting.publishedBy" label="发布人">{{ inspecting.publishedBy }}</NDescriptionsItem>
          <NDescriptionsItem v-if="inspecting.mode === 'maintenance'" label="维护时间（北京时间）">{{ formatTime(inspecting.maintenanceStartAt) }} 至 {{ formatTime(inspecting.maintenanceEndAt) }}（预计）</NDescriptionsItem>
        </NDescriptions>
        <div class="player-preview"><h3>{{ inspecting.title }}</h3><p class="player-content">{{ inspecting.content }}</p><p>请前往QQ群下载最新版</p></div>
      </template>
      <template #footer><div class="modal-actions"><NButton @click="inspecting = null">关闭</NButton></div></template>
    </NModal>
    <NModal :show="editor" preset="card" :title="preview ? '草稿预览' : draft.id ? '编辑更新草稿' : '新建更新草稿'" class="update-modal" :mask-closable="false" :closable="!busy" @update:show="!$event && closeEditor()">
      <NAlert v-if="formError" type="error" :title="formError" class="mb-16px" />
      <NAlert v-if="recoveryNotice" type="info" :title="recoveryNotice" class="mb-16px" />
      <NForm v-if="!preview" label-placement="top" :show-feedback="false" :disabled="busy" class="update-form">
        <NAlert v-if="unparsedVersion" type="warning" :title="`旧草稿版本“${unparsedVersion}”无法拆分，请重新填写三段版本号。`" />
        <NFormItem label="客户端版本" required>
          <div class="version-entry" role="group" aria-label="客户端版本号">
            <VersionSegment v-model="versionParts.major" label="主版本" :disabled="busy" />
            <span class="version-dot" aria-hidden="true">.</span>
            <VersionSegment v-model="versionParts.minor" label="次版本" :disabled="busy" />
            <span class="version-dot" aria-hidden="true">.</span>
            <VersionSegment v-model="versionParts.patch" label="修订版本" :disabled="busy" />
          </div>
        </NFormItem>
        <p id="client-version-help" class="ops-muted">每段填写 0 至 999 的整数，例如 1.1.0；可用方向键调整。请填写安装包实际版本，并确认对应安装包已放到QQ群。</p>
        <NFormItem label="更新方式"><NSelect v-model:value="draft.mode" :options="modes" aria-label="更新方式" /></NFormItem>
        <NAlert type="info" :title="editorModeEffect" />
        <div v-if="draft.mode === 'maintenance'" class="form-grid">
          <NFormItem label="维护开始时间（北京时间）" required><input ref="startInput" v-beijing-date-value="draft.maintenanceStartAt" class="beijing-datetime" type="datetime-local" step="1" :disabled="busy" aria-label="维护开始时间（北京时间）" aria-required="true" @input="markDateInputEdited" @change="updateMaintenanceInput(startInput, 'maintenanceStartAt')"></NFormItem>
          <NFormItem label="预计结束时间（北京时间）" required><input ref="endInput" v-beijing-date-value="draft.maintenanceEndAt" class="beijing-datetime" type="datetime-local" step="1" :disabled="busy" aria-label="预计结束时间（北京时间）" aria-required="true" @input="markDateInputEdited" @change="updateMaintenanceInput(endInput, 'maintenanceEndAt')"></NFormItem>
        </div>
        <NFormItem label="提醒标题（玩家可见）" required><NInput v-model:value="draft.title" :input-props="{ 'aria-label': '提醒标题' }" :maxlength="80" /></NFormItem><NFormItem label="更新说明（玩家可见）" required><NInput v-model:value="draft.content" :input-props="{ 'aria-label': '更新说明' }" type="textarea" :maxlength="4000" :autosize="{ minRows: 4, maxRows: 10 }" /></NFormItem>
        <div class="fixed-message">玩家固定提示：请前往QQ群下载最新版</div>
        <NFormItem label="补充审计说明（可选）"><NInput v-model:value="draft.reason" :input-props="{ 'aria-label': '补充审计说明' }" :maxlength="150" placeholder="系统会记录操作和版本" /></NFormItem>
      </NForm>
      <template v-else>
        <div class="player-preview"><h3>{{ draft.title }}</h3><p class="player-content">{{ draft.content }}</p><p>请前往QQ群下载最新版</p></div>
        <p>{{ versionName }} · {{ modeName(draft.mode) }}</p>
        <NAlert type="info" :title="modeEffect(draft.mode, versionName, { start: draft.maintenanceStartAt, end: draft.maintenanceEndAt })" class="mb-16px" />
        <NAlert v-if="draftVersionProblem" type="warning" :title="draftVersionProblem" class="mb-16px" />
        <p class="ops-muted">保存后生成草稿，管理员确认发布后才会生效。</p>
      </template>
      <template #footer><div class="modal-actions"><NButton :disabled="busy" @click="closeEditor">取消</NButton><NButton v-if="preview" :disabled="busy || editorConflict" @click="preview = false">返回修改</NButton><NButton v-if="editorConflict" type="primary" :loading="busy" @click="refreshDraftState">刷新状态并保留输入</NButton><NButton v-if="!preview" type="primary" :disabled="editorConflict" @click="showPreview">预览草稿</NButton><NButton v-else type="primary" :loading="busy && !editorConflict" :disabled="conflict || !canEdit" @click="save">保存草稿</NButton></div></template>
    </NModal>
    <NModal :show="actionOpen" preset="card" :title="actionNames[action]" class="update-modal" :mask-closable="false" :closable="!busy && !pending" @update:show="!$event && closeAction()">
      <NAlert v-if="actionError" type="error" :title="actionError" class="mb-16px" />
      <template v-if="selected"><div class="player-preview"><h3>{{ selected.title }}</h3><p class="player-content">{{ selected.content }}</p><p>请前往QQ群下载最新版</p></div><p>{{ selected.versionName }} · {{ modeName(selected.mode) }}</p><NAlert :type="action === 'start_maintenance' || action === 'end_maintenance' ? 'warning' : 'info'" :title="actionEffect" class="mb-16px" /><NAlert v-if="action === 'publish'" type="warning" :title="`请确认版本与实际安装包一致，且安装包已放到QQ群。系统不接收安装包，无法自动核验。${selected.mode !== 'notice' ? '强制更新会限制尚未取得新包的玩家登录。' : ''}`" class="mb-16px" /><NAlert v-if="actionVersionProblem" type="warning" :title="actionVersionProblem" class="mb-16px" /><NFormItem label="补充审计说明（可选）"><NInput v-model:value="actionReason" :input-props="{ 'aria-label': '补充审计说明' }" :disabled="busy || !!pending" :maxlength="150" placeholder="系统会记录操作和版本" /></NFormItem><NCheckbox v-model:checked="confirmed" :disabled="busy || !!pending">{{ action === 'publish' ? '已核对版本、玩家文案、QQ群安装包和操作后果' : '已核对上述版本、玩家文案和操作后果' }}</NCheckbox><p v-if="pending" class="ops-muted">结果尚未确认。重试沿用原请求，不会重复执行；请保留此窗口。</p></template>
      <template #footer><div class="modal-actions"><NButton :disabled="busy || !!pending" @click="closeAction">取消</NButton><NButton :type="action === 'start_maintenance' ? 'warning' : 'primary'" :loading="busy" :disabled="!confirmed || conflict || !canPublish || !!actionVersionProblem" @click="execute">{{ pending ? '重试原请求' : `确认${actionNames[action]}` }}</NButton></div></template>
    </NModal>
  </div>
</template>

<style scoped>
:deep(.release-identity) { display: flex; flex-direction: column; gap: 4px; overflow-wrap: anywhere; }
.update-page { padding-bottom: 24px; color: rgb(var(--base-text-color)); }
:deep(.legacy-draft-hint) { align-self: center; font-size: 12px; white-space: nowrap; }
.update-page :deep(.n-button--default-type) { color: rgb(var(--base-text-color)); }
.update-toolbar, .section-heading, .policy-heading, .delivery-footer, .release-row, .release-title { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.update-toolbar { justify-content: flex-end; }
.update-content { display: grid; grid-template-columns: minmax(0, 1.45fr) minmax(360px, 1fr); gap: 16px; align-items: stretch; }
.history-panel { grid-column: 1 / -1; }
.mobile-records { display: none; }
.delivery-panel { display: flex; flex-direction: column; }
.update-panel { padding: 20px; border-radius: 6px; background: rgb(var(--container-bg-color)); }
.section-heading { margin-bottom: 18px; gap: 12px; }
.section-heading h2 { margin: 0; font-size: 16px; line-height: 1.5; font-weight: 600; }
.policy-heading { align-items: flex-start; }
.version-name { font-size: 20px; font-weight: 600; line-height: 1.4; }
.policy-version p { margin: 6px 0 0; }
.policy-actions { flex-shrink: 0; }
.policy-summary, .delivery-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 24px; margin: 20px 0 0; }
.policy-summary { padding-top: 18px; border-top: 1px solid rgb(var(--base-text-color) / .08); }
.policy-summary > div, .delivery-grid > div { min-width: 0; overflow-wrap: anywhere; }
.policy-summary dt, .delivery-grid dt { font-size: 13px; line-height: 1.5; color: rgb(var(--base-text-color) / .65); }
.policy-summary dd, .delivery-grid dd { margin: 6px 0 0; line-height: 1.4; font-weight: 500; font-variant-numeric: tabular-nums; }
.delivery-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); margin-top: 0; gap: 16px 0; }
.delivery-grid > div:nth-child(3) { padding-left: 0; border-left: 0; }
.delivery-grid > div { padding: 0 22px; border-left: 1px solid rgb(var(--base-text-color) / .08); }
.delivery-grid > div:first-child { padding-left: 0; border-left: 0; }
.delivery-grid dd { font-size: 25px; font-weight: 600; }
.delivery-footer { padding-top: 18px; margin-top: auto; font-size: 12px; line-height: 1.6; color: rgb(var(--base-text-color) / .65); flex-wrap: wrap; gap: 6px 16px; }
.legacy-policy { margin-top: 14px; font-size: 13px; line-height: 1.6; color: rgb(var(--base-text-color) / .7); }
.legacy-policy summary { cursor: pointer; width: fit-content; }
.legacy-policy summary:focus-visible { outline: 2px solid rgb(var(--primary-color)); outline-offset: 3px; }
.legacy-policy p { margin: 8px 0 0; }
.maintenance-note { margin: 16px 0 0; line-height: 1.7; color: rgb(var(--warning-color)); }
.permission-note { margin-bottom: 0; }
.record-count { margin-left: 6px; color: rgb(var(--base-text-color) / .5); font-size: 13px; font-weight: 400; }
.history-empty { padding: 18px 0 20px; }
.release-row { align-items: flex-start; padding: 18px 0; border-bottom: 1px solid rgb(var(--base-text-color) / .08); }
.release-row:last-of-type { padding-bottom: 0; border-bottom: 0; }
.release-row:first-of-type { padding-top: 0; }
.release-copy { min-width: 0; flex: 1; overflow-wrap: anywhere; }
.release-title { justify-content: flex-start; flex-wrap: wrap; gap: 8px; }
.release-title h3 { margin: 0; font-size: 16px; font-weight: 600; }
.release-copy p { margin: 6px 0; }
.release-caption { font-size: 14px; }
.release-meta { font-size: 12px; color: rgb(var(--base-text-color) / .65); }
.release-details { margin-top: 10px; font-size: 13px; }
.release-details summary { width: fit-content; cursor: pointer; padding: 4px 0; line-height: 1.6; }
.release-details summary:focus-visible { outline: 2px solid rgb(var(--primary-color)); outline-offset: 3px; border-radius: 2px; }
.release-preview { padding: 12px 16px; margin-top: 8px; border-left: 2px solid rgb(var(--primary-color) / .3); background: rgb(var(--base-text-color) / .025); }
.release-preview h4 { margin: 0 0 8px; font-weight: 500; }
:global(.n-card.update-modal) { width: 720px; max-width: calc(100vw - 28px); }
.update-form { display: flex; flex-direction: column; gap: 16px; }
.update-form p { margin: -6px 0 0; line-height: 1.65; }
.version-entry { display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr) auto minmax(0, 1fr); align-items: start; gap: 8px; width: 100%; max-width: 430px; }
.version-dot { padding-top: 31px; font-size: 18px; font-weight: 600; line-height: 1; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.form-grid :deep(.n-input-number), .form-grid :deep(.n-date-picker) { width: 100%; }
.beijing-datetime { box-sizing: border-box; width: 100%; min-width: 0; height: 36px; padding: 0 10px; border: 1px solid rgb(var(--base-text-color) / .28); border-radius: 4px; background: rgb(var(--container-bg-color)); color: rgb(var(--base-text-color)); font: inherit; }
.beijing-datetime:focus-visible { outline: 2px solid rgb(var(--primary-color)); outline-offset: 1px; }
.beijing-datetime:disabled { opacity: .55; }
:global(.dark) .beijing-datetime { color-scheme: dark; }
.fixed-message, .player-preview { background: rgb(var(--base-text-color) / .04); padding: 16px; border-radius: 8px; }
.fixed-message { font-size: 13px; line-height: 1.6; color: rgb(var(--base-text-color) / .65); }
.player-preview { margin-bottom: 16px; }
.player-preview h3 { margin-top: 0; font-size: 17px; }
.player-content { white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.7; }
.modal-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
@media (max-width: 1200px) {
  .update-content { grid-template-columns: minmax(0, 1fr); }
  .delivery-grid { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 0; }
  .delivery-grid > div:nth-child(3) { border-left: 1px solid rgb(var(--base-text-color) / .08); padding-left: 22px; }
}
@media (max-width: 800px) {
  .policy-heading { flex-direction: column; }
  .update-panel { padding: 18px; }
  .delivery-grid > div { padding: 0 12px; }
}
@media (max-width: 640px) {
  .desktop-records { display: none; }
  .mobile-records { display: block; }
  .record-card { padding: 15px 0; border-bottom: 1px solid rgb(var(--base-text-color) / .08); }
  .record-card:first-of-type { padding-top: 0; }
  .record-card:last-of-type { border-bottom: 0; padding-bottom: 0; }
  .record-card-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
  .record-card-heading strong, .record-card p { min-width: 0; overflow-wrap: anywhere; }
  .record-card p { margin: 7px 0; line-height: 1.5; }
  .record-card-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-top: 12px; }
  .update-toolbar { flex-direction: column; align-items: stretch; gap: 10px; }
  .update-toolbar > :deep(.n-space) { align-self: flex-end; }
  .section-heading { align-items: flex-start; flex-wrap: wrap; margin-bottom: 14px; }
  .section-heading > :deep(.n-space) { gap: 6px !important; }
  .policy-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
  .delivery-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px 0; }
  .delivery-grid > div:nth-child(3) { border-left: 0; padding-left: 0; }
  .version-name { font-size: 18px; }
  .release-row { flex-direction: column; gap: 12px; }
  .release-row > :deep(.n-space) { align-self: flex-end; }
  .form-grid { grid-template-columns: minmax(0, 1fr); }
  .modal-actions .n-button { flex: 1 0 auto; }
}
</style>
