<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NButton, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { useOperations } from '@/composables/useOperations';
import { errorMessage, formatTime } from '@/service/operations';
import { emptyGuildReward, getGuildRewards, saveGuildReward } from '@/service/guild-rewards';
import type { GuildRewardKind, GuildRewardRow } from '@/service/guild-rewards';
import RewardList from './reward-list.vue';
const { canWrite, loading, error, run } = useOperations();
const rows = ref<GuildRewardRow[]>([]);
const kind = ref<GuildRewardKind>('settlement');
const open = ref(false);
const draft = ref(emptyGuildReward('settlement'));
const original = ref('');
const create = ref(false);
// Exact backend publication marker; operator-entered or unknown sources remain editable.
const internalSettlementSource = 'local-published-zero-fight-settlement-v1: original timed zero-kill/simultaneous tie lower frozen total power; no combat intent/audit/P6/current HP change; explicit archived/local reward publication; original day2330 inbox; equal power/withdrawal/absent side unresolved';
const internalSettlementPublication = computed(() => !create.value && draft.value.kind === 'settlement' && draft.value.source === internalSettlementSource);
const reason = ref('');
const saveError = ref('');
const saving = ref(false);
const labels: Record<GuildRewardKind, string> = { settlement: '战后奖励', weekly: '周奖励', monthly: '月奖励' };
const dirty = computed(() => open.value && (JSON.stringify(draft.value) !== original.value || reason.value !== ''));
const locked = computed(() => saving.value || !canWrite.value || !draft.value.editable);
interface PersonalMission { kind: number; target: number; score: number }
const missions = computed<PersonalMission[]>(() => {
  try { const value = JSON.parse(draft.value.personalMissionsJson || '[]'); return Array.isArray(value) ? value : []; } catch { return []; }
});
function setMission(index: number, key: keyof PersonalMission, value: number | null) {
  const next = missions.value.map(row => ({ ...row }));
  next[index] = { ...next[index], [key]: value ?? 0 };
  draft.value.personalMissionsJson = JSON.stringify(next);
}
function addMissions() {
  draft.value.personalMissionsJson = JSON.stringify([{ kind: 1, target: 0, score: 0 }, { kind: 2, target: 0, score: 0 }]);
}
const shown = computed(() => rows.value.filter(row => row.kind === kind.value));
async function load() { const out = await run(getGuildRewards); if (out) rows.value = out.items; }
function close() { if (saving.value || dirty.value && !window.confirm('放弃本项尚未保存的修改？')) return false; open.value = false; return true; }
function edit(row: GuildRewardRow, isNew = false) {
  if (open.value && !close()) return;
  draft.value = JSON.parse(JSON.stringify(row)); original.value = JSON.stringify(draft.value);
  reason.value = ''; saveError.value = ''; create.value = isNew; open.value = true;
}
async function save() {
  saveError.value = '';
  if (!reason.value.trim()) { saveError.value = '请填写修改原因'; return; }
  saving.value = true;
  try {
    const saved = await saveGuildReward({ ...draft.value, reason: reason.value }, create.value);
    const index = rows.value.findIndex(row => row.kind === saved.kind && row.group === saved.group && row.weekStart === saved.weekStart && row.month === saved.month);
    if (index < 0) rows.value.unshift(saved); else rows.value[index] = saved;
    draft.value = saved; original.value = JSON.stringify(saved); reason.value = ''; create.value = false;
    window.$message?.success('本项奖励已保存');
  } catch (e) { saveError.value = errorMessage(e); } finally { saving.value = false; }
}
const columns = computed<DataTableColumns<GuildRewardRow>>(() => [
  { title: '期次 / 分组', key: 'period', minWidth: 160, render: row => row.kind === 'settlement' ? `${row.group} 组` : row.kind === 'weekly' ? `${row.weekStart} · ${row.group} 组` : row.month },
  { title: '状态', key: 'enabled', width: 90, render: row => h(NTag, { type: row.enabled ? 'success' : 'default', bordered: false }, { default: () => row.enabled ? '已启用' : '未启用' }) },
  { title: '版本', key: 'version', minWidth: 120 },
  { title: '发布时间', key: 'publishedAt', minWidth: 160, render: row => formatTime(row.publishedAt) },
  { title: '操作', key: 'action', width: 90, fixed: 'right', render: row => h(NButton, { size: 'small', onClick: () => edit(row) }, { default: () => row.editable && canWrite.value ? '编辑' : '查看' }) }
]);
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value) { event.preventDefault(); event.returnValue = ''; } }
onBeforeRouteLeave(() => !dirty.value || window.confirm('放弃本项尚未保存的修改？'));
onMounted(() => { window.addEventListener('beforeunload', beforeUnload); void load(); });
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));
defineExpose({ hasUnsavedChanges: () => dirty.value, discardChanges: () => { open.value = false; reason.value = ""; } });
</script>

<template>
  <div class="guild-rewards">
    <NSpace justify="space-between" align="center">
      <NSelect v-model:value="kind" class="reward-kind" :options="Object.entries(labels).map(([value,label]) => ({ value,label }))" />
      <NSpace>
        <NButton :loading="loading" @click="load">刷新列表</NButton>
        <NButton v-if="kind !== 'settlement' && canWrite" type="primary" @click="edit(emptyGuildReward(kind), true)">新建{{ labels[kind] }}</NButton>
      </NSpace>
    </NSpace>
    <NText depth="3">每项奖励单独保存。战后奖励调整用于新场次，周奖和月奖只能编辑未来期。</NText>
    <NAlert v-if="error" type="error">{{ error }}</NAlert>
    <NDataTable :columns="columns" :data="shown" :loading="loading" :scroll-x="650" :pagination="{ pageSize: 15 }" />
    <NDrawer :show="open" width="min(760px, 100vw)" :on-update:show="(value: boolean) => { if (!value) close(); }" :mask-closable="!saving" :close-on-esc="!saving">
      <NDrawerContent :title="`${create ? '新建' : '编辑'}${labels[draft.kind]}`" closable>
        <NAlert v-if="!draft.editable" type="info" class="mb-4">本期已开始或已用于奖励结算，仅供查看。</NAlert>
        <NForm label-placement="top" :disabled="locked">
          <div class="reward-grid">
            <NFormItem v-if="draft.kind !== 'monthly'" label="分组"><NSelect v-model:value="draft.group" :disabled="!create || locked" :options="['A','B','C','D'].map(value => ({ label: `${value} 组`, value }))" /></NFormItem>
            <NFormItem v-if="draft.kind === 'weekly'" label="周一日期"><NInput v-model:value="draft.weekStart" :disabled="!create || locked" placeholder="YYYY-MM-DD" /></NFormItem>
            <NFormItem v-if="draft.kind === 'monthly'" label="奖励月份"><NInput v-model:value="draft.month" :disabled="!create || locked" placeholder="YYYY-MM" /></NFormItem>
            <NFormItem label="启用"><NSwitch v-model:value="draft.enabled" /></NFormItem>
            <NFormItem label="版本名称"><NInput v-model:value="draft.version" maxlength="64" /></NFormItem>
          </div>
          <NFormItem v-if="!internalSettlementPublication" label="发布依据"><NInput v-model:value="draft.source" type="textarea" maxlength="2048" /></NFormItem>
          <template v-if="draft.kind === 'settlement'">
            <NFormItem label="参与奖励"><RewardList v-model:value="draft.participationRewards" :disabled="locked" /></NFormItem>
            <NFormItem label="胜利奖励"><RewardList v-model:value="draft.victoryRewards" :disabled="locked" /></NFormItem>
            <NFormItem label="运矿优胜奖励"><RewardList v-model:value="draft.transportMvpRewards" :disabled="locked" /></NFormItem>
            <NFormItem label="任务奖励"><RewardList v-model:value="draft.taskRewards" :disabled="locked" /></NFormItem>
            <NFormItem label="个人任务">
              <div class="mission-list">
                <div v-for="(mission,index) in missions" :key="index" class="reward-grid">
                  <NFormItem :label="mission.kind === 1 ? '采矿目标' : '运矿目标'"><NInputNumber :value="mission.target" :min="1" :max="2147483647" :precision="0" @update:value="setMission(index, 'target', $event)" /></NFormItem>
                  <NFormItem label="完成积分"><NInputNumber :value="mission.score" :min="1" :max="2147483647" :precision="0" @update:value="setMission(index, 'score', $event)" /></NFormItem>
                </div>
                <NButton v-if="!draft.personalMissionsJson" dashed :disabled="locked" @click="addMissions">配置采矿与运矿任务</NButton>
                <NButton v-else quaternary :disabled="locked" @click="draft.personalMissionsJson = ''">清空个人任务</NButton>
              </div>
            </NFormItem>
            <div class="reward-grid">
              <NFormItem label="胜方声望"><NInputNumber v-model:value="draft.winPrestige" :min="0" :max="2147483647" :precision="0" /></NFormItem>
              <NFormItem label="败方声望"><NInputNumber v-model:value="draft.losePrestige" :min="0" :max="2147483647" :precision="0" /></NFormItem>
              <NFormItem label="工厂任务声望"><NInputNumber v-model:value="draft.factoryMissionPrestige" :min="0" :max="2147483647" :precision="0" /></NFormItem>
            </div>
          </template>
          <template v-else>
            <div v-for="(rank,index) in draft.ranks" :key="index" class="rank-line">
              <NFormItem label="名次"><NInputNumber v-model:value="rank.rank" :min="1" :max="draft.kind === 'weekly' ? 20 : 1000" :precision="0" /></NFormItem>
              <RewardList v-model:value="rank.rewards" :disabled="locked" />
              <NButton quaternary :disabled="locked" @click="draft.ranks.splice(index,1)">移除名次</NButton>
            </div>
            <NButton dashed :disabled="locked" @click="draft.ranks.push({ rank: draft.ranks.length + 1, rewards: [] })">添加名次</NButton>
            <NSpace v-if="draft.kind === 'monthly'" class="my-4">
              <NCheckbox v-model:checked="draft.memberTitle">前 20 名公会成员称号</NCheckbox>
              <NCheckbox v-model:checked="draft.chairmanTitle">前 5 名公会会长称号</NCheckbox>
            </NSpace>
          </template>
          <NFormItem v-if="draft.editable && canWrite" label="修改原因"><NInput v-model:value="reason" type="textarea" maxlength="500" /></NFormItem>
        </NForm>
        <NAlert v-if="saveError" type="error">{{ saveError }}</NAlert>
        <template #footer>
          <p v-if="internalSettlementPublication" class="settlement-rules">当前仅支持无战斗记录、无击杀或死亡、生命未变化的到时结算，双方均须有人实际入场。比赛结束后，开战前冻结的报名总战力较低方胜出。奖励按本场开战前发布的奖档，于比赛当日北京时间 23:30 起通过邮件发放。同战力、撤回或一方缺席的胜负规则尚未确定，暂不结算。</p>
          <NSpace justify="end"><NButton :disabled="saving" @click="close">关闭</NButton><NButton v-if="draft.editable && canWrite" type="primary" :loading="saving" :disabled="!dirty" @click="save">保存本项</NButton></NSpace>
        </template>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>

<style scoped>
.guild-rewards { display: flex; flex-direction: column; gap: 16px; }
.reward-kind { width: 180px; }
.settlement-rules { margin: 0 0 12px; font-size: 13px; line-height: 1.7; color: rgb(var(--base-text-color) / .65); overflow-wrap: anywhere; }
.mission-list { width: 100%; }
.reward-grid { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 0 20px; }
.rank-line { padding: 16px 0; border-bottom: 1px solid var(--n-border-color); }
@media (max-width: 640px) { .reward-grid { grid-template-columns: 1fr; } :deep(.n-drawer) { max-width: 100vw; } }
</style>
