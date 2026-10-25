<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import type { DialogReactive } from 'naive-ui';
import ResourceValueEditor from '@/components/operations/resource-value-editor.vue';
import { useOperations } from '@/composables/useOperations';
import { getDailyTasks, saveDailyTasks } from '@/service/daily-tasks';
import type { DailyTaskRow, DailyTaskRules } from '@/service/daily-tasks';
import { ApiError, errorMessage } from '@/service/operations';

const { canWrite } = useOperations();
const snapshot = ref<DailyTaskRules>();
const rows = ref<(DailyTaskRow & { value: string; valid: boolean })[]>([]);
const reason = ref('');
const original = ref('');
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const conflict = ref(false);
let alive = true;
const contents = () => JSON.stringify(rows.value.map(row => ({ id: row.id, value: row.value })));
const dirty = computed(() => Boolean(snapshot.value) && contents() !== original.value);
const disabled = computed(() => !canWrite.value || saving.value || loading.value || conflict.value);
const valid = computed(() => rows.value.length === 7 && rows.value.every(row => row.valid));
function apply(data: DailyTaskRules) {
  snapshot.value = data;
  rows.value = data.items.map(row => ({ ...row, value: `${row.reward.resourceId},${row.reward.amount}`, valid: true }));
  original.value = contents();
  reason.value = '';
  conflict.value = false;
}
async function load() {
  if (loading.value || saving.value) return;
  loading.value = true;
  error.value = '';
  try { const data = await getDailyTasks(); if (alive) apply(data); }
  catch (cause) { if (alive) error.value = errorMessage(cause); }
  finally { if (alive) loading.value = false; }
}
let pendingDialog: { promise: Promise<boolean>; dispose: () => void } | undefined;
function confirm(title: string, content: string, positiveText: string): Promise<boolean> {
  if (pendingDialog) return pendingDialog.promise;
  if (!window.$dialog) return Promise.resolve(false);
  let resolve!: (value: boolean) => void;
  const promise = new Promise<boolean>(done => { resolve = done; });
  let accepted = false;
  let settled = false;
  let dialog: DialogReactive;
  const settle = () => { if (!settled) { settled = true; pendingDialog = undefined; resolve(accepted); } };
  dialog = window.$dialog.warning({ title, content, positiveText, negativeText: '继续编辑',
    onPositiveClick: () => { accepted = true; dialog.destroy(); settle(); },
    onNegativeClick: () => { accepted = false; dialog.destroy(); settle(); },
    onClose: () => { accepted = false; dialog.destroy(); settle(); },
    onEsc: () => { accepted = false; dialog.destroy(); settle(); },
    onMaskClick: () => { accepted = false; dialog.destroy(); settle(); },
    onAfterLeave: settle
  });
  pendingDialog = { promise, dispose: () => { accepted = false; dialog.destroy(); settle(); } };
  return promise;
}
async function refresh() {
  if (loading.value || saving.value) return;
  if (dirty.value && !await confirm('重新载入每日任务？', '尚未保存的奖励修改将丢失。', '重新载入')) return;
  if (alive) await load();
}
async function submit() {
  if (disabled.value || !dirty.value || !valid.value || !reason.value.trim() || !snapshot.value) return;
  saving.value = true;
  error.value = '';
  try {
    const data = await saveDailyTasks({ version: snapshot.value.version, reason: reason.value.trim(), items: rows.value.map(row => {
      const [resourceId, amount] = row.value.split(',');
      return { id: row.id, reward: { resourceId, amount: Number(amount) } };
    }) });
    if (alive) { apply(data); window.$message?.success('每日任务奖励已保存'); }
  } catch (cause) {
    if (alive) {
      conflict.value = cause instanceof ApiError && cause.status === 409;
      error.value = conflict.value
        ? '每日任务奖励已被其他人修改。你的输入已保留，请重新载入后再调整。'
        : errorMessage(cause);
    }
  } finally { if (alive) saving.value = false; }
}
async function save() {
  if (disabled.value || !dirty.value || !valid.value || !reason.value.trim()) return;
  const accepted = await confirm('保存每日任务奖励？', '今天已生成的任务保持原奖励；新奖励从明天生效，今天尚未生成任务的角色首次进入时也会使用。已领取奖励不补发。', '保存奖励');
  if (alive && accepted) await submit();
}
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value || saving.value) { event.preventDefault(); event.returnValue = ''; } }
onBeforeRouteLeave(() => {
  if (pendingDialog) return false;
  if (saving.value) return false;
  if (!dirty.value) return true;
  return confirm('离开每日任务配置？', '尚未保存的奖励修改将丢失。', '放弃修改');
});
onMounted(() => { window.addEventListener('beforeunload', beforeUnload); void load(); });
onBeforeUnmount(() => { alive = false; pendingDialog?.dispose(); window.removeEventListener('beforeunload', beforeUnload); });
</script>

<template>
  <div class="ops-page daily-tasks-page">
    <header class="daily-heading">
      <h1>每日任务奖励</h1>
      <NButton :disabled="loading || saving" :loading="loading" @click="refresh">重新载入</NButton>
    </header>
    <NAlert v-if="error" type="error" :show-icon="false" role="alert">{{ error }}</NAlert>
    <NAlert v-if="!canWrite" type="info" :show-icon="false">当前账号可查看每日任务奖励。</NAlert>
    <NSpin :show="loading">
      <section v-if="snapshot" class="daily-list" aria-label="每日任务配置">
        <article v-for="row in rows" :key="row.id" class="daily-row">
          <div class="daily-condition">
            <h2 :id="`task-${row.id}`">{{ row.name }}</h2>
            <p>目标 {{ row.total }} 次 · {{ row.score }} 积分 · {{ row.minLevel }} 级开放 <span v-if="!row.enabled">· 已停用</span></p>
          </div>
          <ResourceValueEditor v-model:value="row.value" single :label="`${row.name}奖励`" :disabled="disabled" @valid="row.valid = $event" />
        </article>
      </section>
    </NSpin>
    <form v-if="snapshot" class="daily-save" @submit.prevent="save">
      <p>今天已生成的任务保持原奖励。新奖励从明天生效；今天尚未生成任务的角色首次进入时也会使用。</p>
      <NFormItem label="修改原因" required>
        <NInput v-model:value="reason" type="textarea" :maxlength="500" show-count :disabled="disabled" placeholder="填写本次奖励调整原因" />
      </NFormItem>
      <NButton type="primary" attr-type="submit" :loading="saving" :disabled="disabled || !dirty || !valid || !reason.trim()">保存奖励</NButton>
    </form>
  </div>
</template>

<style scoped>
.daily-tasks-page { gap: 16px; }
.daily-heading { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.daily-heading h1 { margin: 0; font-size: 20px; font-weight: 600; }
.daily-list { min-width: 0; }
.daily-row { display: grid; grid-template-columns: minmax(200px, 1fr) minmax(0, 2fr); gap: 16px; align-items: start; padding: 16px 0; border-bottom: 1px solid var(--n-border-color, #d8dee8); }
.daily-condition { min-width: 0; }
.daily-condition h2 { margin: 0 0 4px; font-size: 15px; font-weight: 600; }
.daily-condition p, .daily-save p { margin: 0; line-height: 1.65; }
.daily-save { display: grid; gap: 16px; max-width: 760px; }
.daily-save > .n-button { justify-self: start; }
@media (max-width: 680px) { .daily-row { grid-template-columns: minmax(0, 1fr); gap: 8px; } }
</style>
