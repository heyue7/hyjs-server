<script setup lang="ts">
import { computed, h, onBeforeUnmount, reactive, ref, watch } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NButton, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { useOperations } from '@/composables/useOperations';
import { errorMessage, formatTime } from '@/service/operations';
import { createCDKs, getCDKClaims, getCDKs, updateCDK } from '@/service/cdk';
import type { CDK, CDKClaim, CDKDraft } from '@/service/cdk';
import RewardList from '@/components/operations/reward-list.vue';
import RewardSummary from '@/components/operations/reward-summary.vue';
const scope = useServerScopeStore();
const { canWrite } = useOperations();
const writable = computed(() => canWrite.value && scope.currentServer?.runtimeBound && scope.currentServer.enabled);
const q = ref(''); const page = ref(1); const pageSize = ref(25); const total = ref(0);
const rows = ref<CDK[]>([]); const loading = ref(false); const error = ref('');
let listGeneration = 0;
async function load() {
  const current = ++listGeneration; rows.value = []; total.value = 0;
  if (!scope.serverId || !scope.currentServer?.runtimeBound) return;
  loading.value = true; error.value = '';
  try { const result = await getCDKs(scope.serverId, q.value.trim(), page.value, pageSize.value); if (current === listGeneration) { rows.value = result.items; total.value = result.total; } }
  catch (e) { if (current === listGeneration) error.value = errorMessage(e); }
  finally { if (current === listGeneration) loading.value = false; }
}
function search() { page.value = 1; load(); }
const editorOpen = ref(false); const selected = ref<CDK | null>(null); const saving = ref(false); const saveError = ref('');
const inputMode = ref('batch');
const draft = reactive<CDKDraft>({ name: '', rewards: [], count: 1, prefix: '', code: '', totalLimit: 1, unlimited: true, startAt: 0, endAt: 0, enabled: true, reason: '' });
const rewardsFrozen = computed(() => Boolean(selected.value?.used));
const minimumTotalLimit = computed(() => Math.max(1, selected.value?.used || 0));
const limitedTotalLimit = ref(1);
const noTotalLimit = computed({
  get: () => draft.totalLimit === 0,
  set: (checked: boolean) => {
    if (checked) {
      if (draft.totalLimit > 0) limitedTotalLimit.value = draft.totalLimit;
      draft.totalLimit = 0;
    } else {
      draft.totalLimit = Math.max(minimumTotalLimit.value, limitedTotalLimit.value);
    }
  }
});
const baseline = ref('');
const dirty = computed(() => editorOpen.value && baseline.value !== JSON.stringify({ draft, inputMode: inputMode.value }));
function edit(row: CDK | null = null) {
  if (!writable.value) return;
  selected.value = row ? { ...row, rewards: row.rewards.map(value => ({ ...value })) } : null;
  inputMode.value = 'batch'; saveError.value = '';
  Object.assign(draft, { name: row?.name || '', rewards: row?.rewards.map(value => ({ ...value })) || [], count: 1, prefix: '', code: '', totalLimit: row?.totalLimit ?? 1, unlimited: row?.unlimited ?? true, startAt: row?.startAt || 0, endAt: row?.endAt || 0, enabled: row?.enabled ?? true, reason: '' });
  limitedTotalLimit.value = Math.max(minimumTotalLimit.value, draft.totalLimit);
  editorOpen.value = true; baseline.value = JSON.stringify({ draft, inputMode: inputMode.value });
}
function confirmAction(title: string, content: string, positiveText: string) {
  return new Promise<boolean>(resolve => {
    if (!window.$dialog) return resolve(false);
    window.$dialog.warning({ title, content, positiveText, negativeText: '返回', onPositiveClick: () => resolve(true), onNegativeClick: () => resolve(false), onClose: () => resolve(false), onMaskClick: () => resolve(false) });
  });
}
async function closeEditor() {
  if (saving.value) return;
  if (dirty.value && !await confirmAction('放弃未保存的兑换码设置？', '已创建的兑换码与兑换记录不受影响。', '放弃修改')) return;
  editorOpen.value = false; draft.code = '';
}
function validate() {
  if (!draft.name.trim()) return '请填写兑换码名称';
  if (!draft.rewards.length || draft.rewards.some(row => !row.resourceId || !Number.isSafeInteger(row.amount) || row.amount < 1)) return '请配置完整的奖励与正整数数量';
  if (!Number.isSafeInteger(draft.totalLimit) || draft.totalLimit < 0) return '请选择不限总次数，或填写正整数上限';
  if (draft.totalLimit > 0 && draft.totalLimit < minimumTotalLimit.value) return `总次数不能少于已兑换的 ${selected.value?.used || 0} 次`;
  if (!selected.value && inputMode.value === 'batch' && (!Number.isSafeInteger(draft.count) || draft.count < 1 || draft.count > 500)) return '每批可生成1–500枚兑换码';
  if (!selected.value && inputMode.value === 'batch' && !/^[A-Za-z0-9]{0,12}$/.test(draft.prefix || '')) return '前缀可用最多12位字母或数字';
  if (!selected.value && inputMode.value === 'custom' && !draft.code?.trim()) return '请填写自定义兑换码';
  if (!draft.unlimited && (!draft.startAt || !draft.endAt || draft.endAt <= draft.startAt)) return '请选择有效的开始与结束时间';
  if (!draft.reason.trim()) return '请填写操作原因';
  return '';
}
const generatedCodes = ref<string[]>([]); const codesOpen = ref(false); const codesSaved = ref(false); const codesServerName = ref('');
async function save() {
  if (!writable.value || saving.value || !scope.serverId) return;
  saveError.value = validate(); if (saveError.value) return;
  saving.value = true;
  try {
    const common = { name: draft.name.trim(), rewards: draft.rewards, totalLimit: draft.totalLimit, unlimited: draft.unlimited, startAt: draft.unlimited ? 0 : draft.startAt, endAt: draft.unlimited ? 0 : draft.endAt, enabled: draft.enabled, reason: draft.reason.trim() };
    if (selected.value) { await updateCDK(scope.serverId, selected.value.id, { ...common, version: selected.value.version }); window.$message?.success('兑换码设置已保存'); }
    else {
      const result = await createCDKs(scope.serverId, { ...common, count: inputMode.value === 'custom' ? 1 : draft.count, ...(inputMode.value === 'custom' ? { code: draft.code!.trim() } : { prefix: draft.prefix }) });
      generatedCodes.value = result.codes; codesServerName.value = scope.currentServer?.name || `${scope.serverId}区`; codesSaved.value = false; codesOpen.value = true;
    }
    editorOpen.value = false; draft.code = ''; await load();
  } catch (e) { saveError.value = errorMessage(e); }
  finally { saving.value = false; }
}
async function copyCodes() {
  try { await navigator.clipboard.writeText(generatedCodes.value.join('\n')); window.$message?.success('兑换码已复制，请妥善保存'); }
  catch { window.$message?.error('复制失败，请选择下方兑换码手动复制或下载文本'); }
}
function downloadCodes() {
  const url = URL.createObjectURL(new Blob([generatedCodes.value.join('\n') + '\n'], { type: 'text/plain;charset=utf-8' }));
  const link = document.createElement('a'); link.href = url; link.download = `兑换码-${new Date().toISOString().slice(0,10)}.txt`; link.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
}
async function closeCodes() {
  if (!codesSaved.value && !await confirmAction('尚未确认保存兑换码', '关闭后无法再次查看完整兑换码，列表只保留尾号。', '仍然关闭')) return false;
  codesOpen.value = false; generatedCodes.value = []; return true;
}
const claimsOpen = ref(false); const claimsTarget = ref<CDK | null>(null); const claims = ref<CDKClaim[]>([]); const claimsPage = ref(1); const claimsTotal = ref(0); const claimsLoading = ref(false); const claimsError = ref('');
let claimsGeneration = 0;
async function loadClaims() {
  const current = ++claimsGeneration;
  if (!claimsTarget.value || !scope.serverId) return;
  claimsLoading.value = true; claimsError.value = '';
  try { const result = await getCDKClaims(scope.serverId, claimsTarget.value.id, claimsPage.value, 25); if (current === claimsGeneration) { claims.value = result.items; claimsTotal.value = result.total; } }
  catch (e) { if (current === claimsGeneration) claimsError.value = errorMessage(e); }
  finally { if (current === claimsGeneration) claimsLoading.value = false; }
}
function viewClaims(row: CDK) { claimsTarget.value = row; claims.value = []; claimsTotal.value = 0; claimsPage.value = 1; claimsOpen.value = true; loadClaims(); }
const columns: DataTableColumns<CDK> = [
  { title: '兑换码', key: 'name', minWidth: 200, render: row => h('div', { class: 'cdk-name' }, [h('strong', row.name), h('div', { class: 'ops-mono ops-muted' }, row.codeHint)]) },
  { title: '奖励', key: 'rewards', minWidth: 260, render: row => h(RewardSummary, { rewards: row.rewards }) },
  { title: '兑换次数', key: 'used', width: 170, render: row => h('div', { class: 'cdk-usage' }, [h('span', `已兑换 ${row.used.toLocaleString()} 次`), h('span', { class: 'ops-muted' }, row.totalLimit === 0 ? '不限总次数' : `上限 ${row.totalLimit.toLocaleString()} 次`)]) },
  { title: '有效期', key: 'startAt', minWidth: 230, render: row => row.unlimited ? '不限有效期' : `${formatTime(row.startAt)} 至 ${formatTime(row.endAt)}` },
  { title: '状态', key: 'enabled', width: 110, render: row => h(NTag, { size: 'small', bordered: false, type: row.enabled ? 'success' : 'default' }, () => row.enabled ? '已启用' : '已停用') },
  { title: '操作', key: 'actions', width: 190, render: row => h(NSpace, { size: 6 }, () => [h(NButton, { size: 'small', onClick: () => viewClaims(row) }, () => '兑换记录'), ...(canWrite.value ? [h(NButton, { size: 'small', secondary: true, disabled: !writable.value, onClick: () => edit(row) }, () => '编辑')] : [])]) }
];
const claimColumns: DataTableColumns<CDKClaim> = [
  { title: '角色', key: 'playerName', minWidth: 180, render: row => row.playerName || '角色 #' + row.playerId },
  { title: '角色编号', key: 'playerId', width: 130 }, { title: '兑换时间', key: 'claimedAt', minWidth: 190, render: row => formatTime(row.claimedAt) }
];
async function canLeave() {
  if (saving.value) return false;
  if (codesOpen.value && !await closeCodes()) return false;
  if (dirty.value && !await confirmAction('离开兑换码设置？', '当前未保存的修改将丢弃。', '离开')) return false;
  return true;
}
const removeGuard = scope.addSwitchGuard(canLeave);
onBeforeRouteLeave(canLeave);
watch(() => scope.serverId, () => { editorOpen.value = false; claimsOpen.value = false; claimsGeneration += 1; page.value = 1; load(); }, { immediate: true });
function beforeUnload(event: BeforeUnloadEvent) { if (saving.value || dirty.value || codesOpen.value) { event.preventDefault(); event.returnValue = ''; } }
window.addEventListener('beforeunload', beforeUnload);
onBeforeUnmount(() => { removeGuard(); listGeneration += 1; claimsGeneration += 1; generatedCodes.value = []; draft.code = ''; window.removeEventListener('beforeunload', beforeUnload); });
</script>

<template>
  <div class="ops-page cdk-page">

    <p class="ops-inline-note cdk-note">同一码每个角色限兑一次。</p>
    <p v-if="!scope.currentServer?.runtimeBound" class="ops-inline-note cdk-note">本区服尚未部署，部署后可生成和使用兑换码。</p>
    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false" class="ops-panel cdk-list">
      <div class="ops-filters">
        <NInput
          v-model:value="q"
          class="ops-search"
          clearable
          placeholder="搜索名称或尾号"
          :maxlength="128"
          @keydown.enter="search"
        />
        <NButton @click="search">搜索</NButton>
        <span class="ops-muted">共 {{ total.toLocaleString() }} 项</span>
        <NSpace :size="8">
          <NButton :loading="loading" @click="load">刷新</NButton>
          <NButton v-if="canWrite" type="primary" :disabled="!writable" @click="edit()">生成兑换码</NButton>
        </NSpace>
      </div>
      <NDataTable :columns="columns" :data="rows" :loading="loading" :row-key="(row: CDK) => row.id" size="small" :bordered="false" :scroll-x="1190" />
      <div class="ops-pagination">
        <NPagination v-model:page="page" v-model:page-size="pageSize" :page-sizes="[25,50,100]" :item-count="total" show-size-picker @update:page="load" @update:page-size="search" />
      </div>
    </NCard>

    <NModal :show="editorOpen" preset="card" :title="selected ? '编辑兑换码 · ' + selected.codeHint : '生成兑换码'" class="ops-modal cdk-editor" :mask-closable="!saving" :closable="!saving" @update:show="!$event && closeEditor()">
      <NAlert v-if="saveError" type="error" :title="saveError" class="mb-16px" />
      <NForm label-placement="top" :disabled="saving" :show-feedback="false" class="cdk-form">
        <NFormItem label="名称" required><NInput v-model:value="draft.name" placeholder="例如：开服感谢礼" :maxlength="80" /></NFormItem>
        <template v-if="!selected">
          <NFormItem label="生成方式">
            <NRadioGroup v-model:value="inputMode"><NSpace :size="20"><NRadio value="batch">随机生成</NRadio><NRadio value="custom">自定义一枚</NRadio></NSpace></NRadioGroup>
          </NFormItem>
          <div v-if="inputMode === 'batch'" class="cdk-pair">
            <NFormItem label="生成数量" required><NInputNumber v-model:value="draft.count" :min="1" :max="500" :precision="0" /></NFormItem>
            <NFormItem label="前缀（可选）"><NInput v-model:value="draft.prefix" placeholder="最多12位字母或数字" :maxlength="12" /></NFormItem>
          </div>
          <NFormItem v-else label="自定义兑换码" required><NInput v-model:value="draft.code" placeholder="填写发给玩家的兑换码" autocomplete="off" /></NFormItem>
        </template>
        <NFormItem label="兑换奖励" required>
          <div class="cdk-rewards">
            <RewardList v-model:value="draft.rewards" :disabled="saving || rewardsFrozen" />
            <p v-if="rewardsFrozen" class="ops-inline-note cdk-field-note">已有兑换记录，奖励不可更改。</p>
          </div>
        </NFormItem>
        <div class="cdk-pair">
          <NFormItem label="兑换次数" required>
            <div class="cdk-limit">
              <NCheckbox v-model:checked="noTotalLimit">不限总次数</NCheckbox>
              <div v-if="!noTotalLimit" class="cdk-limit-input">
                <NInputNumber v-model:value="draft.totalLimit" :min="minimumTotalLimit" :precision="0" aria-label="每码总兑换次数" />
                <span>次 / 码</span>
              </div>
              <p class="ops-inline-note cdk-field-note">同一码每个角色限兑一次。</p>
            </div>
          </NFormItem>
          <NFormItem label="允许兑换"><NSwitch v-model:value="draft.enabled" /></NFormItem>
        </div>
        <NFormItem label="有效期"><NCheckbox v-model:checked="draft.unlimited">不限有效期</NCheckbox></NFormItem>
        <div v-if="!draft.unlimited" class="cdk-pair">
          <NFormItem label="开始时间（北京）" required><NDatePicker :value="draft.startAt || null" type="datetime" clearable @update:value="draft.startAt = $event || 0" /></NFormItem>
          <NFormItem label="结束时间（北京）" required><NDatePicker :value="draft.endAt || null" type="datetime" clearable @update:value="draft.endAt = $event || 0" /></NFormItem>
        </div>
        <NFormItem label="操作原因" required><NInput v-model:value="draft.reason" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" :maxlength="300" placeholder="填写发放用途或调整原因" /></NFormItem>
      </NForm>
      <template #footer>
        <div class="ops-drawer-actions"><NButton :disabled="saving" @click="closeEditor">取消</NButton><NButton type="primary" :loading="saving" @click="save">{{ selected ? '保存设置' : '生成兑换码' }}</NButton></div>
      </template>
    </NModal>

    <NModal :show="codesOpen" preset="card" title="保存兑换码" class="ops-modal cdk-codes" :mask-closable="false" :closable="false">
      <div class="cdk-result-meta"><span>{{ codesServerName }}</span><span>已生成 {{ generatedCodes.length }} 枚</span></div>
      <p class="ops-inline-note cdk-result-warning">完整码仅显示这一次，关闭后无法再查看。</p>
      <NSpace :size="8" class="mb-12px"><NButton @click="copyCodes">复制全部</NButton><NButton @click="downloadCodes">下载文本</NButton></NSpace>
      <NInput :value="generatedCodes.join('\n')" class="cdk-code-input" type="textarea" readonly :autosize="{ minRows: 3, maxRows: 12 }" aria-label="本次生成的完整兑换码" />
      <NCheckbox v-model:checked="codesSaved" class="mt-16px">我已保存这些兑换码</NCheckbox>
      <template #footer><div class="ops-drawer-actions"><NButton type="primary" @click="closeCodes">完成</NButton></div></template>
    </NModal>

    <NDrawer v-model:show="claimsOpen" :width="760" class="cdk-claims">
      <NDrawerContent :title="(claimsTarget?.name || '') + ' · 兑换记录'" closable>
        <NAlert v-if="claimsError" type="error" :title="claimsError" class="mb-16px" />
        <NDataTable :columns="claimColumns" :data="claims" :loading="claimsLoading" size="small" :bordered="false" :scroll-x="500" />
        <div class="ops-pagination"><NPagination v-model:page="claimsPage" :item-count="claimsTotal" :page-size="25" @update:page="loadClaims" /></div>
        <template #footer><NButton @click="claimsOpen = false">关闭</NButton></template>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>

<style scoped>
:global(.n-card.cdk-editor), :global(.n-card.cdk-codes) { width: 760px; max-width: calc(100vw - 32px); }
:global(.n-drawer.cdk-claims) { max-width: 100vw; }
.cdk-page { gap: 14px; }
.cdk-note { margin: 0; }

.cdk-list :deep(.ops-filters) { margin-bottom: 16px; }
.cdk-list :deep(.ops-pagination) { margin-top: 18px; }
:deep(.cdk-name), :deep(.cdk-usage) { display: flex; flex-direction: column; gap: 3px; line-height: 1.5; }
:deep(.cdk-name strong) { font-weight: 600; }
:deep(.cdk-usage) { font-variant-numeric: tabular-nums; }
.cdk-form { display: flex; flex-direction: column; gap: 16px; }
.cdk-pair { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 16px; }
.cdk-pair :deep(.n-input-number), .cdk-pair :deep(.n-date-picker), .cdk-rewards, .cdk-limit { width: 100%; }
.cdk-limit { display: flex; flex-direction: column; align-items: flex-start; gap: 10px; }
.cdk-limit-input { display: flex; align-items: center; gap: 8px; width: 100%; }
.cdk-limit-input > span { white-space: nowrap; color: rgb(var(--base-text-color) / .65); font-size: 13px; }
.cdk-field-note { margin: 0; font-size: 13px; }
.cdk-rewards .cdk-field-note { margin-top: 8px; }
.cdk-result-meta { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 12px; margin-bottom: 8px; font-size: 13px; color: rgb(var(--base-text-color) / .65); }
.cdk-result-warning { margin-bottom: 16px; color: rgb(var(--base-text-color) / .85); }
.cdk-code-input :deep(textarea) { font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 13px; line-height: 1.7; }
@media (max-width: 600px) {
  .cdk-pair { grid-template-columns: minmax(0, 1fr); gap: 16px; }
  .cdk-page .ops-toolbar { flex-wrap: wrap; }
  .cdk-list :deep(.n-card__content) { padding: 16px; }
}
</style>
