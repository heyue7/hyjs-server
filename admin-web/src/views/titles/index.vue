<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue';
import { NButton, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { onBeforeRouteLeave } from 'vue-router';
import { api, errorMessage, json } from '@/service/operations';
import type { GameTitle } from '@/service/management-operations';
import { useOperations } from '@/composables/useOperations';
import ManagementWorkspace from '@/components/operations/management-workspace.vue';
const { canWrite, loading, error, run } = useOperations();
const rows = ref<GameTitle[]>([]);
const q = ref('');
const draft = ref<GameTitle | null>(null);
const original = ref('');
const saving = ref(false);
const saveError = ref('');
const section = ref('condition');
const effectOptions = [
  { value: 'hp', label: '生命' },
  { value: 'att', label: '攻击' },
  { value: 'pdef', label: '物理防御' },
  { value: 'mdef', label: '魔法防御' },
  { value: 'cri', label: '暴击' },
  { value: 'hit', label: '命中' },
  { value: 'dodge', label: '闪避' },
  { value: 'antiCri', label: '抗暴' },
  { value: 'criFactor', label: '暴击倍率' },
  { value: 'damageAdd', label: '伤害增加' },
  { value: 'damageReduce', label: '伤害减免' }
];
const effectName = (key: string) => effectOptions.find((row) => row.value === key)?.label || key;
const sections = [
  { key: 'condition', label: '称号资格' },
  { key: 'effects', label: '属性与期限' },
  { key: 'publish', label: '展示与状态' }
];
const paramEditable = computed(() => Boolean(draft.value) && [1, 9, 10, 11, 12, 13, 14].includes(draft.value!.type));
const seasonal = computed(() => Boolean(draft.value) && [17, 18].includes(draft.value!.type));
const qualificationName = (value: number) =>
  (
    ({
      1: '上阵人数',
      2: 'SAO角色收集',
      3: 'ALO角色收集',
      4: 'GGO角色收集',
      5: '角色品质收集',
      6: '角色品质收集',
      7: '角色品质收集',
      8: '角色品质收集',
      9: '角色等级',
      10: '主线章节通关',
      11: '宝石工艺等级',
      12: '料理工艺等级',
      13: '试炼最高层',
      14: '竞技胜利排名',
      17: '赛季胜者',
      18: '赛季胜者'
    }) as Record<number, string>
  )[value] || '所属玩法资格';
const dirty = computed(() => Boolean(draft.value) && original.value !== JSON.stringify(draft.value));
const visible = computed(() =>
  rows.value.filter((row) => `${row.name} ${row.id} ${row.notes}`.toLowerCase().includes(q.value.trim().toLowerCase()))
);
async function load() {
  const data = await run(() => api<{ items: GameTitle[] }>('/titles'));
  if (data)
    rows.value = (data.items || []).map((row) => ({
      ...row,
      effects: row.effects || []
    }));
}
function edit(row: GameTitle) {
  draft.value = JSON.parse(JSON.stringify(row));
  original.value = JSON.stringify(draft.value);
  saveError.value = '';
  section.value = 'condition';
}
function back() {
  if (!dirty.value || !canWrite.value) {
    draft.value = null;
    return;
  }
  window.$dialog?.warning({
    title: '放弃未保存的称号修改？',
    positiveText: '放弃',
    negativeText: '继续编辑',
    onPositiveClick: () => {
      draft.value = null;
    }
  });
}
async function save() {
  if (!draft.value || !canWrite.value || saving.value) return;
  saveError.value = draft.value.effects.some((row) => !row.key || !Number.isInteger(row.value) || row.value < 0 || row.value > 2147483647)
    ? '请填写完整的属性及数值'
    : new Set(draft.value.effects.map((row) => row.key)).size !== draft.value.effects.length
      ? '同一种属性只能配置一次'
      : draft.value.effectTime < 0
        ? '持续时间不能为负数'
        : '';
  if (saveError.value) return;
  saving.value = true;
  try {
    await api(`/titles/${encodeURIComponent(draft.value.id)}`, json('PUT', draft.value));
    original.value = JSON.stringify(draft.value);
    draft.value = null;
    window.$message?.success('称号配置已保存');
    await load();
  } catch (e) {
    saveError.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
const columns: DataTableColumns<GameTitle> = [
  {
    title: '称号',
    key: 'name',
    minWidth: 210,
    render: (row) => h('div', [h('strong', row.name), h('div', { class: 'ops-muted mt-4px' }, row.notes)])
  },
  {
    title: '属性',
    key: 'effects',
    minWidth: 240,
    render: (row) =>
      row.effects.length
        ? row.effects
            .map((effect) => `${effectName(effect.key)} ${effect.value > 0 ? '+' : ''}${effect.value}`)
            .join(' · ')
        : '无属性增幅'
  },
  {
    title: '期限',
    key: 'effectTime',
    width: 110,
    render: (row) => (row.effectTime ? `${row.effectTime} 小时` : '永久')
  },
  {
    title: '状态',
    key: 'enabled',
    width: 160,
    render: (row) =>
      h(NSpace, { size: 5 }, () => [
        h(
          NTag,
          {
            size: 'small',
            bordered: false,
            type: row.enabled ? 'success' : 'default'
          },
          () => (row.enabled ? '可获得' : '关闭资格')
        ),
        ...(!row.show ? [h(NTag, { size: 'small', bordered: false }, () => '隐藏')] : [])
      ])
  },
  {
    title: '操作',
    key: 'action',
    width: 100,
    render: (row) =>
      h(NButton, { size: 'small', secondary: true, onClick: () => edit(row) }, () => (canWrite.value ? '编辑' : '查看'))
  }
];
onBeforeRouteLeave(() => {
  if (!dirty.value || !canWrite.value) return true;
  return new Promise<boolean>((resolve) =>
    window.$dialog?.warning({
      title: '离开称号编辑？',
      content: '未保存的修改将丢失。',
      positiveText: '离开',
      negativeText: '继续编辑',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false)
    })
  );
});
function beforeUnload(event: BeforeUnloadEvent) {
  if (dirty.value && canWrite.value) {
    event.preventDefault();
    event.returnValue = '';
  }
}
window.addEventListener('beforeunload', beforeUnload);
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload));
onMounted(load);
</script>

<template>
  <ManagementWorkspace
    :title="draft ? `${canWrite ? '编辑' : '查看'}称号 · ${draft.name}` : '称号管理'"
    subtitle="管理原客户端称号目录，已持有称号的历史属性快照保持原规则"
    :editing="Boolean(draft)"
    :saving="saving"
    :readonly="!canWrite"
    :dirty="dirty"
    :sections="sections"
    v-model:section="section"
    @back="back"
    @save="save"
  >
    <template v-if="!draft" #actions><NButton :loading="loading" @click="load">刷新</NButton></template>
    <template #alert><NAlert v-if="error || saveError" type="error" :title="saveError || error" /></template>
    <NCard v-if="!draft" :bordered="false" class="ops-panel">
      <div class="ops-filters">
        <NInput v-model:value="q" clearable placeholder="搜索称号、ID 或资格说明" class="ops-search" />
      </div>
      <NDataTable
        :columns="columns"
        :data="visible"
        :row-key="(row: GameTitle) => row.id"
        :bordered="false"
        :loading="loading"
        :pagination="{ pageSize: 20 }"
        :scroll-x="900"
      />
      <p class="ops-muted mt-20px">称号标识、名称和类型对应原客户端目录，暂不创建无原资源的新称号。</p>
    </NCard>
    <NForm v-else label-placement="top" :disabled="saving || !canWrite">
      <section v-show="section === 'condition'">
        <h3 class="mb-20px">称号资格</h3>
        <NDescriptions :column="1" label-placement="left" class="mb-24px">
          <NDescriptionsItem label="名称">{{ draft.name }}</NDescriptionsItem>
          <NDescriptionsItem label="原始标识">
            <span class="ops-mono">{{ draft.id }}</span>
          </NDescriptionsItem>
          <NDescriptionsItem label="资格说明">{{ draft.notes || '由该称号所属玩法判断资格' }}</NDescriptionsItem>
          <NDescriptionsItem label="原类型">{{ qualificationName(draft.type) }}</NDescriptionsItem>
        </NDescriptions>
        <NFormItem label="资格目标参数">
          <NInput
            v-model:value="draft.param"
            :disabled="!paramEditable || saving || !canWrite"
            placeholder="沿原称号类型填写，原值为空时可保留"
          />
        </NFormItem>
        <NAlert type="info" title="参数沿原称号类型解释，赛季等特殊称号仍需真实玩法资格，修改参数不会生成资格。" />
      </section>
      <section v-show="section === 'effects'">
        <NAlert
          v-if="seasonal"
          type="info"
          title="赛季称号的属性与期限由真实赛季结果决定，此处只读。"
          class="mb-20px"
        />
        <div class="ops-section-heading">
          <h3>属性增幅</h3>
          <NButton
            size="small"
            :disabled="saving || !canWrite || seasonal || draft.effects.length >= effectOptions.length"
            @click="draft.effects.push({ key: '', value: 0 })"
          >
            添加属性
          </NButton>
        </div>
        <div v-for="(effect, index) in draft.effects" :key="index" class="title-effect-row">
          <NSelect
            v-model:value="effect.key"
            :disabled="seasonal || !canWrite || saving"
            :options="effectOptions"
            filterable
            placeholder="选择属性"
          />
          <NInputNumber
            v-model:value="effect.value"
            :disabled="seasonal || !canWrite || saving"
            :min="0"
            :max="2147483647"
            :precision="0"
            placeholder="按原字段填写整数数值"
          />
          <NButton quaternary :disabled="saving || !canWrite || seasonal" @click="draft.effects.splice(index, 1)">
            移除
          </NButton>
        </div>
        <NEmpty v-if="!draft.effects.length" description="没有属性增幅" />
        <NDivider />
        <NFormItem label="持续时间（0 为永久）">
          <NInputNumber
            v-model:value="draft.effectTime"
            :disabled="seasonal || !canWrite || saving"
            :min="0"
            :precision="0"
          >
            <template #suffix>小时</template>
          </NInputNumber>
        </NFormItem>
      </section>
      <section v-show="section === 'publish'">
        <h3 class="mb-20px">展示与状态</h3>
        <NFormItem label="开放新资格">
          <NSwitch v-model:value="draft.enabled">
            <template #checked>开放</template>
            <template #unchecked>关闭</template>
          </NSwitch>
        </NFormItem>
        <NFormItem label="在客户端目录中展示"><NSwitch v-model:value="draft.show" /></NFormItem>
        <NAlert type="info" title="关闭影响后续获得，已有持有记录和历史属性由服务端保留。" />
      </section>
    </NForm>
  </ManagementWorkspace>
</template>

<style scoped>
.title-effect-row {
  display: grid;
  grid-template-columns: minmax(140px, 1fr) minmax(140px, 1fr) auto;
  gap: 12px;
  margin-bottom: 15px;
}
@media (max-width: 600px) {
  .title-effect-row {
    grid-template-columns: minmax(0, 1fr) auto;
  }
  .title-effect-row > .n-select {
    grid-column: 1/-1;
  }
}
</style>
