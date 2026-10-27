<script setup lang="ts">
import { computed, h, onBeforeUnmount, ref, watch } from 'vue';
import { NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { useRoute, useRouter } from 'vue-router';
import { api, ApiError, errorMessage, formatTime, json } from '@/service/operations';
import type { TerminalSettings, TerminalDistribution, TerminalDistributionResult } from '@/service/wallet-operations';
import { useOperations } from '@/composables/useOperations';
import { useDraftGuard } from '@/composables/useDraftGuard';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { createRequestId } from '@/utils/request-id';
const scope = useServerScopeStore();
const { canWrite } = useOperations();
const route = useRoute();
const router = useRouter();
// Preserve old user-specific wallet links; ordinary /wallets now opens operations.
if (route.query.playerId) {
  const query = { ...route.query }; query.wallet = query.playerId; delete query.playerId;
  void router.replace({ name: 'players', query });
}
const draft = ref<TerminalSettings | null>(null);
const saved = ref<TerminalSettings | null>(null);
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const conflict = ref(false);
const distributing = ref(false);
const points = ref<number | null>(null);
const reason = ref('');
const distributeError = ref('');
const result = ref<TerminalDistributionResult | null>(null);
const pending = ref<{ requestId: string; points: number; reason: string; version: string; serverId: number; serverName: string } | null>(null);
const records = ref<TerminalDistribution[]>([]);
const recordsLoading = ref(false);
const recordsError = ref('');
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
let generation = 0;
let recordGeneration = 0;
const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value));
const busy = computed(() => saving.value || distributing.value);
const settingsDirty = computed(() => JSON.stringify(draft.value) !== JSON.stringify(saved.value));
const guard = useDraftGuard({ active: () => Boolean(draft.value || pending.value), value: () => ({ settings: draft.value, pending: pending.value, points: points.value, reason: reason.value }), busy: () => busy.value || Boolean(pending.value), writable: () => canWrite.value, label: '终端设置或投放', close: () => { draft.value = saved.value ? clone(saved.value) : null; pending.value = null; points.value = null; reason.value = ''; } });
function addWeekly() { draft.value?.weekly.push({ id: createRequestId(), kind: 'weekly', time: '00:00', day: 15, date: '', weekday: 1, points: 648, enabled: true }); }
const scheduleKinds = [{ label: '每天', value: 'daily' }, { label: '每周', value: 'weekly' }, { label: '每月', value: 'monthly' }, { label: '指定日期', value: 'once' }];
const weekdays = ['周一', '周二', '周三', '周四', '周五', '周六', '周日'].map((label, i) => ({ label, value: i + 1 }));
const locked = computed(() => loading.value || busy.value || !canWrite.value || Boolean(pending.value));
async function loadSettings() {
  if (busy.value || pending.value) return;
  if (settingsDirty.value && !await guard.canLeave()) return;
  const seq = ++generation;
  loading.value = true; error.value = '';
  try {
    const data = await api<TerminalSettings>(scope.serverQuery('/terminal/settings'));
    if (seq !== generation) return;
    data.weekly.forEach(row => { row.kind ||= 'weekly'; row.time ||= '00:00'; });
    saved.value = clone(data); draft.value = clone(data); conflict.value = false; guard.capture();
  } catch (e) { if (seq === generation) error.value = errorMessage(e); }
  finally { if (seq === generation) loading.value = false; }
}
async function saveSettings() {
  const d = draft.value;
  if (!d || locked.value || conflict.value) return;
  const valid = (n: number) => Number.isSafeInteger(n) && n >= 0;
  if (![d.signupPoints, d.burstCostPoints, d.burstMinPoints, d.burstMaxPoints].every(valid) || d.burstMinPoints < 1 || d.burstMaxPoints < d.burstMinPoints || d.weekly.some(row => !valid(row.points) || row.points <= 0)) {
    error.value = '请填写有效点数，Burst 上限须不小于下限，定期投放须大于 0。'; return;
  }
  const invalid = d.weekly.findIndex(row => {
    if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(row.time || '00:00')) return true;
    if (row.kind === 'monthly') return !Number.isInteger(row.day) || (row.day || 0) < 1 || (row.day || 0) > 31;
    if (row.kind === 'once') { const value = row.date || ''; const date = new Date(`${value}T00:00:00Z`); return !/^\d{4}-\d{2}-\d{2}$/.test(value) || !Number.isFinite(date.getTime()) || date.toISOString().slice(0, 10) !== value; }
    return false;
  });
  if (invalid >= 0) { error.value = `请补全第 ${invalid + 1} 条定期投放的日期和时间。`; return; }
  const path = scope.serverQuery('/terminal/settings');
  saving.value = true; error.value = '';
  try { const data = await api<TerminalSettings>(path, json('PUT', clone(d))); saved.value = clone(data); draft.value = clone(data); guard.capture(); window.$message?.success('整组设置已保存'); }
  catch (e) { conflict.value = e instanceof ApiError && e.status === 409; error.value = conflict.value ? '终端设置已被其他操作更新。' : errorMessage(e); }
  finally { saving.value = false; }
}
function cancelDraft() { if (saved.value) draft.value = clone(saved.value); conflict.value = false; error.value = ''; }
async function loadRecords() {
  const seq = ++recordGeneration; recordsLoading.value = true; recordsError.value = '';
  try { const data = await api<{ items: TerminalDistribution[]; total: number }>(scope.serverQuery(`/terminal/distributions?page=${page.value}&pageSize=${pageSize.value}`)); if (seq === recordGeneration) { records.value = data.items || []; total.value = data.total; } }
  catch (e) { if (seq === recordGeneration) recordsError.value = errorMessage(e); }
  finally { if (seq === recordGeneration) recordsLoading.value = false; }
}
function confirmDistribution() {
  if (locked.value || conflict.value || !draft.value || !saved.value || settingsDirty.value || !points.value || !Number.isSafeInteger(points.value) || points.value <= 0 || !reason.value.trim() || !scope.currentServer || !window.$dialog) return;
  const request = { requestId: createRequestId(), points: points.value, reason: reason.value.trim(), version: saved.value.version, serverId: scope.currentServer.id, serverName: scope.currentServer.name };
  window.$dialog.warning({ title: '确认向当前区服投放', content: `区服：${request.serverName}（${request.serverId}）。每个符合条件的角色到账 ${request.points} 超频点数。原因：${request.reason}。包含离线角色，每服每账号 1 个角色，保护账号不参与。`, positiveText: '确认投放', negativeText: '取消', onPositiveClick: () => { if (scope.serverId !== request.serverId || busy.value || !canWrite.value) return; pending.value = request; void distribute(); } });
}
async function distribute() {
  const request = pending.value;
  if (!request || distributing.value || !canWrite.value || request.serverId !== scope.serverId) return;
  distributing.value = true; distributeError.value = ''; result.value = null;
  try {
    result.value = await api<TerminalDistributionResult>(scope.serverQuery('/terminal/distribute'), json('POST', { requestId: request.requestId, points: request.points, reason: request.reason, version: request.version }));
    pending.value = null; points.value = null; reason.value = ''; guard.capture(); await loadRecords();
  } catch (e) {
    if (e instanceof ApiError && e.status >= 400 && e.status < 500 && ![408, 429].includes(e.status)) {
      pending.value = null;
      conflict.value = e.status === 409;
      distributeError.value = `${conflict.value ? '终端设置已更新' : errorMessage(e)}。输入已保留。${conflict.value ? '请到终端设置重新读取当前版本，再确认投放。' : '请检查输入后重新确认投放。'}`;
    } else distributeError.value = `${errorMessage(e)}。请保留本次请求并重试，系统会复用请求编号核对结果。`;
  }
  finally { distributing.value = false; }
}
const statusNames: Record<string, string> = { completed: '已完成', complete: '已完成', applied: '已完成', pending: '待处理', processing: '处理中', blocked: '发放受阻', failed: '失败' };
const columns: DataTableColumns<TerminalDistribution> = [
  { title: '时间（北京）', key: 'createdAt', width: 185, render: row => formatTime(row.createdAt) },
  { title: '投放类型', key: 'kind', width: 120, render: row => ({ admin_manual: '单次投放', manual: '单次投放', admin_weekly: '定期投放', weekly: '定期投放' }[row.kind] || row.kind) },
  { title: '超频点数 / 角色', key: 'points', width: 160 },
  { title: '已到账 / 接收角色', key: 'credited', width: 155, render: row => `${row.credited} / ${row.recipients}` },
  { title: '状态', key: 'status', width: 110, render: row => h(NTag, { size: 'small', bordered: false }, () => statusNames[row.status] || row.status) },
  { title: '批次编号', key: 'batchId', minWidth: 240 }
];
watch(() => scope.serverId, () => { generation++; recordGeneration++; draft.value = null; saved.value = null; pending.value = null; points.value = null; reason.value = ''; result.value = null; error.value = ''; distributeError.value = ''; records.value = []; total.value = 0; page.value = 1; if (scope.currentServer) { void loadSettings(); void loadRecords(); } }, { immediate: true });
onBeforeUnmount(() => { generation++; recordGeneration++; });
</script>

<template>
  <div class="terminal-workspace">
    <h2 class="terminal-page-title">蓝绿终端</h2>
    <NTabs type="line" animated>
      <NTabPane name="settings" tab="终端设置">
        <div class="ops-toolbar mb-16px"><span class="ops-muted">{{ scope.currentServer?.name }} · 按区服保存</span><NButton :loading="loading" :disabled="busy || !!pending" @click="loadSettings">重新读取设置</NButton></div>
        <NAlert v-if="error" type="error" class="mb-16px">{{ error }}<template v-if="conflict"><br>设置已被更新。当前草稿保留，请重新读取后再修改。</template></NAlert>
        <NSpin :show="loading">
          <NForm v-if="draft" label-placement="top" :disabled="locked" @submit.prevent="saveSettings">
            <section class="terminal-section">
              <div class="terminal-section-heading"><h3>注册赠送</h3></div>
              <div class="terminal-single-field">
                <NFormItem label="首次注册赠送（超频点数）"><NInputNumber v-model:value="draft.signupPoints" :min="0" :precision="0" class="w-full" /></NFormItem>
              </div>
            </section>
            <section class="terminal-section">
              <div class="terminal-section-heading"><h3>Burst Link！</h3><NFormItem label="启用" class="terminal-switch"><NSwitch v-model:value="draft.burstEnabled" /></NFormItem></div>
              <div class="terminal-grid">
                <NFormItem label="Burst 消耗（超频点数）"><NInputNumber v-model:value="draft.burstCostPoints" :min="0" :precision="0" class="w-full" /></NFormItem>
                <NFormItem label="Burst 随机下限（超频点数）"><NInputNumber v-model:value="draft.burstMinPoints" :min="1" :precision="0" class="w-full" /></NFormItem>
                <NFormItem label="Burst 随机上限（超频点数）"><NInputNumber v-model:value="draft.burstMaxPoints" :min="1" :precision="0" class="w-full" /></NFormItem>
              </div>
            </section>
            <section class="terminal-section">
              <div class="terminal-section-heading"><div><h3>定期投放</h3><p class="ops-muted">各行独立设置北京时间；到点后投放，当天重启会补本日。历史日期不追发，月末不存在的日期跳过。玩家看到的理由按原随机规则生成。</p></div><NButton v-if="canWrite" :disabled="locked" @click="addWeekly">添加规则</NButton></div>
            <NEmpty v-if="!draft.weekly.length" description="暂无定期投放规则" class="py-20px" />
            <div v-for="(rule, index) in draft.weekly" :key="rule.id" class="terminal-rule">
              <div class="terminal-rule-number">{{ index + 1 }}</div>
              <NFormItem label="投放周期"><NSelect v-model:value="rule.kind" :options="scheduleKinds" :fallback-option="false" /></NFormItem>
              <NFormItem v-if="rule.kind === 'monthly'" label="每月几号"><NInputNumber v-model:value="rule.day" :min="1" :max="31" :precision="0" class="w-full" /></NFormItem>
              <NFormItem v-else-if="rule.kind === 'once'" label="投放日期"><NDatePicker :formatted-value="rule.date || null" @update:formatted-value="rule.date = $event || ''" type="date" value-format="yyyy-MM-dd" format="yyyy-MM-dd" :clearable="false" /></NFormItem>
              <NFormItem v-else-if="rule.kind === 'daily'" label="生效日期"><span>每天</span></NFormItem>
              <NFormItem v-else label="每周几"><NSelect v-model:value="rule.weekday" :options="weekdays" /></NFormItem>
              <NFormItem label="投放时间"><NTimePicker v-model:formatted-value="rule.time" format="HH:mm" value-format="HH:mm" :clearable="false" /></NFormItem>
              <NFormItem label="超频点数"><NInputNumber v-model:value="rule.points" :min="1" :precision="0" class="w-full" /></NFormItem>
              <NFormItem label="启用"><NSwitch v-model:value="rule.enabled" /></NFormItem>
              <NButton v-if="canWrite" :disabled="locked" quaternary type="error" @click="draft.weekly.splice(index, 1)">删除</NButton>
            </div>
            </section>
            <div v-if="canWrite" class="ops-drawer-actions mt-16px"><NButton :disabled="locked" @click="cancelDraft">取消草稿</NButton><NButton type="primary" attr-type="submit" :loading="saving" :disabled="locked || conflict || !settingsDirty">保存整组设置</NButton></div>
          </NForm>
        </NSpin>
      </NTabPane>
      <NTabPane name="distribute" tab="单次全服投放">
        <div class="terminal-scope"><span class="terminal-scope-label">投放区服</span><strong>{{ scope.currentServer?.name }}（{{ scope.serverId }}）</strong><p>向本区服非保护账号的角色投放，包含离线角色；每服每账号 1 个角色，余额按区服独立。</p></div>
        <NAlert v-if="settingsDirty && !pending" type="warning" class="mb-16px">请先保存或取消终端设置草稿，再提交投放。</NAlert>
        <NForm label-placement="top" :disabled="locked" class="terminal-distribute" @submit.prevent="confirmDistribution">
          <NFormItem label="每角色投放超频点数" required><NInputNumber v-model:value="points" :min="1" :precision="0" class="w-full" /></NFormItem>
          <NFormItem label="投放原因（玩家将看到这段原文）" required><NInput v-model:value="reason" type="textarea" :maxlength="200" show-count /></NFormItem>
          <NButton v-if="canWrite" attr-type="submit" type="primary" :disabled="locked || conflict || settingsDirty || !points || !reason.trim() || !saved">确认投放</NButton>
        </NForm>
        <NAlert v-if="pending" type="warning" class="mt-20px">本次请求结果待确认：{{ pending.points }} 超频点数 / 角色，{{ pending.reason }}。请求编号 {{ pending.requestId }}。切区或离页前请先重试确认结果。<div class="mt-12px"><NButton :loading="distributing" :disabled="!canWrite" @click="distribute">重试同一请求</NButton></div></NAlert>
        <NAlert v-if="distributeError" type="error" class="mt-16px">{{ distributeError }}</NAlert>
        <NAlert v-if="result" type="success" class="mt-16px">批次 {{ result.batchId }}：已到账 {{ result.credited }} / {{ result.recipients }} 个角色{{ result.alreadyApplied ? '（已核对原请求）' : '' }}。</NAlert>
      </NTabPane>
      <NTabPane name="records" tab="投放记录">
        <div class="ops-toolbar mb-16px"><span class="ops-muted">{{ scope.currentServer?.name }} · 单次与定期投放</span><NButton :loading="recordsLoading" @click="loadRecords">刷新记录</NButton></div>
        <NAlert v-if="recordsError" type="error" class="mb-16px">{{ recordsError }}</NAlert>
        <NDataTable :columns="columns" :data="records" :loading="recordsLoading" :bordered="false" :scroll-x="980" :row-key="(row: TerminalDistribution) => row.batchId" />
        <div class="ops-pagination"><NPagination v-model:page="page" v-model:page-size="pageSize" :item-count="total" :page-sizes="[20, 50, 100]" show-size-picker @update:page="loadRecords" @update:page-size="page = 1; loadRecords()" /></div>
      </NTabPane>
    </NTabs>
  </div>
</template>

<style scoped>
.terminal-workspace { max-width: 1120px; container-type: inline-size; }
.terminal-page-title { display: none; margin: 0 0 16px; font-size: 18px; }
.terminal-section { padding: 20px 0 4px; border-top: 1px solid rgb(var(--base-text-color) / .1); }
.terminal-section-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 16px; }
.terminal-section-heading h3 { margin: 0; font-size: 15px; font-weight: 600; }
.terminal-section-heading p { margin: 6px 0 0; line-height: 1.5; }
.terminal-section-heading :deep(.terminal-switch) { margin: 0; }
.terminal-section-heading :deep(.terminal-switch .n-form-item-blank) { min-height: 24px; }
.terminal-single-field { max-width: 340px; }
.terminal-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 0 20px; }
.terminal-rule { display: grid; grid-template-columns: 26px minmax(110px,1fr) minmax(140px,1.2fr) 140px minmax(130px,1fr) 60px 60px; gap: 14px; align-items: center; padding: 16px 0 0; border-top: 1px solid rgb(var(--base-text-color) / .1); }
.terminal-rule-number { color: var(--ops-muted, #84909c); font-variant-numeric: tabular-nums; align-self: center; margin-top: -20px; }
.terminal-rule :deep(.n-time-picker) { width: 100%; }
.terminal-scope { max-width: 700px; margin-bottom: 24px; padding: 16px 18px; border: 1px solid rgb(var(--base-text-color) / .1); border-radius: 8px; }
.terminal-scope-label { display: block; margin-bottom: 4px; opacity: .65; font-size: 12px; }
.terminal-scope strong { font-size: 15px; }
.terminal-scope p { margin: 8px 0 0; opacity: .75; line-height: 1.6; }
.terminal-distribute { max-width: 560px; }
@container (max-width: 800px) {
  .terminal-page-title { display: block; }
  .terminal-grid { grid-template-columns: 1fr; }
  .terminal-rule { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 0 12px; padding-bottom: 12px; }
  .terminal-rule-number { grid-column: 1 / -1; margin: 0 0 8px; }
  .terminal-section-heading { flex-wrap: wrap; }
}
</style>
