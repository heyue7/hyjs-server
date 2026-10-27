<script setup lang="ts">
// UI decision record: the operator compares one enrollment window with two
// fight windows. Keep those civil times together, preview their ends beside
// the editor, and use the existing Naive drawer/table and restrained surfaces.
// Browser rendering is reserved for the integrating agent's verification.
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NAlert, NButton, NDataTable, NDrawer, NDrawerContent, NForm, NFormItem, NInput, NInputNumber, NSpace, NSwitch, NTag, NText } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { useOperations } from '@/composables/useOperations';
import { errorMessage } from '@/service/operations';
import { emptyGuildSchedule, getGuildSchedules, saveGuildCalendar, saveGuildSchedule } from '@/service/guild-schedules';
import type { GuildScheduleRow, GuildSchedules } from '@/service/guild-schedules.types';
const { canWrite, loading, error, run } = useOperations();
const data = ref<GuildSchedules>();
const mode = ref(false);
const modeReason = ref('');
const modeSaving = ref(false);
const open = ref(false);
const creating = ref(false);
const saving = ref(false);
const reason = ref('');
const saveError = ref('');
const publicationNotes = ref<string[]>([]);
const draft = ref(emptyGuildSchedule());
const baseline = ref('');
const dirty = computed(() => open.value && (JSON.stringify(draft.value) !== baseline.value || reason.value !== ''));
const modeDirty = computed(() => Boolean(data.value) && (mode.value !== data.value?.customCalendarEnabled || modeReason.value !== ''));
const locked = computed(() => !canWrite.value || saving.value || !draft.value.editable);
const preparationMinutes = computed(() => draft.value.enabled && !data.value?.preparationMinutes ? 5 : data.value?.preparationMinutes ?? 0);
function apply(value: GuildSchedules) { data.value = value; mode.value = value.customCalendarEnabled; modeReason.value = ''; }
async function load() {
  if ((dirty.value || modeDirty.value) && !window.confirm('放弃尚未保存的排期修改并刷新？')) return;
  const value = await run(getGuildSchedules); if (value) { apply(value); publicationNotes.value = []; open.value = false; }
}
function edit(row: GuildScheduleRow, create = false) {
  if (dirty.value && !window.confirm('放弃当前排期尚未保存的修改？')) return;
  draft.value = { ...row, enrollmentStart: row.enrollmentStart.replace(' ', 'T'), enrollmentEnd: row.enrollmentEnd.replace(' ', 'T') };
  baseline.value = JSON.stringify(draft.value); reason.value = ''; saveError.value = ''; creating.value = create; open.value = true;
}
function close() {
  if (saving.value || dirty.value && !window.confirm('放弃本项尚未保存的排期修改？')) return;
  open.value = false;
}
// Always parse and format explicit UTC+8. Browser/device timezone cannot shift a publication.
function endTime(time: string) {
  const value = new Date(`${draft.value.day}T${time}:00+08:00`).getTime() + draft.value.durationMinutes * 60000;
  if (!Number.isFinite(value)) return '请选择日期和开战时间';
  return new Date(value + 8 * 3600000).toISOString().slice(0, 16).replace('T', ' ');
}
const firstEnd = computed(() => endTime(draft.value.firstStartTime));
const secondEnd = computed(() => endTime(draft.value.secondStartTime));
const scheduleError = computed(() => {
  const row = draft.value;
  if (!row.day || !row.enrollmentStart || !row.enrollmentEnd || !row.firstStartTime || !row.secondStartTime) return '';
  const civil = (value: string) => new Date(`${value}:00+08:00`).getTime();
  const start = civil(`${row.day}T${row.firstStartTime}`);
  const second = civil(`${row.day}T${row.secondStartTime}`);
  const enrollStart = civil(row.enrollmentStart);
  const enrollEnd = civil(row.enrollmentEnd);
  const duration = row.durationMinutes * 60000;
  if (![start, second, enrollStart, enrollEnd, duration].every(Number.isFinite)) return '请填写完整的日期和时间。';
  if (enrollStart >= enrollEnd) return '报名截止须晚于报名开始。';
  if (enrollEnd > start - preparationMinutes.value * 60000) return '报名截止须不晚于第一场准备开始。';
  if (second < start + duration) return '第二场开战须不早于第一场结束。';
  const nextDay = civil(`${row.day}T00:00`) + 86400000;
  if (start + duration > nextDay || second + duration > nextDay) return '两场公会战须在战斗日期当天结束。';
  return '';
});
const valid = computed(() => Boolean(draft.value.day && draft.value.enrollmentStart && draft.value.enrollmentEnd && draft.value.firstStartTime && draft.value.secondStartTime && draft.value.version.trim() && draft.value.durationMinutes >= 1 && draft.value.durationMinutes <= 30 && reason.value.trim()));
async function save() {
  if (!valid.value || locked.value || scheduleError.value) return;
  saving.value = true; saveError.value = '';
  try {
    const { day, enrollmentStart, enrollmentEnd, firstStartTime, secondStartTime, durationMinutes, enabled, version, rowVersion } = draft.value;
    const row = await saveGuildSchedule({ day, enrollmentStart: enrollmentStart.replace('T', ' '), enrollmentEnd: enrollmentEnd.replace('T', ' '), firstStartTime, secondStartTime, durationMinutes, enabled, version, rowVersion, reason: reason.value }, creating.value);
    publicationNotes.value = (row.publicationNotes ?? []).map(note => note.replaceAll('门禁', '开关'));
    if (data.value) { const at = data.value.items.findIndex(item => item.day === row.day); if (at < 0) data.value.items.unshift(row); else data.value.items[at] = row; }
    reason.value = ''; baseline.value = JSON.stringify(draft.value); open.value = false;
    window.$message?.success('排期已保存');
  } catch (e) { saveError.value = errorMessage(e); } finally { saving.value = false; }
}
async function saveMode() {
  if (!data.value || !canWrite.value || !modeReason.value.trim()) return;
  modeSaving.value = true;
  try { apply(await saveGuildCalendar(mode.value, data.value.policyVersion, modeReason.value)); publicationNotes.value = []; window.$message?.success('日历模式已保存'); }
  catch (e) { error.value = errorMessage(e); } finally { modeSaving.value = false; }
}
const columns = computed<DataTableColumns<GuildScheduleRow>>(() => [
  { title: '战斗日期', key: 'day', width: 130 },
  { title: '报名开始 → 截止（北京时间）', key: 'enrollmentStart', minWidth: 260, render: row => row.legacy ? '原固定宣战日历' : `${row.enrollmentStart} → ${row.enrollmentEnd}` },
  { title: '两场开战 / 每场时长', key: 'firstStartTime', minWidth: 190, render: row => `${row.firstStartTime} / ${row.secondStartTime} · ${row.durationMinutes} 分钟` },
  { title: '发布状态', key: 'enabled', width: 150, render: row => h(NTag, { type: row.legacy ? 'default' : row.enabled ? 'success' : 'default', bordered: false }, { default: () => row.legacy ? '旧日历' : `${row.enabled ? '启用' : '关闭'}${row.frozen ? ' · 已锁定' : ''}` }) },
  { title: '版本', key: 'version', width: 140 },
  { title: '操作', key: 'actions', width: 90, render: row => h(NButton, { size: 'small', onClick: () => edit(row) }, { default: () => row.editable && canWrite.value ? '编辑' : '查看' }) }
]);
function hasUnsavedChanges() { return dirty.value || modeDirty.value; }
function discardChanges() { open.value = false; reason.value = ''; if (data.value) apply(data.value); }
defineExpose({ hasUnsavedChanges, discardChanges });
onBeforeRouteLeave(() => !hasUnsavedChanges() || window.confirm('排期有未保存修改，确定离开？'));
function beforeUnload(event: BeforeUnloadEvent) { if (hasUnsavedChanges()) event.preventDefault(); }
onMounted(() => { void load(); window.addEventListener('beforeunload', beforeUnload); });
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));
</script>

<template>
  <section class="schedule-workspace">
    <div class="calendar-mode">
      <div><h3>公会战日历</h3><NText depth="3">所有时间均为北京时间（UTC+8）。已宣战或已创建比赛的排期锁定。</NText></div>
      <NSpace><NButton :disabled="loading || saving" @click="load">刷新</NButton><NButton type="primary" :disabled="!canWrite || loading" @click="edit(emptyGuildSchedule(), true)">新增未来排期</NButton></NSpace>
    </div>
    <div class="mode-controls">
      <label class="switch-label"><NSwitch v-model:value="mode" :disabled="!canWrite || modeSaving" aria-label="使用自定义日期时间" /> 使用自定义日期时间</label>
      <NInput v-model:value="modeReason" placeholder="切换日历的原因" :disabled="!canWrite || modeSaving" maxlength="500" :input-props="{ 'aria-label': '切换日历原因' }" />
      <NButton :loading="modeSaving" :disabled="!canWrite || !modeDirty || !modeReason.trim()" @click="saveMode">保存日历模式</NButton>
    </div>
    <NAlert v-if="!data?.customCalendarEnabled" type="info">当前沿用原一、三、五宣战日历。自定义排期保存后，需开启自定义日期时间才会生效。</NAlert>
    <NAlert v-if="error" type="error">{{ error }}</NAlert>
    <NAlert v-if="publicationNotes.length" type="warning" title="本次排期发布提示"><p v-for="note in publicationNotes" :key="note">{{ note }}</p></NAlert>
    <div class="schedule-desktop">
      <NDataTable :columns="columns" :data="data?.items || []" :loading="loading" :scroll-x="960" :pagination="{ pageSize: 12 }" />
    </div>
    <div class="schedule-mobile" :aria-busy="loading">
      <NText v-if="loading" depth="3" role="status">正在加载公会战排期…</NText>
      <ul v-if="data?.items.length" class="schedule-cards" aria-label="公会战排期">
        <li v-for="row in data.items" :key="row.day" class="schedule-card">
          <div class="schedule-card-heading">
            <h4>{{ row.day }}</h4>
            <NButton size="small" :aria-label="`${row.editable && canWrite ? '编辑' : '查看'} ${row.day} 公会战排期`" @click="edit(row)">{{ row.editable && canWrite ? '编辑' : '查看' }}</NButton>
          </div>
          <NSpace :size="8">
            <NTag :type="row.enabled ? 'success' : 'default'" :bordered="false">{{ row.enabled ? '启用' : '关闭' }}</NTag>
            <NTag v-if="row.legacy" :bordered="false">旧日历</NTag>
            <NTag v-if="row.frozen" :bordered="false">已锁定</NTag>
          </NSpace>
          <dl class="schedule-card-details">
            <div><dt>报名开始</dt><dd>{{ row.enrollmentStart || '—' }}</dd></div>
            <div><dt>报名截止</dt><dd>{{ row.enrollmentEnd || '—' }}</dd></div>
            <div><dt>第一场开战</dt><dd>{{ row.firstStartTime || '—' }}</dd></div>
            <div><dt>第二场开战</dt><dd>{{ row.secondStartTime || '—' }}</dd></div>
            <div><dt>每场时长</dt><dd>{{ row.durationMinutes }} 分钟</dd></div>
            <div><dt>版本</dt><dd>{{ row.version || '—' }}</dd></div>
          </dl>
        </li>
      </ul>
      <NText v-else-if="!loading && data" depth="3" class="schedule-empty">暂无公会战排期</NText>
    </div>
    <NDrawer :show="open" width="min(760px, 100vw)" :mask-closable="!saving" :close-on-esc="!saving" :on-update:show="(value: boolean) => { if (!value) close(); }">
      <NDrawerContent :title="`${creating ? '新增' : draft.editable && canWrite ? '编辑' : '查看'}公会战排期`" closable>
        <NAlert v-if="!draft.editable" type="info" class="mb-4">此排期已经开始、用于报名或属于旧日历，当前仅供查看。请新增未来排期。</NAlert>
        <NForm label-placement="top" :disabled="locked">
          <div class="schedule-grid">
            <NFormItem label="战斗日期（北京时间）"><input v-model="draft.day" type="date" :disabled="locked || !creating" class="civil-input" aria-label="战斗日期（北京时间）"></NFormItem>
            <NFormItem label="版本名称"><NInput v-model:value="draft.version" :disabled="locked" maxlength="64" placeholder="例如：十一月首周" :input-props="{ 'aria-label': '排期版本名称' }" /></NFormItem>
            <NFormItem label="报名开始（北京时间）"><input v-model="draft.enrollmentStart" type="datetime-local" :disabled="locked" class="civil-input" aria-label="报名开始（北京时间）"></NFormItem>
            <NFormItem label="报名截止（北京时间）"><input v-model="draft.enrollmentEnd" type="datetime-local" :disabled="locked" class="civil-input" aria-label="报名截止（北京时间）"></NFormItem>
            <NFormItem label="第一场开战（北京时间）"><input v-model="draft.firstStartTime" type="time" :disabled="locked" class="civil-input" aria-label="第一场开战（北京时间）"></NFormItem>
            <NFormItem label="第二场开战（北京时间）"><input v-model="draft.secondStartTime" type="time" :disabled="locked" class="civil-input" aria-label="第二场开战（北京时间）"></NFormItem>
            <NFormItem label="每场时长（分钟）"><NInputNumber :value="draft.durationMinutes || null" :min="1" :max="30" :precision="0" :disabled="locked" :input-props="{ 'aria-label': '每场时长（分钟）' }" @update:value="draft.durationMinutes = $event ?? 0" /></NFormItem>
            <NFormItem label="本排期启用"><NSwitch v-model:value="draft.enabled" :disabled="locked" aria-label="启用本排期" /></NFormItem>
          </div>
          <div class="end-preview" aria-live="polite"><strong>北京时间预览</strong><span>第一场结束：{{ firstEnd }}</span><span>第二场结束：{{ secondEnd }}</span><NText depth="3">报名截止须不晚于第一场准备开始；准备时长 {{ preparationMinutes }} 分钟。两场须在同一天内结束。</NText><NText v-if="draft.enabled" depth="3">启用时同时准备本场完整规则。已开始月份的月奖不追溯发放；报名、匹配和日历开关保留当前选择。</NText></div>
          <NFormItem v-if="draft.editable && canWrite" label="修改原因"><NInput v-model:value="reason" type="textarea" maxlength="500" :disabled="locked" :input-props="{ 'aria-label': '排期修改原因' }" /></NFormItem>
        </NForm>
        <NAlert v-if="scheduleError || saveError" type="error">{{ scheduleError || saveError }}</NAlert>
        <template #footer><NSpace justify="end"><NButton :disabled="saving" @click="close">关闭</NButton><NButton v-if="draft.editable && canWrite" type="primary" :loading="saving" :disabled="!valid || !dirty || Boolean(scheduleError)" @click="save">保存排期</NButton></NSpace></template>
      </NDrawerContent>
    </NDrawer>
  </section>
</template>

<style scoped>
.schedule-workspace { display: flex; flex-direction: column; gap: 16px; }
.calendar-mode { display: flex; justify-content: space-between; align-items: center; gap: 16px; }
h3 { margin: 0 0 6px; font-size: 16px; font-weight: 600; }
.mode-controls { display: grid; grid-template-columns: auto minmax(160px, 1fr) auto; align-items: center; gap: 16px; }
.switch-label { display: flex; align-items: center; gap: 10px; white-space: nowrap; }
.schedule-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 20px; }
.civil-input { width: 100%; min-width: 0; height: 34px; padding: 0 10px; color: inherit; background: transparent; border: 1px solid rgb(var(--base-text-color) / .24); border-radius: 3px; font: inherit; }
.civil-input:focus-visible { outline: 2px solid rgb(var(--primary-color)); outline-offset: 2px; }
.civil-input:disabled { opacity: .6; cursor: not-allowed; }
.end-preview { display: grid; gap: 6px; margin-bottom: 20px; padding: 16px 0; border-block: 1px solid rgb(var(--base-text-color) / .16); line-height: 1.7; }
/* Both views share the same rows and edit handler; display:none also removes
   the inactive view's controls from keyboard and accessibility navigation. */
.schedule-desktop { min-width: 0; }
.schedule-mobile { display: none; }
.schedule-cards { display: grid; gap: 16px; margin: 0; padding: 0; list-style: none; }
.schedule-card { display: grid; gap: 16px; min-width: 0; padding: 16px; border: 1px solid rgb(var(--base-text-color) / .16); border-radius: 6px; }
.schedule-card-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.schedule-card-heading h4 { margin: 0; font-size: 16px; font-weight: 600; }
.schedule-card-details { display: grid; gap: 16px; margin: 0; }
.schedule-card-details > div { display: grid; grid-template-columns: 84px minmax(0, 1fr); align-items: baseline; gap: 16px; }
.schedule-card-details dt { color: rgb(var(--base-text-color) / .65); }
.schedule-card-details dd { min-width: 0; margin: 0; overflow-wrap: anywhere; }
.schedule-empty { padding-block: 16px; text-align: center; }
@media (max-width: 640px) {
  .calendar-mode { align-items: flex-start; flex-direction: column; }
  .mode-controls, .schedule-grid { grid-template-columns: 1fr; }
  .schedule-desktop { display: none; }
  .schedule-mobile { display: grid; gap: 16px; }
}
</style>
