<script setup lang="ts">
import ContextHelp from '@/components/operations/context-help.vue';
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue';
import { NButton, NImage, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { onBeforeRouteLeave } from 'vue-router';
import { api, errorMessage, json } from '@/service/operations';
import type { GameTitle } from '@/service/management-operations';
import { useOperations } from '@/composables/useOperations';
import ManagementWorkspace from '@/components/operations/management-workspace.vue';
const { canWrite, loading, error, run } = useOperations();
const rows = ref<GameTitle[]>([]);
const q = ref('');
const chapters = ref<{ value: string; label: string }[]>([]);
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
const targetLabels: Record<number, [string, string, number?]> = {
  1: ['同时上阵人数', '名角色', 4], 9: ['角色达到等级', '级'],
  11: ['珠宝工艺等级', '级', 10], 12: ['料理工艺等级', '级', 6],
  13: ['试炼通关层数', '层'], 14: ['竞技场进入前多少名', '名']
};
const target = computed(() => targetLabels[draft.value?.type || 0]);
const targetNumber = computed({
  get: () => draft.value?.param ? Number(draft.value.param) : null,
  set: (value: number | null) => { if (draft.value) draft.value.param = value === null ? '' : String(value); }
});
const currentRequirement = computed(() => {
  const d = draft.value; if (!d) return '';
  if (d.type === 1) return `同时上阵 ${d.param || '—'} 名角色`;
  if (d.type === 9) return `角色等级达到 ${d.param || '—'} 级`;
  if (d.type === 10) return `通关${chapters.value.find(c => c.value === d.param)?.label || `第 ${d.param} 章`}`;
  if (d.type === 11 || d.type === 12) return `${d.type === 11 ? '珠宝' : '料理'}工艺达到 ${d.param || '—'} 级`;
  if (d.type === 13) return `试炼之地通关第 ${d.param || '—'} 层`;
  if (d.type === 14) return `竞技场获胜并达到前 ${d.param || '—'} 名`;
  return d.requirement || d.notes;
});
const seasonal = computed(() => Boolean(draft.value) && [17, 18].includes(draft.value!.type));
const dirty = computed(() => Boolean(draft.value) && original.value !== JSON.stringify(draft.value));
const visible = computed(() =>
  rows.value.filter((row) => `${row.name} ${row.id} ${row.notes}`.toLowerCase().includes(q.value.trim().toLowerCase()))
);
async function load() {
  const data = await run(() => api<{ items: GameTitle[]; chapters: { value: string; label: string }[] }>('/titles'));
  if (data) {
    chapters.value = data.chapters || [];
    rows.value = (data.items || []).map((row) => ({
      ...row,
      effects: row.effects || []
    }));
  }
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
    title: '称号图片', key: 'iconUrl', width: 220,
    render: row => row.iconUrl
      ? h(NImage, { src: row.iconUrl, alt: row.name, width: 190, height: 54, objectFit: 'contain', lazy: true })
      : h('span', { class: 'ops-muted' }, '暂无原图')
  },
  { title: '称号名称', key: 'name', minWidth: 150, render: row => h('strong', row.name) },
  { title: '获得条件', key: 'requirement', minWidth: 260, render: row => row.requirement || row.notes || '—' },
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
          () => (row.enabled ? '已启用' : '已停用')
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
    :editing="Boolean(draft)"
    :saving="saving"
    :readonly="!canWrite"
    :dirty="dirty"
    :sections="sections"
    v-model:section="section"
    @back="back"
    @save="save"
  >
    <template #title-extra><ContextHelp v-if="draft" title="称号配置说明" :text="`称号编号：${draft.id}\n条件和属性仅影响新获得的称号，已持有称号保持原属性和到期时间。`" /></template>
    <template v-if="!draft" #actions><NButton :loading="loading" @click="load">刷新</NButton></template>
    <template #alert><NAlert v-if="error || saveError" type="error" :title="saveError || error" /></template>
    <NCard v-if="!draft" :bordered="false" class="ops-panel">
      <div class="ops-filters">
        <NInput v-model:value="q" clearable placeholder="搜索称号或获得条件" class="ops-search" />
      </div>
      <NDataTable
        :columns="columns"
        :data="visible"
        :row-key="(row: GameTitle) => row.id"
        :bordered="false"
        :loading="loading"
        :pagination="{ pageSize: 20 }"
        :scroll-x="1300"
      />

    </NCard>
    <NForm v-else label-placement="top" :disabled="saving || !canWrite">
      <section v-show="section === 'condition'">
        <div class="title-art-preview">
          <NImage v-if="draft.iconUrl" :src="draft.iconUrl" :alt="draft.name" :height="76" object-fit="contain" />
          <strong>{{ draft.name }}</strong>
        </div>
        <NFormItem v-if="target" :label="target[0]" class="title-target-field">
          <NInputNumber v-model:value="targetNumber" :min="1" :max="target[2] || 2147483647" :precision="0">
            <template #suffix>{{ target[1] }}</template>
          </NInputNumber>
        </NFormItem>
        <NFormItem v-else-if="draft.type === 10" label="需通关的主线章节">
          <NSelect v-model:value="draft.param" :options="chapters" filterable placeholder="选择章节" />
        </NFormItem>
        <div class="title-requirement"><span class="ops-muted">获得条件</span><p>{{ currentRequirement }}</p></div>
        <p v-if="seasonal" class="ops-inline-note">赛季结果确认后发放。</p>
        <p class="ops-inline-note">修改仅影响新获得的称号，已持有称号保持原属性和期限。</p>
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
            placeholder="增幅数值"
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
          <NSwitch v-model:value="draft.enabled" aria-label="开放新资格" />
        </NFormItem>
        <NFormItem label="在客户端目录中展示"><NSwitch v-model:value="draft.show" /></NFormItem>
        <p class="ops-inline-note">关闭后不能再新获得，已持有的称号不受影响。</p>
      </section>
    </NForm>
  </ManagementWorkspace>
</template>

<style scoped>
.title-list-art { display: flex; flex-direction: column; align-items: flex-start; gap: 3px; padding-block: 6px; }
.title-art-preview { display: flex; flex-direction: column; align-items: flex-start; gap: 8px; margin-bottom: 28px; }
.title-art-preview :deep(img) { max-width: min(100%, 360px); }
.title-target-field :deep(.n-input-number) { width: min(100%, 320px); }
.title-requirement { margin: 16px 0 24px; }
.title-requirement p { margin: 7px 0 0; font-size: 15px; }

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
