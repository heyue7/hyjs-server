<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { ApiError, errorMessage, formatTime } from '@/service/operations';
import { actClientUpdate, getClientUpdateHistory, getClientUpdates, saveClientUpdate } from '@/service/client-updates';
import type { ClientRelease, ClientUpdateCommand, ClientUpdateDraft, ClientUpdateOverview, UpdateAction, UpdateMode } from '@/service/client-updates';
import { createRequestId } from '@/utils/request-id';

const overview = ref<ClientUpdateOverview | null>(null);
const loading = ref(false); const busy = ref(false); const error = ref(''); const conflict = ref(false);
const history = ref<ClientRelease[]>([]); const total = ref(0); const page = ref(1); const historyError = ref('');
const editor = ref(false); const preview = ref(false); const formError = ref(''); const baseline = ref('');
const draft = reactive<ClientUpdateDraft>(emptyDraft());
let preparingDraft = false;
function applyModeDefaults() {
  if (preparingDraft || !editor.value) return;
  const policy = overview.value!.policy;
  draft.minLoginBuild = draft.mode === 'notice' ? policy.minLoginBuild : draft.buildCode;
  draft.minRunBuild = draft.mode === 'maintenance' ? draft.buildCode : policy.minRunBuild;
  if (draft.mode !== 'maintenance') { draft.maintenanceStartAt = 0; draft.maintenanceEndAt = 0; }
}
watch(() => [draft.mode, draft.buildCode], applyModeDefaults, { flush: 'sync' });
function emptyDraft(): ClientUpdateDraft { return { id: 0, version: '', buildCode: 2026102101, versionName: '', title: '', content: '', mode: 'notice', minLoginBuild: 0, minRunBuild: 0, maintenanceStartAt: 0, maintenanceEndAt: 0, reason: '' }; }
const dirty = computed(() => editor.value && JSON.stringify(draft) !== baseline.value);
const canEdit = computed(() => Boolean(overview.value?.canEdit));
const canPublish = computed(() => Boolean(overview.value?.canPublish));
const blocked = computed(() => busy.value || loading.value || conflict.value || Boolean(pending.value));
const modes = [{ label: '仅提醒更新', value: 'notice' }, { label: '下次登录要求更新', value: 'next_login' }, { label: '维护更新', value: 'maintenance' }];
const modeName = (mode: UpdateMode) => modes.find(item => item.value === mode)?.label || '未发布';
const stateName = (state: string) => ({ draft: '草稿', published: '已发布', withdrawn: '已撤回提醒' })[state] || state;
async function loadHistory() {
  historyError.value = '';
  try { const result = await getClientUpdateHistory(page.value); history.value = result.items || []; total.value = result.total; }
  catch (e) { historyError.value = errorMessage(e); }
}
async function load() {
  if (busy.value || pending.value) return;
  loading.value = true; error.value = '';
  try { overview.value = await getClientUpdates(); conflict.value = false; await loadHistory(); }
  catch (e) { error.value = errorMessage(e); }
  finally { loading.value = false; }
}
function edit(row?: ClientRelease) {
  if (!canEdit.value || blocked.value) return;
  const policy = overview.value!.policy;
  const values = emptyDraft();
  for (const key of Object.keys(values) as (keyof ClientUpdateDraft)[]) {
    if (row && key in row) Object.assign(values, { [key]: row[key as keyof ClientRelease] });
  }
  if (!row) { values.minLoginBuild = policy.minLoginBuild; values.minRunBuild = policy.minRunBuild; }
  preparingDraft = true;
  Object.assign(draft, values, { version: overview.value!.version, reason: '' });
  preparingDraft = false;
  baseline.value = JSON.stringify(draft); preview.value = false; formError.value = ''; editor.value = true;
}
function validate() {
  if (!draft.title.trim() || !draft.content.trim() || !draft.versionName.trim()) return '请填写版本名称、提醒标题和更新说明';
  if (!draft.reason.trim()) return '请填写操作原因';
  if (![draft.buildCode, draft.minLoginBuild, draft.minRunBuild].every(value => Number.isSafeInteger(value) && value >= 0) || draft.buildCode < 1 || draft.buildCode > 2100000000) return '构建号须为 1 至 2100000000 的整数，兼容门槛须为非负整数';
  const policy = overview.value!.policy;
  if (draft.minRunBuild > draft.minLoginBuild || draft.minLoginBuild > draft.buildCode) return '门槛须满足：最低运行构建 ≤ 最低登录构建 ≤ 最新构建号';
  if (draft.mode === 'notice' && (draft.minLoginBuild > policy.minLoginBuild || draft.minRunBuild > policy.minRunBuild)) return '仅提醒更新不能提高登录或运行门槛';
  if (draft.mode === 'next_login' && (draft.minRunBuild > policy.minRunBuild || draft.minLoginBuild <= 0)) return '下次登录更新须设置正数登录门槛，运行门槛不能提高';
  if (draft.mode === 'maintenance' && (!draft.maintenanceStartAt || !draft.maintenanceEndAt || draft.maintenanceEndAt <= draft.maintenanceStartAt)) return '请填写维护开始及预计结束时间，结束时间须晚于开始时间';
  return '';
}
function showPreview() { formError.value = validate(); if (!formError.value) preview.value = true; }
async function save() {
  if (busy.value || !canEdit.value || conflict.value) return;
  formError.value = validate(); if (formError.value) return;
  busy.value = true;
  try {
    overview.value = await saveClientUpdate({ ...draft, title: draft.title.trim(), content: draft.content.trim(), versionName: draft.versionName.trim(), reason: draft.reason.trim() });
    editor.value = false; window.$message?.success('草稿已保存，尚未发布'); await loadHistory();
  } catch (e) { formError.value = errorMessage(e); if (e instanceof ApiError && e.status === 409) conflict.value = true; }
  finally { busy.value = false; }
}
function discard(title: string, content: string): Promise<boolean> {
  return new Promise(resolve => {
    if (!window.$dialog) { resolve(false); return; }
    window.$dialog.warning({ title, content, positiveText: '放弃并继续', negativeText: '返回', onPositiveClick: () => resolve(true), onNegativeClick: () => resolve(false), onClose: () => resolve(false), onMaskClick: () => resolve(false) });
  });
}
async function closeEditor() {
  if (busy.value) return;
  if (dirty.value && !await discard('放弃未保存的修改？', '当前输入尚未保存，关闭后将丢失。')) return;
  editor.value = false;
}
const actionOpen = ref(false); const actionReason = ref(''); const actionError = ref(''); const confirmed = ref(false);
const selected = ref<ClientRelease | null>(null); const action = ref<UpdateAction>('publish');
const pending = ref<ClientUpdateCommand | null>(null);
const actionNames: Record<UpdateAction, string> = { publish: '发布并提醒', remind: '再次提醒', withdraw: '撤回提醒', start_maintenance: '立即开始维护', end_maintenance: '结束维护' };
const actionEffect = computed(() => {
  const row = selected.value;
  if (action.value === 'withdraw') return '停止当前提醒。已生效的登录和运行门槛保持，维护计划和维护状态也保持。';
  if (action.value === 'remind') return '创建新提醒批次，已看过上一批次的玩家也会再次收到提醒。';
  if (action.value === 'start_maintenance') return '当前接入运行区服立即进入维护，在线玩家将退出，业务请求受到拦截。';
  if (action.value === 'end_maintenance') return `当前接入运行区服恢复开放，并启用此发布的登录门槛 ${row?.minLoginBuild ?? 0}、运行门槛 ${row?.minRunBuild ?? 0}。预计结束时间不会自动开放。`;
  if (row?.mode === 'maintenance') return `保存维护计划并提醒玩家。开始时间 ${formatTime(row.maintenanceStartAt)} 到达后拦截登录和业务，预计结束时间后仍须手动结束维护。`;
  if (row?.mode === 'next_login') return `提醒玩家更新。构建号低于 ${row.minLoginBuild} 的客户端下次登录受限；已在线且达到运行门槛的合法会话继续运行。运行门槛为 ${row.minRunBuild}。`;
  return `提醒玩家更新，登录门槛 ${row?.minLoginBuild ?? 0}，运行门槛 ${row?.minRunBuild ?? 0}。发布不会降低已生效门槛。`;
});
function prepareAction(value: UpdateAction, row?: ClientRelease) {
  if (!canPublish.value || blocked.value) return;
  selected.value = row || [...(overview.value?.items || []), ...history.value].find(item => item.id === overview.value?.policy.releaseId) || null;
  if (!selected.value) { error.value = '当前发布记录未载入，请刷新后重试'; return; }
  action.value = value; actionReason.value = ''; actionError.value = ''; confirmed.value = false; actionOpen.value = true;
}
async function execute() {
  if (busy.value || !canPublish.value || !selected.value || !confirmed.value) return;
  if (!actionReason.value.trim()) { actionError.value = '请填写操作原因'; return; }
  pending.value ||= { id: selected.value.id, version: overview.value!.version, action: action.value, reason: actionReason.value.trim(), requestId: createRequestId() };
  busy.value = true; actionError.value = '';
  try {
    overview.value = await actClientUpdate(pending.value); pending.value = null; actionOpen.value = false;
    window.$message?.success(`${actionNames[action.value]}已完成`); await loadHistory();
  } catch (e) {
    actionError.value = errorMessage(e);
    if (e instanceof ApiError && e.status >= 400 && e.status < 500) { pending.value = null; if (e.status === 409) conflict.value = true; }
  } finally { busy.value = false; }
}
function closeAction() { if (!busy.value && !pending.value) actionOpen.value = false; }
async function leave() {
  if (busy.value || pending.value) { window.$message?.warning('操作结果尚未确认，请使用原请求重试'); return false; }
  return !dirty.value || await discard('离开客户端更新？', '未保存的修改将丢失。');
}
onBeforeRouteLeave(leave);
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value || busy.value || pending.value) { event.preventDefault(); event.returnValue = ''; } }
onMounted(() => { load(); window.addEventListener('beforeunload', beforeUnload); });
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));
</script>

<template>
  <div class="ops-page update-page">
    <div class="ops-filters"><span class="ops-muted">更新策略针对同一 APK；维护仅作用于当前接入运行区服。</span><NSpace><NButton :loading="loading" :disabled="busy || !!pending" @click="load">刷新</NButton><NButton v-if="canEdit" type="primary" :disabled="blocked" @click="edit()">新建草稿</NButton></NSpace></div>
    <NAlert v-if="error" type="error" :title="error" />
    <NAlert v-if="conflict" type="warning" title="策略已被其他操作更新。请关闭编辑或确认窗口，刷新后重新核对；当前输入仍保留。" />
    <NSpin :show="loading">
      <template v-if="overview">
        <NCard title="当前生效策略" :bordered="false" class="ops-panel">
          <div class="policy-heading"><div><strong class="version-name">{{ overview.policy.versionName || '尚未发布更新' }}</strong><p class="ops-muted">最新构建 {{ overview.policy.latestBuild }} · 策略修订 {{ overview.policy.revision }}</p></div><NSpace><NTag :type="overview.policy.maintenance ? 'error' : 'success'">{{ overview.policy.maintenance ? '维护中' : '开放中' }}</NTag><NTag>{{ overview.policy.noticeEnabled ? '提醒已开启' : '提醒已关闭' }}</NTag></NSpace></div>
          <div class="policy-details"><div><span>更新方式</span><strong>{{ modeName(overview.policy.mode) }}</strong></div><div><span>最低登录构建</span><strong>{{ overview.policy.minLoginBuild }}</strong></div><div><span>最低运行构建</span><strong>{{ overview.policy.minRunBuild }}</strong></div><div><span>提醒批次</span><strong>{{ overview.policy.noticeBatch || '—' }}</strong></div></div>
          <p v-if="overview.policy.maintenanceStartAt" class="ops-muted">维护开始 {{ formatTime(overview.policy.maintenanceStartAt) }} · 预计结束 {{ formatTime(overview.policy.maintenanceEndAt) }}（须手动开放）</p>
          <NSpace v-if="canPublish && overview.policy.releaseId" class="mt-16px"><NButton :disabled="blocked" @click="prepareAction('remind')">再次提醒</NButton><NButton :disabled="blocked || !overview.policy.noticeEnabled" @click="prepareAction('withdraw')">撤回提醒</NButton><NButton v-if="!overview.policy.maintenance" type="warning" :disabled="blocked || overview.policy.mode !== 'maintenance'" @click="prepareAction('start_maintenance')">开始维护</NButton><NButton v-else type="warning" :disabled="blocked" @click="prepareAction('end_maintenance')">结束维护</NButton></NSpace>
          <p v-if="!canPublish" class="ops-muted">发布、提醒和维护操作需要管理员权限。</p>
        </NCard>
        <NCard title="当前提醒送达" :bordered="false" class="ops-panel mt-16px"><div class="delivery-grid"><div v-for="metric in [{ label: '在线连接', value: overview.stats.online }, { label: '旧版', value: overview.stats.older }, { label: '版本未知', value: overview.stats.unknown }, { label: '已展示', value: overview.stats.shown }, { label: '已确认', value: overview.stats.acknowledged }, { label: '待送达', value: overview.stats.pending }]" :key="metric.label"><span>{{ metric.label }}</span><strong>{{ metric.value }}</strong></div></div><p class="ops-muted">当前本服务连接快照 · {{ formatTime(overview.stats.observedAt) }}。展示和确认按当前提醒批次统计，未接入区服不在统计范围。</p></NCard>
        <NCard title="草稿与发布历史" :bordered="false" class="ops-panel mt-16px">
          <NAlert v-if="historyError" type="error" :title="historyError" class="mb-16px"><NButton size="small" @click="loadHistory">重试加载历史</NButton></NAlert>
          <NEmpty v-if="!history.length && !historyError" description="暂无草稿或发布记录" />
          <div v-for="row in history" :key="row.id" class="release-row"><div class="release-copy"><div class="release-title"><strong>{{ row.versionName }} · {{ row.title }}</strong><NTag size="small" :type="row.state === 'draft' ? 'default' : 'info'">{{ stateName(row.state) }}</NTag></div><p>{{ modeName(row.mode) }} · 构建 {{ row.buildCode }} · 登录 ≥ {{ row.minLoginBuild }} · 运行 ≥ {{ row.minRunBuild }}</p><p class="ops-muted">{{ row.state === 'draft' ? '更新' : '发布' }}于 {{ formatTime(row.publishedAt || row.updatedAt) }} <span v-if="row.publishedBy">· {{ row.publishedBy }}</span></p><details><summary>查看玩家文案及维护时间</summary><h4>{{ row.title }}</h4><p class="player-content">{{ row.content }}</p><p>请前往QQ群下载最新版</p><p v-if="row.mode === 'maintenance'">{{ formatTime(row.maintenanceStartAt) }} 至 {{ formatTime(row.maintenanceEndAt) }}（预计）</p></details></div><NSpace v-if="row.state === 'draft'"><NButton v-if="canEdit" size="small" :disabled="blocked" @click="edit(row)">编辑</NButton><NButton v-if="canPublish" size="small" type="primary" :disabled="blocked" @click="prepareAction('publish', row)">预览发布</NButton></NSpace></div>
          <div class="ops-pagination"><NPagination v-model:page="page" :page-size="20" :item-count="total" simple @update:page="loadHistory" /></div>
        </NCard>
      </template>
    </NSpin>
    <NModal :show="editor" preset="card" :title="preview ? '草稿预览' : draft.id ? '编辑更新草稿' : '新建更新草稿'" class="update-modal" :mask-closable="false" :closable="!busy" @update:show="!$event && closeEditor()">
      <NAlert v-if="formError" type="error" :title="formError" class="mb-16px" />
      <NForm v-if="!preview" label-placement="top" :show-feedback="false" :disabled="busy" class="update-form">
        <div class="form-grid"><NFormItem label="版本名称" required><NInput v-model:value="draft.versionName" :maxlength="80" placeholder="例如 1.0.41" /></NFormItem><NFormItem label="最新构建号" required><NInputNumber v-model:value="draft.buildCode" :min="1" :max="2100000000" :precision="0" :show-button="false" /></NFormItem></div>
        <NFormItem label="更新方式"><NSelect v-model:value="draft.mode" :options="modes" /></NFormItem>
        <div class="form-grid"><NFormItem label="最低登录构建"><NInputNumber v-model:value="draft.minLoginBuild" :min="0" :precision="0" :show-button="false" /></NFormItem><NFormItem label="最低运行构建"><NInputNumber v-model:value="draft.minRunBuild" :min="0" :precision="0" :show-button="false" /></NFormItem></div>
        <p class="ops-muted">切换方式或构建号后自动填写门槛，可按兼容范围调整。仅提醒不能提高门槛；下次登录更新保留当前运行门槛。发布不会降低已生效门槛。</p>
        <div v-if="draft.mode === 'maintenance'" class="form-grid"><NFormItem label="维护开始时间" required><NDatePicker :value="draft.maintenanceStartAt || null" type="datetime" @update:value="draft.maintenanceStartAt = $event || 0" /></NFormItem><NFormItem label="预计结束时间" required><NDatePicker :value="draft.maintenanceEndAt || null" type="datetime" @update:value="draft.maintenanceEndAt = $event || 0" /></NFormItem></div>
        <NFormItem label="提醒标题" required><NInput v-model:value="draft.title" :maxlength="80" /></NFormItem><NFormItem label="更新说明" required><NInput v-model:value="draft.content" type="textarea" :maxlength="4000" :autosize="{ minRows: 4, maxRows: 10 }" /></NFormItem>
        <div class="fixed-message">玩家固定提示：请前往QQ群下载最新版</div>
        <NFormItem label="操作原因" required><NInput v-model:value="draft.reason" :maxlength="300" placeholder="仅供后台审计记录" /></NFormItem>
      </NForm>
      <template v-else><div class="player-preview"><h3>{{ draft.title }}</h3><p class="player-content">{{ draft.content }}</p><p>请前往QQ群下载最新版</p></div><p>{{ draft.versionName }} · {{ modeName(draft.mode) }} · 构建 {{ draft.buildCode }}</p><p>登录 ≥ {{ draft.minLoginBuild }} · 运行 ≥ {{ draft.minRunBuild }}</p><p v-if="draft.mode === 'maintenance'">维护 {{ formatTime(draft.maintenanceStartAt) }} 至 {{ formatTime(draft.maintenanceEndAt) }}（预计，须手动开放）</p><NAlert type="info" title="保存后仅生成草稿。管理员预览并确认发布后，玩家才会收到提醒。" /></template>
      <template #footer><div class="modal-actions"><NButton :disabled="busy" @click="closeEditor">取消</NButton><NButton v-if="preview" :disabled="busy" @click="preview = false">返回修改</NButton><NButton v-if="!preview" type="primary" @click="showPreview">预览草稿</NButton><NButton v-else type="primary" :loading="busy" :disabled="conflict || !canEdit" @click="save">保存草稿</NButton></div></template>
    </NModal>
    <NModal :show="actionOpen" preset="card" :title="actionNames[action]" class="update-modal" :mask-closable="false" :closable="!busy && !pending" @update:show="!$event && closeAction()">
      <NAlert v-if="actionError" type="error" :title="actionError" class="mb-16px" />
      <template v-if="selected"><div class="player-preview"><h3>{{ selected.title }}</h3><p class="player-content">{{ selected.content }}</p><p>请前往QQ群下载最新版</p></div><p>{{ selected.versionName }} · 构建 {{ selected.buildCode }}</p><NAlert :type="action === 'start_maintenance' || action === 'end_maintenance' ? 'warning' : 'info'" :title="actionEffect" class="mb-16px" /><NFormItem label="操作原因" required><NInput v-model:value="actionReason" :disabled="busy || !!pending" :maxlength="300" placeholder="填写本次操作原因" /></NFormItem><NCheckbox v-model:checked="confirmed" :disabled="busy || !!pending">已核对上述版本、玩家文案和操作后果</NCheckbox><p v-if="pending" class="ops-muted">结果尚未确认。重试沿用原请求，不会重复执行；请保留此窗口。</p></template>
      <template #footer><div class="modal-actions"><NButton :disabled="busy || !!pending" @click="closeAction">取消</NButton><NButton :type="action === 'start_maintenance' ? 'warning' : 'primary'" :loading="busy" :disabled="!confirmed || conflict || !canPublish" @click="execute">{{ pending ? '重试原请求' : `确认${actionNames[action]}` }}</NButton></div></template>
    </NModal>
  </div>
</template>

<style scoped>
.update-page { padding-bottom: 24px; }
.policy-heading, .release-row, .release-title { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.version-name { font-size: 21px; line-height: 1.4; }
.policy-details, .delivery-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 20px; }
.policy-details > div, .delivery-grid > div { display: flex; flex-direction: column; gap: 6px; overflow-wrap: anywhere; }
.policy-details span, .delivery-grid span { font-size: 13px; color: rgb(var(--base-text-color) / .65); }
.delivery-grid { grid-template-columns: repeat(6, minmax(0, 1fr)); }
.delivery-grid strong { font-size: 23px; font-variant-numeric: tabular-nums; }
.release-row { align-items: flex-start; padding: 18px 0; border-bottom: 1px solid rgb(var(--base-text-color) / .1); }
.release-row:first-child { padding-top: 0; }
.release-copy { min-width: 0; flex: 1; overflow-wrap: anywhere; }
.release-title { justify-content: flex-start; flex-wrap: wrap; gap: 8px; }
.release-copy p { margin: 7px 0; }
.release-copy summary { cursor: pointer; padding: 6px 0; }
:global(.n-card.update-modal) { width: 720px; max-width: calc(100vw - 28px); }
.update-form { display: flex; flex-direction: column; gap: 18px; }
.update-form p { margin: -6px 0 0; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px; }
.form-grid :deep(.n-input-number), .form-grid :deep(.n-date-picker) { width: 100%; }
.fixed-message, .player-preview { background: rgb(var(--base-text-color) / .04); padding: 16px; border-radius: 8px; }
.player-preview { margin-bottom: 16px; }
.player-preview h3 { margin-top: 0; font-size: 17px; }
.player-content { white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.7; }
.modal-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
@media (max-width: 640px) {
  .update-page .ops-filters, .policy-heading, .release-row { flex-direction: column; align-items: stretch; }
  .policy-details, .delivery-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
  .form-grid { grid-template-columns: minmax(0, 1fr); }
  .version-name { font-size: 19px; }
  .modal-actions .n-button { flex: 1 0 auto; }
}
</style>
