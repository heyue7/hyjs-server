<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NButton, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
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
function emptyDraft(): ClientUpdateDraft { return { id: 0, version: '', buildCode: 0, versionName: '', title: '', content: '', mode: 'notice', minLoginBuild: 0, minRunBuild: 0, maintenanceStartAt: 0, maintenanceEndAt: 0, reason: '' }; }
const dirty = computed(() => editor.value && JSON.stringify(draft) !== baseline.value);
const canEdit = computed(() => Boolean(overview.value?.canEdit));
const canPublish = computed(() => Boolean(overview.value?.canPublish));
const blocked = computed(() => busy.value || loading.value || conflict.value || Boolean(pending.value));
const modes = [{ label: '仅提醒更新', value: 'notice' }, { label: '下次登录要求更新', value: 'next_login' }, { label: '维护更新', value: 'maintenance' }];
const modeName = (mode: UpdateMode) => modes.find(item => item.value === mode)?.label || '未发布';
const buildThreshold = (value: number) => value > 0 ? String(value) : '不限';
const deliveryMetrics = computed(() => {
  if (!overview.value) return [];
  const { policy, stats } = overview.value;
  return [
    { label: '在线连接', value: stats.online },
    { label: '已展示', value: policy.noticeEnabled ? stats.shown : '—' },
    { label: '已确认', value: policy.noticeEnabled ? stats.acknowledged : '—' },
    { label: '待送达', value: policy.noticeEnabled ? stats.pending : '—' }
  ];
});
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
  if (!row) { values.buildCode = policy.latestBuild; values.minLoginBuild = policy.minLoginBuild; values.minRunBuild = policy.minRunBuild; }
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
const inspecting = ref<ClientRelease | null>(null);
const historyColumns = computed<DataTableColumns<ClientRelease>>(() => [
  { title: '版本 / 提醒标题', key: 'title', minWidth: 230, render: row => h('div', { class: 'release-identity' }, [h('strong', row.versionName), h('span', { class: 'ops-muted' }, row.title)]) },
  { title: '构建号', key: 'buildCode', width: 135 },
  { title: '更新方式', key: 'mode', width: 165, render: row => modeName(row.mode) },
  { title: '状态', key: 'state', width: 110, render: row => h(NTag, { size: 'small', bordered: false, type: row.state === 'draft' ? 'default' : 'info' }, () => stateName(row.state)) },
  { title: '更新时间', key: 'updatedAt', width: 185, render: row => formatTime(row.publishedAt || row.updatedAt) },
  { title: '操作', key: 'actions', width: 240, render: row => h(NSpace, { size: 8, wrap: false }, () => [
    h(NButton, { size: 'small', quaternary: true, onClick: () => inspecting.value = row }, () => '详情'),
    row.state === 'draft' && canEdit.value ? h(NButton, { size: 'small', disabled: blocked.value, onClick: () => edit(row) }, () => '编辑') : null,
    row.state === 'draft' && canPublish.value ? h(NButton, { size: 'small', type: 'primary', disabled: blocked.value, onClick: () => prepareAction('publish', row) }, () => '预览发布') : null
  ]) }
]);
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
    <div class="update-toolbar">
      <p class="ops-muted">更新策略适用于同一客户端；维护仅影响当前接入区服。</p>
      <NSpace :wrap="false">
        <NButton :loading="loading" :disabled="busy || !!pending" @click="load">刷新</NButton>
        <NButton v-if="canEdit" type="primary" :disabled="blocked" @click="edit()">新建草稿</NButton>
      </NSpace>
    </div>
    <NAlert v-if="error" type="error" :title="error" />
    <NAlert v-if="conflict" type="warning" title="策略已更新，请关闭窗口后刷新。当前输入已保留。" />
    <NSpin :show="loading">
      <div v-if="overview" class="update-content">
        <section class="update-panel" aria-labelledby="current-policy-title">
          <div class="section-heading">
            <h2 id="current-policy-title">当前版本</h2>
            <NSpace size="small">
              <NTag size="small" :type="overview.policy.maintenance ? 'error' : 'success'">{{ overview.policy.maintenance ? '维护中' : '开放中' }}</NTag>
              <NTag size="small" :type="overview.policy.noticeEnabled ? 'info' : 'default'">{{ overview.policy.noticeEnabled ? '提醒已开启' : '提醒已关闭' }}</NTag>
            </NSpace>
          </div>
          <div class="policy-heading">
            <div class="policy-version">
              <strong class="version-name">{{ overview.policy.versionName || '尚未发布更新' }}</strong>
              <p v-if="overview.policy.releaseId" class="ops-muted">构建 {{ overview.policy.latestBuild }} · {{ modeName(overview.policy.mode) }}</p>
              <p v-else class="ops-muted">保存草稿并确认发布后，玩家才会收到更新提醒。</p>
            </div>
            <NSpace v-if="canPublish && overview.policy.releaseId" class="policy-actions">
              <NButton :disabled="blocked" @click="prepareAction('remind')">再次提醒</NButton>
              <NButton :disabled="blocked || !overview.policy.noticeEnabled" @click="prepareAction('withdraw')">撤回提醒</NButton>
              <NButton v-if="!overview.policy.maintenance && overview.policy.mode === 'maintenance'" type="warning" :disabled="blocked" @click="prepareAction('start_maintenance')">开始维护</NButton>
              <NButton v-if="overview.policy.maintenance" type="warning" :disabled="blocked" @click="prepareAction('end_maintenance')">结束维护</NButton>
            </NSpace>
          </div>
          <dl class="policy-summary">
            <div><dt>最低登录构建</dt><dd>{{ buildThreshold(overview.policy.minLoginBuild) }}</dd></div>
            <div><dt>最低运行构建</dt><dd>{{ buildThreshold(overview.policy.minRunBuild) }}</dd></div>
            <div><dt>提醒批次</dt><dd>{{ overview.policy.noticeBatch || '—' }}</dd></div>
            <div><dt>策略修订</dt><dd>{{ overview.policy.revision }}</dd></div>
          </dl>
          <p v-if="overview.policy.maintenanceStartAt" class="maintenance-note">维护 {{ formatTime(overview.policy.maintenanceStartAt) }} 至 {{ formatTime(overview.policy.maintenanceEndAt) }}（预计）；结束后须手动开放。</p>
          <p v-if="!canPublish" class="ops-muted permission-note">发布、提醒和维护操作需要管理员权限。</p>
        </section>
        <section class="update-panel delivery-panel" aria-labelledby="delivery-title">
          <div class="section-heading delivery-heading"><h2 id="delivery-title">提醒送达</h2><span class="ops-muted">{{ overview.policy.noticeEnabled ? '当前提醒批次' : '提醒未开启' }}</span></div>
          <dl class="delivery-grid"><div v-for="metric in deliveryMetrics" :key="metric.label"><dt>{{ metric.label }}</dt><dd>{{ metric.value }}</dd></div></dl>
          <div class="delivery-footer"><span>旧版 {{ overview.policy.releaseId ? overview.stats.older : '—' }} · 版本未知 {{ overview.stats.unknown }}</span><span>本服连接快照 · {{ formatTime(overview.stats.observedAt) }}</span></div>
        </section>
        <section class="update-panel history-panel" aria-labelledby="history-title">
          <div class="section-heading"><h2 id="history-title">草稿与发布记录 <span v-if="total" class="record-count">{{ total }}</span></h2></div>
          <NAlert v-if="historyError" type="error" :title="historyError" class="mb-16px"><NButton size="small" @click="loadHistory">重试加载历史</NButton></NAlert>
          <NDataTable :columns="historyColumns" :data="history" :row-key="(row: ClientRelease) => row.id" :bordered="false" :scroll-x="1065">
            <template #empty>暂无草稿或发布记录</template>
          </NDataTable>
          <div v-if="total > 20" class="ops-pagination"><NPagination v-model:page="page" :page-size="20" :item-count="total" simple @update:page="loadHistory" /></div>
        </section>
      </div>
    </NSpin>
    <NModal :show="!!inspecting" preset="card" title="更新记录详情" class="update-modal" @update:show="!$event && (inspecting = null)">
      <template v-if="inspecting">
        <NDescriptions :column="1" label-placement="left" class="mb-20px">
          <NDescriptionsItem label="版本">{{ inspecting.versionName }} · 构建 {{ inspecting.buildCode }}</NDescriptionsItem>
          <NDescriptionsItem label="状态">{{ stateName(inspecting.state) }}</NDescriptionsItem>
          <NDescriptionsItem label="更新方式">{{ modeName(inspecting.mode) }}</NDescriptionsItem>
          <NDescriptionsItem label="最低登录构建">{{ buildThreshold(inspecting.minLoginBuild) }}</NDescriptionsItem>
          <NDescriptionsItem label="最低运行构建">{{ buildThreshold(inspecting.minRunBuild) }}</NDescriptionsItem>
          <NDescriptionsItem label="更新时间">{{ formatTime(inspecting.publishedAt || inspecting.updatedAt) }}</NDescriptionsItem>
          <NDescriptionsItem v-if="inspecting.publishedBy" label="发布人">{{ inspecting.publishedBy }}</NDescriptionsItem>
          <NDescriptionsItem v-if="inspecting.mode === 'maintenance'" label="维护时间">{{ formatTime(inspecting.maintenanceStartAt) }} 至 {{ formatTime(inspecting.maintenanceEndAt) }}（预计）</NDescriptionsItem>
        </NDescriptions>
        <div class="player-preview"><h3>{{ inspecting.title }}</h3><p class="player-content">{{ inspecting.content }}</p><p>请前往QQ群下载最新版</p></div>
      </template>
      <template #footer><div class="modal-actions"><NButton @click="inspecting = null">关闭</NButton></div></template>
    </NModal>
    <NModal :show="editor" preset="card" :title="preview ? '草稿预览' : draft.id ? '编辑更新草稿' : '新建更新草稿'" class="update-modal" :mask-closable="false" :closable="!busy" @update:show="!$event && closeEditor()">
      <NAlert v-if="formError" type="error" :title="formError" class="mb-16px" />
      <NForm v-if="!preview" label-placement="top" :show-feedback="false" :disabled="busy" class="update-form">
        <div class="form-grid"><NFormItem label="版本名称" required><NInput v-model:value="draft.versionName" :maxlength="80" placeholder="例如 1.0.0" :input-props="{ 'aria-label': '版本名称' }" /></NFormItem><NFormItem label="最新构建号" required><NInputNumber v-model:value="draft.buildCode" :input-props="{ 'aria-label': '最新构建号' }" :min="1" :max="2100000000" :precision="0" :show-button="false" /></NFormItem></div>
        <NFormItem label="更新方式"><NSelect v-model:value="draft.mode" :options="modes" /></NFormItem>
        <div class="form-grid"><NFormItem label="最低登录构建"><NInputNumber v-model:value="draft.minLoginBuild" :input-props="{ 'aria-label': '最低登录构建' }" :min="0" :precision="0" :show-button="false" /></NFormItem><NFormItem label="最低运行构建"><NInputNumber v-model:value="draft.minRunBuild" :input-props="{ 'aria-label': '最低运行构建' }" :min="0" :precision="0" :show-button="false" /></NFormItem></div>
        <p class="ops-muted">0 表示不限。仅提醒保留当前门槛；其他方式会自动填入对应构建号。发布不会降低已有门槛。</p>
        <div v-if="draft.mode === 'maintenance'" class="form-grid"><NFormItem label="维护开始时间" required><NDatePicker :value="draft.maintenanceStartAt || null" type="datetime" @update:value="draft.maintenanceStartAt = $event || 0" /></NFormItem><NFormItem label="预计结束时间" required><NDatePicker :value="draft.maintenanceEndAt || null" type="datetime" @update:value="draft.maintenanceEndAt = $event || 0" /></NFormItem></div>
        <NFormItem label="提醒标题" required><NInput v-model:value="draft.title" :input-props="{ 'aria-label': '提醒标题' }" :maxlength="80" /></NFormItem><NFormItem label="更新说明" required><NInput v-model:value="draft.content" :input-props="{ 'aria-label': '更新说明' }" type="textarea" :maxlength="4000" :autosize="{ minRows: 4, maxRows: 10 }" /></NFormItem>
        <div class="fixed-message">玩家固定提示：请前往QQ群下载最新版</div>
        <NFormItem label="操作原因" required><NInput v-model:value="draft.reason" :input-props="{ 'aria-label': '操作原因' }" :maxlength="300" placeholder="仅供后台审计记录" /></NFormItem>
      </NForm>
      <template v-else><div class="player-preview"><h3>{{ draft.title }}</h3><p class="player-content">{{ draft.content }}</p><p>请前往QQ群下载最新版</p></div><p>{{ draft.versionName }} · {{ modeName(draft.mode) }} · 构建 {{ draft.buildCode }}</p><p>登录 ≥ {{ draft.minLoginBuild }} · 运行 ≥ {{ draft.minRunBuild }}</p><p v-if="draft.mode === 'maintenance'">维护 {{ formatTime(draft.maintenanceStartAt) }} 至 {{ formatTime(draft.maintenanceEndAt) }}（预计，须手动开放）</p><NAlert type="info" title="保存后仅生成草稿。管理员预览并确认发布后，玩家才会收到提醒。" /></template>
      <template #footer><div class="modal-actions"><NButton :disabled="busy" @click="closeEditor">取消</NButton><NButton v-if="preview" :disabled="busy" @click="preview = false">返回修改</NButton><NButton v-if="!preview" type="primary" @click="showPreview">预览草稿</NButton><NButton v-else type="primary" :loading="busy" :disabled="conflict || !canEdit" @click="save">保存草稿</NButton></div></template>
    </NModal>
    <NModal :show="actionOpen" preset="card" :title="actionNames[action]" class="update-modal" :mask-closable="false" :closable="!busy && !pending" @update:show="!$event && closeAction()">
      <NAlert v-if="actionError" type="error" :title="actionError" class="mb-16px" />
      <template v-if="selected"><div class="player-preview"><h3>{{ selected.title }}</h3><p class="player-content">{{ selected.content }}</p><p>请前往QQ群下载最新版</p></div><p>{{ selected.versionName }} · 构建 {{ selected.buildCode }}</p><NAlert :type="action === 'start_maintenance' || action === 'end_maintenance' ? 'warning' : 'info'" :title="actionEffect" class="mb-16px" /><NFormItem label="操作原因" required><NInput v-model:value="actionReason" :input-props="{ 'aria-label': '操作原因' }" :disabled="busy || !!pending" :maxlength="300" placeholder="填写本次操作原因" /></NFormItem><NCheckbox v-model:checked="confirmed" :disabled="busy || !!pending">已核对上述版本、玩家文案和操作后果</NCheckbox><p v-if="pending" class="ops-muted">结果尚未确认。重试沿用原请求，不会重复执行；请保留此窗口。</p></template>
      <template #footer><div class="modal-actions"><NButton :disabled="busy || !!pending" @click="closeAction">取消</NButton><NButton :type="action === 'start_maintenance' ? 'warning' : 'primary'" :loading="busy" :disabled="!confirmed || conflict || !canPublish" @click="execute">{{ pending ? '重试原请求' : `确认${actionNames[action]}` }}</NButton></div></template>
    </NModal>
  </div>
</template>

<style scoped>
:deep(.release-identity) { display: flex; flex-direction: column; gap: 4px; overflow-wrap: anywhere; }
.update-page { padding-bottom: 24px; color: rgb(var(--base-text-color)); }
.update-page :deep(.n-button--default-type) { color: rgb(var(--base-text-color)); }
.update-toolbar, .section-heading, .policy-heading, .delivery-footer, .release-row, .release-title { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.update-toolbar { min-height: 36px; }
.update-toolbar p { margin: 0; line-height: 1.6; }
.update-content { display: grid; grid-template-columns: minmax(0, 1.45fr) minmax(360px, 1fr); gap: 16px; align-items: stretch; }
.history-panel { grid-column: 1 / -1; }
.delivery-panel { display: flex; flex-direction: column; }
.update-panel { padding: 20px; border-radius: 6px; background: rgb(var(--container-bg-color)); }
.section-heading { margin-bottom: 18px; gap: 12px; }
.section-heading h2 { margin: 0; font-size: 16px; line-height: 1.5; font-weight: 600; }
.policy-heading { align-items: flex-start; }
.version-name { font-size: 20px; font-weight: 600; line-height: 1.4; }
.policy-version p { margin: 6px 0 0; }
.policy-actions { flex-shrink: 0; }
.policy-summary, .delivery-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 24px; margin: 20px 0 0; }
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
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
.form-grid :deep(.n-input-number), .form-grid :deep(.n-date-picker) { width: 100%; }
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
