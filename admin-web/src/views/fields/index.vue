<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { onBeforeRouteLeave, useRoute } from 'vue-router';
import { ApiError, api, errorMessage, json } from '@/service/operations';
import type { FieldDeployment, FieldDeployments, FieldResource, FieldTemplate } from '@/service/fields';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { useOperations } from '@/composables/useOperations';
import DropEditor from './drop-editor.vue';
import PointEditor from './point-editor.vue';

const scope = useServerScopeStore();
const route = useRoute();
const { canWrite } = useOperations();
const mode = ref<'templates' | 'deployments'>(route.query.mode === 'deployments' ? 'deployments' : 'templates');
const templates = ref<FieldTemplate[]>([]);
const resources = ref<FieldResource[]>([]);
const gatherResources = computed(() => resources.value.filter(r => r.allowedKinds.includes('gather')));
const monsterResources = computed(() => resources.value.filter(r => r.allowedKinds.includes('monster')));
const bossResources = computed(() => resources.value.filter(r => r.allowedKinds.includes('boss')));
const deployments = ref<FieldDeployment[]>([]);
const selected = ref<FieldTemplate>();
const original = ref('');
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const success = ref('');
const conflict = ref(false);
const search = ref('');
const activeTab = ref('overview');
const bossLevel = ref(1);
const dirty = computed(() => Boolean(selected.value) && JSON.stringify(selected.value) !== original.value);
const filtered = computed(() =>
  templates.value.filter(t => `${t.name} ${t.configuration.scene}`.includes(search.value))
);
const cfg = computed(() => selected.value?.configuration);
const readonly = computed(() => !canWrite.value || saving.value);
const gatherOptions = (kind: string) =>
  (cfg.value?.gatherGroups || [])
    .filter(g => g.id.includes(`group_${kind}_`))
    .map(g => ({ label: `${kind === 'mine' ? '矿脉' : '鱼群'} · 生活等级 ${g.level} · ${g.id}`, value: g.id }));
const monsterOptions = computed(() =>
  (cfg.value?.monsterGroups || []).map(g => ({ label: `野外怪物编队 · 等级 ${g.level} · ${g.id}`, value: g.id }))
);
const selectedBossRewards = computed(() => cfg.value?.bossRewards.filter(r => r.level === bossLevel.value) || []);
const bossLabels = ['参与奖励', '伤害第 11 名以后', '伤害第 4～10 名', '伤害第 2～3 名', '伤害第 1 名', '最后一击奖励'];
function accept(row: FieldTemplate) {
  selected.value = structuredClone(row);
  original.value = JSON.stringify(selected.value);
  conflict.value = false;
  bossLevel.value = 1;
}
function confirmAction(content: string): Promise<boolean> {
  return new Promise(resolve => {
    if (!window.$dialog) {
      error.value = '确认组件暂不可用，请刷新后重试';
      resolve(false);
      return;
    }
    window.$dialog.warning({
      title: '确认操作',
      content,
      positiveText: '确认',
      negativeText: '取消',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false)
    });
  });
}
async function confirmLeave() {
  if (saving.value) {
    error.value = '正在保存或发布，请稍候再切换';
    return false;
  }
  return !dirty.value || (await confirmAction('当前全局方案有未保存修改，确认放弃？'));
}
async function choose(row: FieldTemplate) {
  if (dirty.value && !(await confirmAction('放弃当前方案未保存的修改？'))) return;
  accept(row);
}
async function loadTemplates() {
  const data = await api<{ items: FieldTemplate[]; resources: FieldResource[] }>('/fields/templates');
  templates.value = data.items;
  resources.value = data.resources || [];
  if (!selected.value && data.items[0]) accept(data.items[0]);
}
async function loadDeployments() {
  deployments.value = [];
  if (!scope.serverId) return;
  const data = await api<FieldDeployments>(scope.serverQuery('/fields/deployments'));
  if (data.serverId === scope.serverId) deployments.value = data.items;
}
async function load() {
  loading.value = true;
  error.value = '';
  try {
    await loadTemplates();
    await loadDeployments();
  } catch (e) {
    error.value = errorMessage(e);
  } finally {
    loading.value = false;
  }
}
async function save(asNew = false) {
  if (!selected.value || !canWrite.value || saving.value) return;
  if (!selected.value.name.trim()) {
    error.value = '请填写方案名称';
    return;
  }
  saving.value = true;
  error.value = '';
  success.value = '';
  try {
    const body = {
      id: asNew ? '' : selected.value.id,
      version: asNew ? '' : selected.value.version,
      name: selected.value.name.trim(),
      configuration: selected.value.configuration
    };
    const data = await api<FieldTemplate>('/fields/templates', json('POST', body));
    accept(data);
    await loadTemplates();
    success.value = '全局方案已保存；区服需重新发布后生效。';
  } catch (e) {
    error.value = errorMessage(e);
    conflict.value = e instanceof ApiError && e.status === 409;
  } finally {
    saving.value = false;
  }
}
async function remove() {
  if (!selected.value || !(await confirmAction('删除此方案？已部署的方案会受到保护。'))) return;
  saving.value = true;
  error.value = '';
  try {
    await api('/fields/templates', json('DELETE', { id: selected.value.id, version: selected.value.version }));
    selected.value = undefined;
    original.value = '';
    await loadTemplates();
  } catch (e) {
    error.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
async function deploy(row: FieldDeployment) {
  const template = templates.value.find(t => t.id === row.templateId);
  const serverId = scope.serverId;
  if (!template || !serverId) return;
  if (row.enabled && !row.available) {
    error.value = '该地图客户端场景资源缺失，暂未开放';
    return;
  }
  if (
    !(await confirmAction(
      `将“${template.name}”发布至 ${scope.currentServer?.name || scope.serverId}？${row.enabled ? '地图将开放。' : '地图将关闭新进入和新采集/战斗，已接受的回执保留。'}`
    ))
  )
    return;
  saving.value = true;
  error.value = '';
  success.value = '';
  try {
    const data = await api<FieldDeployments>(
      '/fields/deployments',
      json('PUT', {
        serverId,
        scene: row.scene,
        templateId: template.id,
        templateVersion: template.version,
        version: row.version,
        enabled: row.enabled
      })
    );
    if (scope.serverId === serverId && data.serverId === serverId) {
      deployments.value = data.items;
      success.value = '区服野外配置已发布，后继进入和刷新使用新方案。';
    }
  } catch (e) {
    error.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
const removeGuard = scope.addSwitchGuard(confirmLeave);
onBeforeRouteLeave(confirmLeave);
onBeforeUnmount(removeGuard);
watch(
  () => scope.serverId,
  () => {
    error.value = '';
    success.value = '';
    loadDeployments().catch(e => {
      error.value = errorMessage(e);
    });
  }
);
watch(
  () => route.query.mode,
  value => {
    mode.value = value === 'deployments' ? 'deployments' : 'templates';
  }
);
onMounted(load);
</script>

<template>
  <div class="p-4 space-y-4">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <h1 class="text-xl font-semibold">野外管理</h1>
        <p class="mt-1 text-sm text-gray-500">
          先配置完整地图资源组合，再按区服发布。每张地图同时包含场景、矿物、钓鱼、小怪与首领奖励。
        </p>
      </div>
      <NButton :loading="loading" :disabled="saving || dirty" @click="load">刷新</NButton>
    </div>
    <NAlert v-if="error" type="error" closable @close="error = ''">
      {{ error }}
      <NButton
        v-if="conflict"
        text
        @click="
          selected = undefined;
          load();
        "
      >
        重新载入最新版
      </NButton>
    </NAlert>
    <NAlert v-if="success" type="success" closable @close="success = ''">{{ success }}</NAlert>
    <NTabs v-model:value="mode" type="segment">
      <NTab name="templates">全局 · 地图资源方案</NTab>
      <NTab name="deployments">区服 · 野外部署</NTab>
    </NTabs>
    <template v-if="mode === 'templates'">
      <div class="field-workspace">
        <NCard size="small" title="资源方案" class="self-start">
          <NInput v-model:value="search" placeholder="搜索地图或方案" clearable class="mb-3" />
          <div class="space-y-2">
            <button
              v-for="row in filtered"
              :key="row.id"
              class="w-full border rounded p-3 text-left transition hover:border-blue-400"
              :class="
                selected?.id === row.id
                  ? 'border-blue-500 bg-blue-50 dark:bg-blue-950'
                  : 'border-gray-200 bg-white dark:border-gray-700 dark:bg-gray-900'
              "
              @click="choose(row)"
            >
              <span class="block font-medium">{{ row.name }}</span>
              <span class="text-xs text-gray-500">
                {{ row.available ? '场景资源齐备' : '暂未开放 · 场景资源缺失' }}
              </span>
            </button>
          </div>
        </NCard>
        <NCard v-if="selected && cfg" size="small">
          <div class="mb-4 flex flex-wrap items-center gap-2">
            <NInput
              v-model:value="selected.name"
              :disabled="readonly"
              class="min-w-50 flex-1"
              maxlength="80"
              placeholder="方案名称"
            />
            <NTag :type="dirty ? 'warning' : 'default'">{{ dirty ? '未保存' : '已保存' }}</NTag>
            <NButton :disabled="readonly || !dirty" type="primary" @click="save()">保存修改</NButton>
            <NButton :disabled="readonly" @click="save(true)">另存为新方案</NButton>
            <NButton :disabled="readonly" @click="remove">删除</NButton>
          </div>
          <NAlert v-if="!selected.available" type="warning" class="mb-4">
            {{ selected.unavailableReason }}。可整理资源方案，区服暂不能启用。
          </NAlert>
          <NTabs v-model:value="activeTab" type="line" animated>
            <NTabPane name="overview" tab="地图与规则">
              <NDescriptions :column="1" label-placement="left" bordered>
                <NDescriptionsItem label="原地图">{{ cfg.scene }}</NDescriptionsItem>
                <NDescriptionsItem label="场景资源">{{ cfg.sceneResource }}</NDescriptionsItem>
                <NDescriptionsItem label="战斗资源">{{ cfg.battleResource }}</NDescriptionsItem>
              </NDescriptions>
              <NForm label-placement="left" label-width="140" class="mt-5">
                <NFormItem label="开放等级">
                  <NInputNumber v-model:value="cfg.minLevel" :min="1" :max="1000" :disabled="readonly" />
                </NFormItem>
                <NFormItem label="允许野外 PK">
                  <NSwitch v-model:value="cfg.allowPk" :disabled="readonly" />
                  <span class="ml-3 text-xs text-gray-500">发布后只允许本服本图 PK，保留原 25 级门槛与同屏、安全区等保护。</span>
                </NFormItem>
                <NFormItem label="同时存在矿点">
                  <NInputNumber
                    v-model:value="cfg.mineCount"
                    :min="0"
                    :max="cfg.minePoints.length"
                    :disabled="readonly"
                  />
                </NFormItem>
                <NFormItem label="同时存在钓鱼点">
                  <NInputNumber
                    v-model:value="cfg.fishCount"
                    :min="0"
                    :max="cfg.fishPoints.length"
                    :disabled="readonly"
                  />
                </NFormItem>
              </NForm>
              <NAlert type="info">
                矿物与鱼群从原候选点无放回随机分布，按权重刷新。客户端坐标与场景资源保持原组合。
              </NAlert>
            </NTabPane>
            <NTabPane name="gather" tab="矿物与钓鱼">
              <NCollapse>
                <NCollapseItem
                  v-for="group in cfg.gatherGroups"
                  :key="group.id"
                  :name="group.id"
                  :title="`${group.id.includes('mine') ? '矿脉' : '鱼群'} · 生活等级 ${group.level} · ${group.id}`"
                >
                  <div class="grid grid-cols-2 mb-4 gap-3 lg:grid-cols-3">
                    <label>
                      生活等级
                      <NInputNumber v-model:value="group.level" :min="1" :max="6" :disabled="readonly" />
                    </label>
                    <label>
                      采集时长 ms
                      <NInputNumber v-model:value="group.gatherMs" :min="1" :max="600000" :disabled="readonly" />
                    </label>
                    <label>
                      刷新时长 ms
                      <NInputNumber v-model:value="group.cooldownMs" :min="1" :max="600000" :disabled="readonly" />
                    </label>
                    <label>
                      每轮次数
                      <NInputNumber v-model:value="group.times" :min="1" :max="100" :disabled="readonly" />
                    </label>
                    <label>
                      消耗疲劳
                      <NInputNumber v-model:value="group.fatigue" :min="1" :max="200" :disabled="readonly" />
                    </label>
                  </div>
                  <DropEditor v-model:value="group.drops" :disabled="readonly" :resources="gatherResources" />
                </NCollapseItem>
              </NCollapse>
            </NTabPane>
            <NTabPane name="points" tab="分布与刷新权重">
              <h3 class="mb-2 font-semibold">矿物候选点</h3>
              <PointEditor :value="cfg.minePoints" :groups="gatherOptions('mine')" :disabled="readonly" />
              <h3 class="mb-2 mt-5 font-semibold">钓鱼候选点</h3>
              <PointEditor :value="cfg.fishPoints" :groups="gatherOptions('fish')" :disabled="readonly" />
              <h3 class="mb-2 mt-5 font-semibold">怪物候选点</h3>
              <PointEditor :value="cfg.monsterPoints" :groups="monsterOptions" :disabled="readonly" />
            </NTabPane>
            <NTabPane name="monsters" tab="小怪与掉落">
              <NCollapse>
                <NCollapseItem
                  v-for="group in cfg.monsterGroups"
                  :key="group.id"
                  :name="group.id"
                  :title="`野外怪物编队 · 等级 ${group.level} · ${group.id}`"
                >
                  <label class="mb-4 block">
                    重生时长 ms
                    <NInputNumber
                      v-model:value="group.cooldownMs"
                      :min="1"
                      :max="86400000"
                      :disabled="readonly"
                      class="mt-1 w-60"
                    />
                  </label>
                  <DropEditor v-model:value="group.drops" :disabled="readonly" :resources="monsterResources" />
                </NCollapseItem>
              </NCollapse>
            </NTabPane>
            <NTabPane name="boss" tab="首领与掉落">
              <div class="mb-4 flex items-center gap-3">
                <span>显示原首领周期</span>
                <NSwitch v-model:value="cfg.bossEnabled" :disabled="readonly" />
                <span class="text-xs text-gray-500">原首领当前出生于雪原，其他地图的该项保持备用。</span>
              </div>
              <NAlert type="info" class="mb-4">
                奖励调整只用于后继首领周期，当前已冻结周期与历史奖励保持。原首领刷新时间与战斗政策继续沿用。
              </NAlert>
              <label class="mb-4 block">
                选择首领等级
                <NInputNumber v-model:value="bossLevel" :min="1" :max="50" class="mt-1 w-48" />
              </label>
              <NCollapse>
                <NCollapseItem
                  v-for="reward in selectedBossRewards"
                  :key="reward.type"
                  :name="reward.type"
                  :title="bossLabels[reward.type] || `原奖励档 ${reward.type}`"
                >
                  <DropEditor
                    v-model:value="reward.drops"
                    :fixed="!reward.weighted"
                    :disabled="readonly"
                    :resources="bossResources"
                  />
                </NCollapseItem>
              </NCollapse>
            </NTabPane>
          </NTabs>
        </NCard>
      </div>
    </template>
    <template v-else>
      <NAlert v-if="!scope.serverId" type="info">请先在顶部选择区服。</NAlert>
      <NCard v-else size="small" :title="`${scope.currentServer?.name || scope.serverId} · 野外部署`">
        <p class="mb-4 text-sm text-gray-500">
          选择全局方案后单独发布。全局修改不会自动覆盖区服，需再次发布；采集中和战斗中的已接受结果保持。
        </p>
        <div class="space-y-4">
          <div
            v-for="row in deployments"
            :key="row.scene"
            class="border border-gray-200 rounded p-4 dark:border-gray-700"
          >
            <div class="mb-3 flex flex-wrap items-center gap-3">
              <strong>{{ row.name }}</strong>
              <NTag :type="row.available ? 'success' : 'warning'">
                {{ row.available ? '资源齐备' : '暂未开放 · 场景缺失' }}
              </NTag>
              <NTag v-if="row.templateChanged" type="warning">全局方案有更新</NTag>
            </div>
            <div class="flex flex-wrap items-center gap-3">
              <NSelect
                v-model:value="row.templateId"
                :disabled="readonly"
                :options="
                  templates.filter(t => t.configuration.scene === row.scene).map(t => ({ label: t.name, value: t.id }))
                "
                filterable
                class="min-w-60 flex-1"
              />
              <NSwitch v-model:value="row.enabled" :disabled="readonly || !row.available" />
              <span>{{ row.enabled ? '开放' : '关闭' }}</span>
              <NButton type="primary" :disabled="readonly" :loading="saving" @click="deploy(row)">发布至本区服</NButton>
            </div>
          </div>
        </div>
      </NCard>
    </template>
  </div>
</template>

<style scoped>
.field-workspace {
  display: grid;
  grid-template-columns: 260px minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}
@media (max-width: 1000px) {
  .field-workspace {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
