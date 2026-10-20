<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NButton, NInput, NInputNumber, NSwitch, NTag } from 'naive-ui';
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
    width: table === 'levels' ? 150 : 130,
    render(row: Record<string, string>) {
      return h(NInput, {
        value: row[column.key] || '',
        size: 'small',
        disabled: !canWrite.value || !column.editable,
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
  if (!dirty.value) return;
  event.preventDefault();
  event.returnValue = '';
}
onMounted(() => {
  load();
  window.addEventListener('beforeunload', beforeUnload);
});
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));
onBeforeRouteLeave(() => {
  if (!dirty.value) return true;
  return window.confirm('全局公会设置有未保存修改，确定离开并放弃？');
});

async function save() {
  if (!form.value || !settings.value || saving.value || !canWrite.value) return;
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
      <NButton v-if="canWrite && tab !== 'rewards'" type="primary" :loading="saving" :disabled="!form || !dirty" @click="save">保存设置</NButton>
      <NTag v-if="dirty && canWrite && tab !== 'rewards'" type="warning" :bordered="false">未保存</NTag>
      <NTag v-if="!canWrite" :bordered="false">只读</NTag>
    </NSpace>
    <NAlert v-if="error" type="error" :title="error" class="mb-12px" />
    <NAlert v-if="saveError" type="error" :title="saveError" class="mb-12px" />
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
                <NCard v-for="field in groupFields(group.key)" :key="field.key" :bordered="false" class="guild-field-card">
                  <div class="guild-field-label">{{ field.label }}</div>
                  <NInput
                    v-model:value="form.fields[field.key]"
                    :aria-label="field.label"
                    :disabled="!canWrite"
                    :status="form.fields[field.key] === '' ? 'error' : undefined"
                    placeholder="必填"
                  />
                </NCard>
              </div>
            </template>
          </NTabPane>

          <NTabPane name="rewards" tab="公会战奖励"><GuildRewardsWorkspace ref="rewardsWorkspace" /></NTabPane>
          <NTabPane name="policies" tab="规则与门槛">
            <NCard :bordered="false" class="ops-panel mb-12px">
              <template #header>公会影响入口</template>
              <NForm label-placement="left" label-width="160" :disabled="!canWrite">
                <NFormItem label="开放公会管理">
                  <NSwitch v-model:value="form.policies.guild.managementEnabled" />
                </NFormItem>
                <NFormItem label="开放公会经济">
                  <NSwitch v-model:value="form.policies.guild.economyEnabled" />
                </NFormItem>
                <NFormItem label="开放公会聊天">
                  <NSwitch v-model:value="form.policies.guild.chatEnabled" />
                </NFormItem>
                <NFormItem label="原来源">
                  <NInput :value="form.policies.guild.source" readonly type="textarea" :autosize="{ minRows: 1, maxRows: 3 }" />
                </NFormItem>
              </NForm>
            </NCard>

            <NCard :bordered="false" class="ops-panel mb-12px">
              <template #header>祈祷与科技发布</template>
              <NForm label-placement="left" label-width="160" :disabled="!canWrite">
                <NFormItem label="开放祈祷">
                  <NSwitch v-model:value="form.policies.prayer.enabled" />
                </NFormItem>
                <NFormItem label="祈祷池版本">
                  <NInput v-model:value="form.policies.prayer.version" placeholder="必填，启用前发布" />
                </NFormItem>
                <NFormItem label="祈祷池 JSON">
                  <NInput v-model:value="form.policies.prayer.poolJson" type="textarea" :autosize="{ minRows: 3, maxRows: 10 }" />
                </NFormItem>
                <NFormItem label="祈祷池哈希">
                  <NInput :value="form.policies.prayer.poolHash" readonly />
                </NFormItem>
                <NFormItem label="开放科技">
                  <NSwitch v-model:value="form.policies.tech.enabled" />
                </NFormItem>
                <NFormItem label="科技目录版本">
                  <NInput v-model:value="form.policies.tech.version" placeholder="必填，启用前发布" />
                </NFormItem>
                <NFormItem label="科技目录 JSON">
                  <NInput v-model:value="form.policies.tech.directoryJson" type="textarea" :autosize="{ minRows: 3, maxRows: 10 }" />
                </NFormItem>
                <NFormItem label="科技目录哈希">
                  <NInput :value="form.policies.tech.directoryHash" readonly />
                </NFormItem>
              </NForm>
            </NCard>

            <NCard :bordered="false" class="ops-panel mb-12px">
              <template #header>捐献宝箱与金币徽章</template>
              <NForm label-placement="left" label-width="160" :disabled="!canWrite">
                <NFormItem label="开放捐献宝箱">
                  <NSwitch v-model:value="form.policies.donateBox.enabled" />
                </NFormItem>
                <NFormItem label="宝箱版本">
                  <NInput :value="form.policies.donateBox.version" readonly />
                </NFormItem>
                <NFormItem v-for="(tier, index) in form.policies.donateBox.tiers" :key="index" :label="`宝箱 ${index + 1} 积分 / 奖励`">
                  <div class="flex flex-wrap gap-8px w-full">
                    <NInputNumber v-model:value="tier.threshold" :min="1" :max="2147483647" style="width: 150px" />
                    <NInput v-model:value="tier.reward" placeholder="coin,1000;item_005,1" style="flex: 1; min-width: 200px" />
                  </div>
                </NFormItem>
                <NFormItem label="开放金币捐献徽章">
                  <NSwitch v-model:value="form.policies.donationBadge.enabled" />
                </NFormItem>
                <NFormItem label="单笔金币/徽章/日限">
                  <NSpace>
                    <NInputNumber v-model:value="form.policies.donationBadge.coinCost" :min="1" style="width: 150px" />
                    <NInputNumber v-model:value="form.policies.donationBadge.badgeAmount" :min="1" style="width: 120px" />
                    <NInputNumber v-model:value="form.policies.donationBadge.dailyLimit" :min="1" :max="1000" style="width: 120px" />
                  </NSpace>
                </NFormItem>
              </NForm>
            </NCard>

            <NCard :bordered="false" class="ops-panel mb-12px">
              <template #header>公会战入口与场景</template>
              <NForm label-placement="left" label-width="160" :disabled="!canWrite">
                <NFormItem label="开放报名">
                  <NSwitch v-model:value="form.policies.unionFight.enrollment.enrollmentEnabled" />
                </NFormItem>
                <NFormItem label="开放匹配">
                  <NSwitch v-model:value="form.policies.unionFight.enrollment.matchEnabled" />
                </NFormItem>
                <NFormItem label="开放场内文字">
                  <NSwitch v-model:value="form.policies.unionFight.chat.textEnabled" />
                </NFormItem>
                <NFormItem label="开放原十八条指挥">
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
                <NFormItem label="左/右固定复活点">
                  <NSpace>
                    <NInputNumber v-model:value="form.policies.unionFight.field.leftRevivePointId" :min="0" />
                    <NInputNumber v-model:value="form.policies.unionFight.field.rightRevivePointId" :min="0" />
                  </NSpace>
                </NFormItem>
                <NFormItem label="守卫/城门初始 HP">
                  <NSpace>
                    <NInputNumber v-model:value="form.policies.unionFight.field.initialGuardHp" :min="0.01" />
                    <NInputNumber v-model:value="form.policies.unionFight.field.initialGateHp" :min="0.01" />
                  </NSpace>
                </NFormItem>
                <NFormItem label="位置容差">
                  <NInputNumber v-model:value="form.policies.unionFight.field.moveTolerance" :min="0" :max="50" />
                </NFormItem>
                <NFormItem label="场景来源">
                  <NInput v-model:value="form.policies.unionFight.field.source" type="textarea" :autosize="{ minRows: 1, maxRows: 3 }" />
                </NFormItem>
              </NForm>
            </NCard>

            <NCard :bordered="false" class="ops-panel">
              <template #header>普通玩家参战策略</template>
              <NDataTable
                v-if="form.policies.unionFight.playerBattle.length"
                :columns="[
                  { title: '编号', key: 'id', width: 70 },
                  { title: '开放', key: 'enabled', width: 80, render: (row: any) => h(NSwitch, { value: row.enabled, disabled: !canWrite, 'onUpdate:value': (value: boolean) => (row.enabled = value) }) },
                  { title: '版本', key: 'version', render: (row: any) => h(NInput, { value: row.version, disabled: !canWrite, 'onUpdate:value': (value: string) => (row.version = value) }) },
                  { title: '开始时间', key: 'startsAt', width: 150, render: (row: any) => h(NInputNumber, { value: row.startsAt, disabled: !canWrite, 'onUpdate:value': (value: number | null) => (row.startsAt = value || 0) }) },
                  { title: '结束时间', key: 'endsAt', width: 150, render: (row: any) => h(NInputNumber, { value: row.endsAt, disabled: !canWrite, 'onUpdate:value': (value: number | null) => (row.endsAt = value || 0) }) },
                  { title: '规则', key: 'reviveRule', width: 220 }
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
              <NInput v-model:value="reason" :disabled="!canWrite" maxlength="500" show-count placeholder="用于审计" />
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
.guild-field-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px; }
.guild-field-card { border: 1px solid var(--n-border-color, #e5e7eb); border-radius: 8px; }
.guild-field-label { min-height: 42px; font-weight: 600; line-height: 1.45; }
@media (max-width: 600px) {
  .guild-field-grid { grid-template-columns: minmax(0, 1fr); }
  .guild-search { width: 100%; }
}
</style>
