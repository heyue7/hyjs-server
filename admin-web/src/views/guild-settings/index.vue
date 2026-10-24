<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NButton, NDatePicker, NInput, NInputNumber, NSelect, NSwitch, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import {
  getGuildSettings,
  saveGuildSettings
} from '@/service/guilds';
import type {
  GuildPolicyState,
  GuildSettings,
  GuildTableColumn
} from '@/service/guilds';
import GuildRewardsWorkspace from '@/components/operations/guild-rewards-workspace.vue';
import ContextHelp from '@/components/operations/context-help.vue';
import ResourceValueEditor from '@/components/operations/resource-value-editor.vue';
import NumberSequenceEditor from '@/components/operations/number-sequence-editor.vue';
import GuildPrayerEditor from '@/components/operations/guild-prayer-editor.vue';
import GuildTechDirectoryEditor from '@/components/operations/guild-tech-directory-editor.vue';
import { useOperations } from '@/composables/useOperations';

interface EditorState {
  fields: Record<string, string>;
  unionLevels: Record<string, string>[];
  unionTech: Record<string, string>[];
  policies: GuildPolicyState;
}

const { canWrite, loading, error, run } = useOperations();
const settings = ref<GuildSettings>();
const form = ref<EditorState>();
const baseline = ref('');
const saving = ref(false);
const saveError = ref('');
const reason = ref('');
const q = ref('');
const editorValidity = ref<Record<string, boolean>>({});
const invalidInputs = computed(() => Object.values(editorValidity.value).includes(false) || Object.values(form.value?.fields || {}).some(value => value === ''));
function setValidity(key: string, valid: boolean) { editorValidity.value[key] = valid; }
function numberValue(value: string) { return value !== '' && Number.isFinite(Number(value)) ? Number(value) : null; }
const attributes = [
  { label: '攻击加成', value: 'peratt' }, { label: '生命加成', value: 'perhp' },
  { label: '物防加成', value: 'perpdef' }, { label: '魔防加成', value: 'permdef' },
  { label: '暴击', value: 'cri' }, { label: '命中', value: 'hit' },
  { label: '抗暴', value: 'anticri' }, { label: '闪避', value: 'dodge' }
];
const tab = ref('basic');
const rewardsWorkspace = ref<{ hasUnsavedChanges: () => boolean; discardChanges: () => void }>();
function changeTab(value: string) {
  if (tab.value === 'rewards' && rewardsWorkspace.value?.hasUnsavedChanges() && !window.confirm('奖励配置有未保存修改，确定离开并放弃？')) return;
  if (tab.value === 'rewards' && value !== 'rewards') rewardsWorkspace.value?.discardChanges();
  tab.value = value;
}
const dirty = computed(() => Boolean(form.value) && JSON.stringify(form.value) !== baseline.value && canWrite.value);

const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value)) as T;

function applySettings(value: GuildSettings) {
  editorValidity.value = {};
  settings.value = value;
  const fields: Record<string, string> = {};
  for (const group of value.groups) for (const field of group.fields) fields[field.key] = field.value;
  form.value = {
    fields,
    unionLevels: clone(value.unionLevels.rows),
    unionTech: clone(value.unionTech.rows),
    policies: clone({
      ...value.policies,
      unionFight: {
        ...value.policies.unionFight,
        playerBattle: value.policies.unionFight.playerBattle || []
      }
    })
  };
  baseline.value = JSON.stringify(form.value);
  if (!value.groups.some(group => group.key === tab.value) && !['policies','rewards','levels','tech'].includes(tab.value)) tab.value = value.groups[0]?.key || 'policies';
}

async function load() {
  if (dirty.value && !window.confirm('全局公会设置有未保存修改，确定刷新并放弃？')) return;
  if (rewardsWorkspace.value?.hasUnsavedChanges() && !window.confirm('奖励配置有未保存修改，确定刷新并放弃？')) return;
  rewardsWorkspace.value?.discardChanges();
  const result = await run(getGuildSettings);
  if (result) applySettings(result);
}

function groupFields(groupKey: string) {
  const group = settings.value?.groups.find(row => row.key === groupKey);
  const keyword = q.value.trim().toLowerCase();
  return (group?.fields || []).filter(field => !keyword || `${field.key} ${field.label}`.toLowerCase().includes(keyword));
}

function tableColumns(table: 'levels' | 'tech') {
  const columns = (table === 'levels' ? settings.value?.unionLevels.columns : settings.value?.unionTech.columns) || [];
  return columns.map((column: GuildTableColumn) => ({
    title: column.label,
    key: column.key,
    width: column.key.endsWith('Reward') ? 360 : 170,
    render(row: Record<string, string>) {
      if (!column.editable) return row[column.key] || '—';
      const key = `${table}:${table === 'levels' ? row.Level : form.value?.unionTech.indexOf(row)}:${column.key}`;
      if (column.key.endsWith('Reward')) return h(ResourceValueEditor, {
        value: row[column.key] || '', disabled: !canWrite.value || saving.value, label: column.label,
        'onUpdate:value': (value: string) => { row[column.key] = value; }, onValid: (valid: boolean) => setValidity(key, valid)
      });
      if (table === 'tech' && column.key === 'type') return h(NSelect, {
        value: row.type, options: attributes, disabled: !canWrite.value || saving.value,
        'aria-label': '科技属性', 'onUpdate:value': (value: string) => {
          const amount = row.prop.split(',')[1] || '';
          row.type = value; row.prop = `${value},${amount}`;
        }
      });
      if (table === 'tech' && column.key === 'prop') return h(NInputNumber, {
        value: numberValue(row.prop.split(',')[1] || ''), min: 0, disabled: !canWrite.value || saving.value,
        inputProps: { 'aria-label': `${attributes.find(item => item.value === row.type)?.label || '科技'}加成数值` },
        'onUpdate:value': (value: number | null) => { row.prop = `${row.type},${value ?? ''}`; setValidity(key, value !== null && value > 0); }
      });
      if (table === 'tech' && ['UnionCoin', 'UnionBadge'].includes(column.key)) return h(NInput, {
        value: row[column.key], disabled: !canWrite.value || saving.value, placeholder: row.techLevel === '1' ? '无消耗' : '必填',
        inputProps: { inputmode: 'numeric', 'aria-label': column.label },
        'onUpdate:value': (value: string) => {
          row[column.key] = value;
          setValidity(key, value === '' && row.techLevel === '1' || /^\d+$/.test(value) && BigInt(value) <= 9223372036854775807n);
        }
      });
      if (column.valueType !== 'string') {
        if ((row[column.key] === '' || row[column.key] === '-1') && table === 'levels' && row === form.value?.unionLevels.at(-1)) return '已达最高等级';
        return h(NInputNumber, {
          value: numberValue(row[column.key]), min: 0, precision: column.valueType === 'int' ? 0 : undefined,
          disabled: !canWrite.value || saving.value, inputProps: { 'aria-label': column.label },
          placeholder: table === 'tech' && row.techLevel === '1' ? '无消耗' : '必填',
          'onUpdate:value': (value: number | null) => { row[column.key] = value === null ? '' : String(value); setValidity(key, value !== null || table === 'tech' && row.techLevel === '1'); }
        });
      }
      return h(NInput, {
        value: row[column.key] || '',
        size: 'small',
        disabled: !canWrite.value || saving.value,
        inputProps: { 'aria-label': column.label },
        onUpdateValue(value: string) {
          row[column.key] = value;
        }
      });
    }
  })) as DataTableColumns<Record<string, string>>;
}

const levelColumns = computed(() => tableColumns('levels'));
const techColumns = computed(() => tableColumns('tech'));
const gaps = computed(() => (settings.value?.capabilities || []).filter(row => !row.implemented || !row.editable));

function beforeUnload(event: BeforeUnloadEvent) {
  if (!dirty.value && !saving.value) return;
  event.preventDefault();
  event.returnValue = '';
}
onMounted(() => {
  load();
  window.addEventListener('beforeunload', beforeUnload);
});
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));
onBeforeRouteLeave(() => {
  if (saving.value) return false;
  if (!dirty.value) return true;
  return window.confirm('全局公会设置有未保存修改，确定离开并放弃？');
});

async function save() {
  if (!form.value || !settings.value || saving.value || !canWrite.value) return;
  if (invalidInputs.value) { saveError.value = '请补全标记的费用、奖励或规则数值'; return; }
  try {
    if (form.value.policies.prayer.enabled && JSON.parse(form.value.policies.prayer.poolJson).length === 0) { saveError.value = '启用祈祷前请配置祈祷奖励'; tab.value = 'policies'; return; }
    if (form.value.policies.tech.enabled && !JSON.parse(form.value.policies.tech.directoryJson).heroStatue?.length) { saveError.value = '启用科技前请完成八项属性的建筑分配'; tab.value = 'policies'; return; }
  } catch { saveError.value = '祈祷或科技配置无法读取，请检查规则'; tab.value = 'policies'; return; }
  if (!reason.value.trim()) {
    saveError.value = '请填写本次变更原因';
    return;
  }
  if (dirty.value && !window.confirm('确认提交当前全局公会设置？新配置只影响后续操作与新场次。')) return;
  saving.value = true;
  saveError.value = '';
  try {
    const result = await saveGuildSettings({
      version: settings.value.version,
      reason: reason.value.trim(),
      fields: Object.entries(form.value.fields).map(([key, value]) => ({ key, value })),
      unionLevels: clone(form.value.unionLevels),
      unionTech: clone(form.value.unionTech),
      policies: clone(form.value.policies)
    });
    applySettings(result);
    reason.value = '';
    window.$message?.success('全局公会设置已保存');
  } catch (e) {
    saveError.value = e instanceof Error ? e.message : '保存失败，请重试';
  } finally {
    saving.value = false;
  }
}

function reset() {
  if (!window.confirm('放弃未保存的全局公会设置修改？')) return;
  if (settings.value) applySettings(settings.value);
  saveError.value = '';
  reason.value = '';
}
</script>

<template>
  <div class="ops-page">
    <NSpace align="center" class="guild-actions">
      <NInput v-model:value="q" clearable placeholder="搜索参数名称" class="guild-search" />
      <ContextHelp title="设置何时生效" text="修改对后续操作与新场次生效。已开始的祈祷、科技报价和公会战继续使用开始时的设置。" />
      <NButton :loading="loading" :disabled="saving" @click="load">刷新</NButton>
      <NButton v-if="canWrite && tab !== 'rewards'" :disabled="!dirty || saving" @click="reset">放弃修改</NButton>
      <NButton v-if="canWrite && tab !== 'rewards'" type="primary" :loading="saving" :disabled="!form || !dirty || invalidInputs" @click="save">保存设置</NButton>
      <NTag v-if="dirty && canWrite && tab !== 'rewards'" type="warning" :bordered="false">未保存</NTag>
      <NTag v-if="!canWrite" :bordered="false">只读</NTag>
    </NSpace>
    <NAlert v-if="error" type="error" :title="error" class="mb-12px" />
    <NAlert v-if="saveError" type="error" :title="saveError" class="mb-12px" />
    <NAlert v-if="invalidInputs" type="warning" title="请补全标记的费用、奖励或规则数值后再保存。" />
    <NAlert v-for="gap in gaps" :key="gap.key" type="warning" :title="`${gap.label}：${gap.detail}`" class="mb-8px" />

    <NSpin :show="loading">
      <template v-if="settings && form">
        <NTabs :value="tab" @update:value="changeTab" type="line" animated>
          <NTabPane v-for="group in settings.groups" :key="group.key" :name="group.key" :tab="group.label">
            <template v-if="group.key === 'shop'">
              <NCard v-for="shop in settings.shops" :key="shop.path" :bordered="false" class="ops-panel mb-12px">
                <NSpace align="center" justify="space-between">
                  <div>
                    <div class="font-600">{{ shop.label }}</div>
                    <div class="ops-muted mt-4px">{{ shop.note }}</div>
                  </div>
                  <NButton tag="a" :href="shop.path" target="_blank" secondary>打开已有商店设置</NButton>
                </NSpace>
              </NCard>
            </template>
            <template v-else>
              <NAlert v-if="q && !groupFields(group.key).length" type="info" :title="`${group.label}没有匹配参数`" />
              <div v-else class="guild-field-grid">
                <div v-for="field in groupFields(group.key)" :key="field.key" class="guild-field-row" :class="{ 'guild-field-wide': field.valueType === 'string' && field.key !== 'UnionCreateCost' }">
                  <div class="guild-field-label">{{ field.label }}</div>
                  <ResourceValueEditor v-if="field.key === 'UnionCreateCost'" v-model:value="form.fields[field.key]" single label="创建费用" :disabled="!canWrite || saving || !field.editable" @valid="setValidity(field.key, $event)" />
                  <NumberSequenceEditor v-else-if="field.key === 'UnionFightReviveTime'" v-model:value="form.fields[field.key]" label="死亡复活时间" :min="1" :max="86400" integer :max-rows="64" :disabled="!canWrite || saving || !field.editable" @valid="setValidity(field.key, $event)" />
                  <NumberSequenceEditor v-else-if="field.key === 'uFGroupConfig'" v-model:value="form.fields[field.key]" label="分组占比（%）" :labels="['A组', 'B组', 'C组', 'D组']" :min="0.001" :max="100" :total="100" :disabled="!canWrite || saving || !field.editable" @valid="setValidity(field.key, $event)" />
                  <NInputNumber v-else-if="field.valueType !== 'string'" :value="numberValue(form.fields[field.key])" :min="field.min" :max="field.max" :precision="field.valueType === 'int' ? 0 : undefined" :disabled="!canWrite || saving || !field.editable" :input-props="{ 'aria-label': field.label }" placeholder="必填" @update:value="form.fields[field.key] = $event === null ? '' : String($event)" />
                  <NInput
                    v-else
                    v-model:value="form.fields[field.key]"
                    :input-props="{ 'aria-label': field.label }"
                    :disabled="!canWrite || saving"
                    :status="form.fields[field.key] === '' ? 'error' : undefined"
                    placeholder="必填"
                  />
                </div>
              </div>
            </template>
          </NTabPane>

          <NTabPane name="rewards" tab="公会战奖励"><GuildRewardsWorkspace ref="rewardsWorkspace" /></NTabPane>
          <NTabPane name="policies" tab="规则与门槛">
            <NCard :bordered="false" class="ops-panel mb-12px">
              <template #header>公会影响入口</template>
              <NForm label-placement="top" :disabled="!canWrite || saving">
                <NFormItem label="开放公会管理">
                  <NSwitch v-model:value="form.policies.guild.managementEnabled" />
                </NFormItem>
                <NFormItem label="开放公会经济">
                  <NSwitch v-model:value="form.policies.guild.economyEnabled" />
                </NFormItem>
                <NFormItem label="开放公会聊天">
                  <NSwitch v-model:value="form.policies.guild.chatEnabled" />
                </NFormItem>
              </NForm>
            </NCard>

            <NCard :bordered="false" class="ops-panel mb-12px">
              <template #header>祈祷与科技发布</template>
              <NForm label-placement="top" :disabled="!canWrite || saving">
                <NFormItem label="开放祈祷">
                  <NSwitch v-model:value="form.policies.prayer.enabled" />
                </NFormItem>
                <NFormItem label="祈祷池版本">
                  <NInput v-model:value="form.policies.prayer.version" placeholder="必填，启用前发布" />
                </NFormItem>
                <NFormItem label="祈祷奖励">
                  <GuildPrayerEditor v-model:value="form.policies.prayer.poolJson" :disabled="!canWrite || saving" @valid="setValidity('prayerPool', $event)" />
                </NFormItem>
                <NFormItem label="开放科技">
                  <NSwitch v-model:value="form.policies.tech.enabled" />
                </NFormItem>
                <NFormItem label="科技目录版本">
                  <NInput v-model:value="form.policies.tech.version" placeholder="必填，启用前发布" />
                </NFormItem>
                <NFormItem label="科技建筑分配">
                  <GuildTechDirectoryEditor v-model:value="form.policies.tech.directoryJson" :disabled="!canWrite || saving" @valid="setValidity('techDirectory', $event)" />
                </NFormItem>
              </NForm>
            </NCard>

            <NCard :bordered="false" class="ops-panel mb-12px">
              <template #header>捐献宝箱与金币徽章</template>
              <NForm label-placement="top" :disabled="!canWrite || saving">
                <NFormItem label="开放捐献宝箱">
                  <NSwitch v-model:value="form.policies.donateBox.enabled" />
                </NFormItem>
                <section v-for="(tier, index) in form.policies.donateBox.tiers" :key="index" class="guild-reward-tier">
                  <h4>捐献宝箱 {{ index + 1 }}</h4>
                  <NFormItem label="所需捐献积分"><NInputNumber v-model:value="tier.threshold" :min="1" :max="2147483647" :precision="0" :input-props="{ 'aria-label': `捐献宝箱${index + 1}所需积分` }" /></NFormItem>
                  <NFormItem label="获得奖励"><ResourceValueEditor v-model:value="tier.reward" :disabled="!canWrite || saving" :label="`宝箱${index + 1}奖励`" @valid="setValidity(`donateBox:${index}`, $event)" /></NFormItem>
                </section>
                <NFormItem label="开放金币捐献徽章">
                  <NSwitch v-model:value="form.policies.donationBadge.enabled" />
                </NFormItem>
                <NFormItem label="每次消耗金币"><NInputNumber v-model:value="form.policies.donationBadge.coinCost" :min="1" :precision="0" /></NFormItem>
                <NFormItem label="每次获得徽章"><NInputNumber v-model:value="form.policies.donationBadge.badgeAmount" :min="1" :precision="0" /></NFormItem>
                <NFormItem label="每日捐献次数"><NInputNumber v-model:value="form.policies.donationBadge.dailyLimit" :min="1" :max="1000" :precision="0" /></NFormItem>
              </NForm>
            </NCard>

            <NCard :bordered="false" class="ops-panel mb-12px">
              <template #header>公会战入口与场景</template>
              <NForm label-placement="top" :disabled="!canWrite || saving">
                <NFormItem label="开放报名">
                  <NSwitch v-model:value="form.policies.unionFight.enrollment.enrollmentEnabled" />
                </NFormItem>
                <NFormItem label="开放匹配">
                  <NSwitch v-model:value="form.policies.unionFight.enrollment.matchEnabled" />
                </NFormItem>
                <NFormItem label="开放场内文字">
                  <NSwitch v-model:value="form.policies.unionFight.chat.textEnabled" />
                </NFormItem>
                <NFormItem label="开放十八条指挥">
                  <NSwitch v-model:value="form.policies.unionFight.chat.talkEnabled" />
                </NFormItem>
                <NFormItem label="指挥冷却（毫秒）">
                  <NInputNumber v-model:value="form.policies.unionFight.chat.talkCooldownMillis" :min="1000" :max="3600000" />
                </NFormItem>
                <NFormItem label="开放公会战道具">
                  <NSwitch v-model:value="form.policies.unionFight.item.enabled" />
                </NFormItem>
                <NFormItem label="开放场景">
                  <NSwitch v-model:value="form.policies.unionFight.field.enabled" />
                </NFormItem>
                <NFormItem label="准备窗口（毫秒）">
                  <NInputNumber v-model:value="form.policies.unionFight.field.preparationMillis" :min="1" :max="86400000" />
                </NFormItem>
                <NFormItem label="左侧固定复活点"><NInputNumber v-model:value="form.policies.unionFight.field.leftRevivePointId" :min="0" :precision="0" /></NFormItem>
                <NFormItem label="右侧固定复活点"><NInputNumber v-model:value="form.policies.unionFight.field.rightRevivePointId" :min="0" :precision="0" /></NFormItem>
                <NFormItem label="守卫初始生命"><NInputNumber v-model:value="form.policies.unionFight.field.initialGuardHp" :min="0.01" /></NFormItem>
                <NFormItem label="城门初始生命"><NInputNumber v-model:value="form.policies.unionFight.field.initialGateHp" :min="0.01" /></NFormItem>
                <NFormItem label="位置容差">
                  <NInputNumber v-model:value="form.policies.unionFight.field.moveTolerance" :min="0" :max="50" />
                </NFormItem>
              </NForm>
            </NCard>

            <NCard :bordered="false" class="ops-panel">
              <template #header>普通玩家参战策略</template>
              <NDataTable
                v-if="form.policies.unionFight.playerBattle.length"
                :columns="[
                  { title: '编号', key: 'id', width: 70 },
                  { title: '开放', key: 'enabled', width: 80, render: (row: any) => h(NSwitch, { value: row.enabled, disabled: !canWrite || saving, 'onUpdate:value': (value: boolean) => (row.enabled = value) }) },
                  { title: '版本', key: 'version', render: (row: any) => h(NInput, { value: row.version, disabled: !canWrite || saving, 'onUpdate:value': (value: string) => (row.version = value) }) },
                  { title: '开始时间', key: 'startsAt', width: 220, render: (row: any) => h(NDatePicker, { value: row.startsAt || null, type: 'datetime', clearable: true, disabled: !canWrite || saving, 'onUpdate:value': (value: number | null) => (row.startsAt = value || 0) }) },
                  { title: '结束时间', key: 'endsAt', width: 220, render: (row: any) => h(NDatePicker, { value: row.endsAt || null, type: 'datetime', clearable: true, disabled: !canWrite || saving, 'onUpdate:value': (value: number | null) => (row.endsAt = value || 0) }) },
                  { title: '复活规则', key: 'reviveRule', width: 220, render: (row: any) => h('span', { title: row.reviveRule }, row.reviveRule === 'local-death-ordinal-one-based-cap40-v1' ? '按死亡次数递增，最高档封顶' : '其他规则（查看原始标记）') }
                ]"
                :data="form.policies.unionFight.playerBattle"
                :scroll-x="950"
              />
              <NAlert v-else type="info" title="暂无参战策略" />
            </NCard>
          </NTabPane>
          <NTabPane name="levels" tab="等级与每日奖励">
            <NDataTable :columns="levelColumns" :data="form.unionLevels" :scroll-x="1200" size="small" />
          </NTabPane>
          <NTabPane name="tech" tab="公会科技">
            <NDataTable :columns="techColumns" :data="form.unionTech" :scroll-x="1000" size="small" :pagination="{ pageSize: 20 }" />
          </NTabPane>
        </NTabs>

        <NCard v-if="tab !== 'rewards'" :bordered="false" class="ops-panel mt-12px">
          <NForm label-placement="left" label-width="100">
            <NFormItem label="变更原因" required>
              <NInput v-model:value="reason" :disabled="!canWrite || saving" maxlength="500" show-count placeholder="用于审计" />
            </NFormItem>
          </NForm>
        </NCard>

      </template>
    </NSpin>
  </div>
</template>

<style scoped>
.guild-actions { margin-bottom: 12px; }
.guild-search { width: min(260px, 100%); }
.guild-field-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 24px; }
.guild-field-row { display: grid; grid-template-columns: minmax(110px, 1fr) minmax(120px, 1.2fr); align-items: start; gap: 12px; padding: 12px 0; border-bottom: 1px solid rgb(var(--base-text-color) / .09); min-width: 0; }
.guild-field-label { font-size: 13px; font-weight: 500; line-height: 1.5; padding-top: 7px; overflow-wrap: anywhere; }
.guild-field-wide { grid-column: 1 / -1; grid-template-columns: minmax(0, 1fr); gap: 8px; }
.guild-field-wide .guild-field-label { padding-top: 0; }
.guild-field-row :deep(.n-input-number) { width: 100%; }
.guild-reward-tier { border-top: 1px solid rgb(var(--base-text-color) / .09); padding-top: 12px; margin-top: 12px; }
.guild-reward-tier h4 { margin: 0 0 16px; font-weight: 600; }
@media (max-width: 1000px) { .guild-field-grid { grid-template-columns: minmax(0, 1fr); } }
@media (max-width: 600px) {
  .guild-field-grid { grid-template-columns: minmax(0, 1fr); }
  .guild-search { width: 100%; }
  .guild-field-row { grid-template-columns: minmax(0, 1fr); gap: 8px; }
  .guild-field-label { padding-top: 0; }
}
</style>
