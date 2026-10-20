<script setup lang="ts">
import { computed, h, onBeforeUnmount, reactive, ref, watch } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NButton, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { errorMessage, formatTime } from '@/service/operations';
import type { MailBatch, MailDraft } from '@/service/broadcast';
import { getMails, previewMail, sendMail } from '@/service/broadcast';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { useOperations } from '@/composables/useOperations';
import { createRequestId } from '@/utils/request-id';
import ContextHelp from '@/components/operations/context-help.vue';
import PlayerSelection from '@/views/broadcast/components/player-selection.vue';

const scope = useServerScopeStore();
const { canWrite } = useOperations();
const writable = computed(() => Boolean(canWrite.value && scope.serverId && scope.currentServer?.runtimeBound));
const page = ref(1); const pageSize = ref(25); const total = ref(0);
const rows = ref<MailBatch[]>([]); const loading = ref(false); const error = ref('');
let generation = 0;
async function load() {
  generation += 1;
  const current = generation; rows.value = []; total.value = 0; loading.value = false;
  if (!scope.serverId || !scope.currentServer?.runtimeBound) return;
  loading.value = true; error.value = '';
  try {
    const result = await getMails(scope.serverId, page.value, pageSize.value);
    if (current === generation) { rows.value = result.items; total.value = result.total; }
  } catch (e) { if (current === generation) error.value = errorMessage(e); }
  finally { if (current === generation) loading.value = false; }
}
const editorOpen = ref(false); const inspecting = ref(false); const busy = ref(false); const saveError = ref('');
const draft = reactive<MailDraft>({ audience: { mode: 'players', playerIds: [] }, title: '', body: '', reason: '' });
const baseline = ref(''); const stage = ref<'form' | 'preview' | 'result'>('form');
const batch = ref<MailBatch | null>(null); const confirmed = ref(false); const attemptedSend = ref(false);
const previewRequestId = ref(''); const sendRequestId = ref(''); const previewBaseline = ref('');
const dirty = computed(() => editorOpen.value && !inspecting.value && stage.value !== 'result' && JSON.stringify(draft) !== baseline.value);
function start() {
  if (!writable.value) return;
  Object.assign(draft, { audience: { mode: 'players', playerIds: [] }, title: '', body: '', reason: '' });
  baseline.value = JSON.stringify(draft); previewBaseline.value = ''; previewRequestId.value = ''; sendRequestId.value = '';
  batch.value = null; stage.value = 'form'; inspecting.value = false; confirmed.value = false; attemptedSend.value = false; saveError.value = ''; editorOpen.value = true;
}
function view(row: MailBatch) { batch.value = row; stage.value = 'result'; inspecting.value = true; saveError.value = ''; editorOpen.value = true; }
function audienceLabel(value: { audienceMode: string; recipientCount: number }) {
  return value.audienceMode === 'players' ? `指定玩家（${value.recipientCount} 名）` : '全部玩家（跨分页）';
}
function validate() {
  if (!draft.title.trim()) return '请填写邮件标题';
  if (!draft.body.trim()) return '请填写邮件正文';
  if (draft.audience.mode === 'players' && (!draft.audience.playerIds.length || draft.audience.playerIds.length > 5000)) return '请至少勾选一名玩家，单次指定玩家不超过 5000 名';
  if (!draft.reason.trim()) return '请填写操作原因';
  return '';
}
async function preview() {
  if (!writable.value || busy.value || !scope.serverId) return;
  saveError.value = validate(); if (saveError.value) return;
  const content = JSON.stringify(draft);
  if (!previewRequestId.value || previewBaseline.value !== content) { previewRequestId.value = createRequestId(); previewBaseline.value = content; }
  busy.value = true;
  try {
    const result = await previewMail(scope.serverId, previewRequestId.value, { audience: { ...draft.audience, playerIds: [...draft.audience.playerIds] }, title: draft.title.trim(), body: draft.body.trim(), reason: draft.reason.trim() });
    batch.value = result.batch; sendRequestId.value = createRequestId(); confirmed.value = false; attemptedSend.value = false; stage.value = 'preview';
  } catch (e) { saveError.value = errorMessage(e); }
  finally { busy.value = false; }
}
async function send() {
  if (!batch.value || !confirmed.value || busy.value || batch.value.recipientCount < 1 || !writable.value || scope.serverId !== batch.value.serverId) return;
  busy.value = true; saveError.value = ''; attemptedSend.value = true;
  try {
    // Preserve both UUID and frozen preview through uncertain network outcomes; retries return the same receipt.
    const result = await sendMail(batch.value.serverId, batch.value, sendRequestId.value);
    batch.value = result.batch; stage.value = 'result'; attemptedSend.value = false;
    window.$message?.success(`已向 ${result.batch.successCount} 个角色发送邮件`);
    await load();
  } catch (e) { saveError.value = errorMessage(e); }
  finally { busy.value = false; }
}
function backToForm() {
  if (busy.value || attemptedSend.value) return;
  stage.value = 'form'; batch.value = null; confirmed.value = false; previewRequestId.value = ''; saveError.value = '';
}
function askDiscard() {
  if (!window.$dialog) return Promise.resolve(false);
  return new Promise<boolean>(resolve => {
    window.$dialog!.warning({ title: attemptedSend.value ? '发送结果尚未确认' : '放弃当前邮件草稿？', content: attemptedSend.value ? '建议返回并使用原请求重试，以免重复发送。关闭后请先查看批次记录，确认结果后再新建。' : '尚未发送的草稿将被清除。', positiveText: '关闭', negativeText: '返回', onPositiveClick: () => resolve(true), onNegativeClick: () => resolve(false), onClose: () => resolve(false), onMaskClick: () => resolve(false) });
  });
}
async function closeEditor() {
  if (busy.value) return false;
  if ((dirty.value || attemptedSend.value) && !await askDiscard()) return false;
  editorOpen.value = false; return true;
}
async function canLeave() { return !editorOpen.value || await closeEditor(); }
const removeGuard = scope.addSwitchGuard(canLeave);
onBeforeRouteLeave(canLeave);
watch(() => scope.serverId, () => { editorOpen.value = false; page.value = 1; load(); }, { immediate: true });
function beforeUnload(event: BeforeUnloadEvent) { if (busy.value || dirty.value || attemptedSend.value) { event.preventDefault(); event.returnValue = ''; } }
window.addEventListener('beforeunload', beforeUnload);
onBeforeUnmount(() => { generation += 1; removeGuard(); window.removeEventListener('beforeunload', beforeUnload); });
const columns: DataTableColumns<MailBatch> = [
  { title: '邮件标题', key: 'title', minWidth: 220 },
  { title: '发送范围', key: 'audienceMode', width: 170, render: row => h(NTag, { size: 'small', bordered: false, type: row.audienceMode === 'players' ? 'info' : 'default' }, () => audienceLabel(row)) },
  { title: '发送数量', key: 'successCount', width: 130, render: row => `${row.successCount} / ${row.recipientCount} 个角色` },
  { title: '发送时间', key: 'sentAt', minWidth: 180, render: row => formatTime(row.sentAt) },
  { title: '操作人', key: 'actorName', minWidth: 110 },
  { title: '操作', key: 'actions', width: 90, fixed: 'right', render: row => h(NButton, { size: 'small', onClick: () => view(row) }, () => '查看') }
];
</script>

<template>
  <div class="ops-page mail-page">
    <div class="ops-toolbar">
      <div class="mail-heading"><div class="ops-page-label">发送邮件</div><ContextHelp title="发送范围" text="仅向当前所选真实区服、预览时冻结的角色发送原系统邮件。可选全部玩家（跨分页）或勾选部分玩家；冻结账号和保护账号自动排除，预览后新建角色不追加。邮件只写入游戏内邮箱，不创建领奖中心附件。预览15分钟有效，失败重试沿用原批次。" /></div>
      <NSpace :size="8"><NButton :loading="loading" @click="load">刷新</NButton><NButton v-if="canWrite" type="primary" :disabled="!writable" @click="start">新建邮件</NButton></NSpace>
    </div>
    <NAlert v-if="!scope.currentServer?.runtimeBound" type="warning" title="请选择已部署的真实区服后发送。" />
    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false">
      <NDataTable :columns="columns" :data="rows" :loading="loading" :row-key="(row: MailBatch) => row.id" :bordered="false" size="small" :scroll-x="1060" />
      <div class="ops-pagination"><NPagination v-model:page="page" v-model:page-size="pageSize" :page-sizes="[25,50,100]" :item-count="total" show-size-picker @update:page="load" @update:page-size="page = 1; load()" /></div>
    </NCard>
    <NModal :show="editorOpen" preset="card" :title="inspecting ? '邮件记录' : stage === 'form' ? '新建邮件' : stage === 'preview' ? '确认发送范围' : '发送完成'" class="mail-editor" :closable="!busy" :mask-closable="!busy" @update:show="!$event && closeEditor()">
      <NAlert v-if="saveError" type="error" :title="saveError" class="mb-16px" />
      <NForm v-if="stage === 'form'" label-placement="top" :disabled="busy" :show-feedback="false" class="mail-form">
        <div class="mail-realm">收件区服 <strong>{{ scope.currentServer?.name }}</strong></div>
        <NFormItem label="收件玩家" required><PlayerSelection v-model:value="draft.audience" :server-id="scope.serverId" :disabled="busy" /></NFormItem>
        <NFormItem label="邮件标题" required><NInput v-model:value="draft.title" :maxlength="60" placeholder="例如：版本更新说明" /></NFormItem>
        <NFormItem label="邮件正文" required><NInput v-model:value="draft.body" type="textarea" :maxlength="4000" :autosize="{ minRows: 3, maxRows: 6 }" placeholder="填写给玩家的邮件正文，将显示在原系统邮箱" /></NFormItem>
        <NFormItem label="操作原因" required><NInput v-model:value="draft.reason" :maxlength="300" placeholder="仅供后台操作记录使用" /></NFormItem>
      </NForm>
      <template v-else-if="batch">
        <div class="mail-review">
          <div><span>收件区服</span><strong>{{ batch.serverName }}</strong></div>
          <div><span>发送范围</span><strong>{{ audienceLabel(batch) }}</strong></div>
          <div><span>{{ stage === 'result' ? '成功发送' : '收件角色' }}</span><strong>{{ stage === 'result' ? batch.successCount : batch.recipientCount }} 个</strong></div>
          <div v-if="batch.excludedCount"><span>已排除冻结/保护角色</span><strong>{{ batch.excludedCount }} 个</strong></div>
          <div v-if="stage === 'preview'"><span>预览有效至</span><span>{{ formatTime(batch.expiresAt) }}</span></div>
          <div v-else><span>发送时间</span><span>{{ formatTime(batch.sentAt) }}</span></div>
        </div>
        <section class="mail-preview"><h3>{{ batch.title }}</h3><p>{{ batch.body }}</p></section>
        <div class="mail-reason"><span>操作原因</span><p>{{ batch.reason }}</p></div>
        <NAlert v-if="stage === 'preview' && batch.recipientCount < 1" type="warning" title="当前没有可发送的角色，本批次不能发送。" />
        <NAlert v-else-if="stage === 'preview'" type="info" title="确认后只写入原系统邮箱，不会创建领奖中心附件。" />
        <NCheckbox v-if="stage === 'preview'" v-model:checked="confirmed" :disabled="busy || batch.recipientCount < 1">确认向以上 {{ batch.recipientCount }} 个角色发送同一封邮件。</NCheckbox>
      </template>
      <template #footer><div class="ops-drawer-actions">
        <NButton :disabled="busy" @click="closeEditor">{{ stage === 'result' ? '关闭' : '取消' }}</NButton>
        <NButton v-if="stage === 'form'" type="primary" :loading="busy" @click="preview">预览发送</NButton>
        <template v-else-if="stage === 'preview'"><NButton :disabled="busy || attemptedSend" @click="backToForm">返回修改</NButton><NButton type="primary" :loading="busy" :disabled="!confirmed || !batch?.recipientCount" @click="send">{{ attemptedSend ? '重试原批次' : '确认发送' }}</NButton></template>
      </div></template>
    </NModal>
  </div>
</template>

<style scoped>
.mail-heading { display: flex; align-items: center; gap: 8px; }
:global(.n-card.mail-editor) { width: 780px; max-width: calc(100vw - 32px); }
.mail-form { display: flex; flex-direction: column; gap: 18px; }
.mail-realm { display: flex; gap: 16px; color: rgb(var(--base-text-color) / .7); }
.mail-realm strong { color: rgb(var(--base-text-color)); }
.mail-review { display: grid; grid-template-columns: 1fr 1fr; gap: 12px 24px; }
.mail-review > div { display: flex; flex-direction: column; gap: 5px; }
.mail-review > div > span:first-child, .mail-reason > span { color: rgb(var(--base-text-color) / .65); font-size: 13px; }
.mail-preview { border-block: 1px solid rgb(var(--base-text-color) / .1); padding: 18px 0; margin: 18px 0; }
.mail-preview h3 { margin: 0 0 10px; font-size: 16px; }
.mail-preview p, .mail-reason p { white-space: pre-wrap; overflow-wrap: anywhere; }
.mail-reason { margin-bottom: 18px; }
.mail-reason p { margin: 5px 0 0; }
@media (max-width: 600px) { .mail-page .ops-toolbar { flex-wrap: wrap; } .mail-review { grid-template-columns: 1fr; } }
</style>
