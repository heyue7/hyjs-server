<script setup lang="ts">
import { useDraftGuard } from '@/composables/useDraftGuard';
import { computed, h, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import { NButton, NTag } from 'naive-ui';
import type { DataTableColumns, SelectOption } from 'naive-ui';
import { api, ApiError, errorMessage, json } from '@/service/operations';
import type { Item, ItemFacets, ItemsResponse } from '@/service/operations';
import { useOperations } from '@/composables/useOperations';
import ResourceImage from '@/components/operations/resource-image.vue';
import ResourceBatchLines from '@/components/operations/resource-batch-lines.vue';
import { validResourceBatch, type ResourceBatchLine } from '@/service/resource-batch';
import ItemQuality from '@/components/operations/item-quality.vue';
import { getItemDetail } from '@/service/item-details';
import type { ItemDetail } from '@/service/item-details';
import { createRequestId } from '@/utils/request-id';
const { canWrite, loading, error, run } = useOperations();
const detailLabels: Record<string, string> = {
  '原表未提供可明确解释的独立数值，不补造属性或效果': '暂无独立属性或效果数据',
  '原目录未提供可读简介': '暂无简介',
  '未恢复该道具的独立原数值表；不推算属性或效果': '暂无独立属性或效果数据',
  '原精灵表的基础生命、攻击、防御字段已标记废弃，不作为当前实际属性展示': '基础生命、攻击、防御字段已停用，不计入当前属性',
  '原料理表仅列影响属性，单份增幅及累计上限见当前生效配置': '单份增幅及累计上限见当前生效数值',
  '当前生效配置；与游戏食用时使用同一料理规则': '当前游戏内食用规则'
};
function detailValue(value: string) { return detailLabels[value] || value || '未提供'; }

const q = ref('');
const category = ref<string | null>(null);
const quality = ref<string | number | null>(null);
const applicableType = ref<string | null>(null);
const subtype = ref<string | null>(null);
const facets = ref<ItemFacets>({ categories: [], qualities: [], applicableTypes: [], subtypes: [] });
const page = ref(1);
const pageSize = ref(25);
const total = ref(0);
const rows = ref<Item[]>([]);
const categories = ref<string[]>([]);
async function load() {
  const params = new URLSearchParams({
    q: q.value,
    category: category.value || '',
    applicableType: applicableType.value || '',
    subtype: subtype.value || '',
    page: String(page.value),
    pageSize: String(pageSize.value)
  });
  if (quality.value != null && quality.value !== '') params.set('quality', String(quality.value));
  const data = await run(() => api<ItemsResponse>(`/items?${params}`));
  if (data) {
    rows.value = data.items;
    total.value = data.total;
    categories.value = data.categories || [];
    if (data.facets) facets.value = data.facets;
  }
}
function search() {
  page.value = 1;
  load();
}
const categoryLabels: Record<string, string> = {
  currency: '货币',
  character: '角色',
  'character-exp': '角色经验',
  equipment: '装备',
  accessory: '饰品',
  pet: '精灵',
  'pet-equipment': '精灵装备',
  material: '生活材料',
  item: '道具',
  fragment: '碎片',
  gem: '宝石',
  cooking: '料理'
};
const categoryOptions = computed(() =>
  facets.value.categories?.length
    ? facets.value.categories
    : categories.value.map(value => ({ label: categoryLabels[value] || value, value }))
);
function clearFilters() {
  q.value = '';
  category.value = null;
  quality.value = null;
  applicableType.value = null;
  subtype.value = null;
  search();
}
const grantOpen = ref(false);
const granting = ref(false);
const grantError = ref('');
const checkedItems = ref<Item[]>([]);
const batch = ref<ResourceBatchLine[]>([]);
const checkedKeys = computed(() => checkedItems.value.map(item => item.id));
function updateChecked(keys: (string | number)[]) {
  const selected = new Set(keys);
  const next = [...checkedItems.value.filter(item => selected.has(item.id)), ...rows.value.filter(item => selected.has(item.id) && !checkedItems.value.some(old => old.id === item.id))];
  if (next.length > 20) window.$message?.warning('每批最多选择20项道具');
  checkedItems.value = next.slice(0, 20);
}
function changeAmount(id: string, value: number | null) {
  const line = batch.value.find(line => line.item.id === id);
  if (line) line.amount = value;
}
function removeItem(id: string) { batch.value = batch.value.filter(line => line.item.id !== id); }
const grantPending = ref<Record<string, unknown> | null>(null);
const grantUncertain = ref(false);
const grant = reactive({ playerId: null as number | null, amount: 1, reason: '', requestId: '' });
const grantGuard = useDraftGuard({ active: () => grantOpen.value, value: () => ({ ...grant, resources: batch.value }), busy: () => granting.value, writable: () => canWrite.value, label: '发放草稿', close: () => { grantOpen.value = false; } });
const playerOptions = ref<{ label: string; value: number }[]>([]);
const playerLoading = ref(false);
let playerTimer: ReturnType<typeof setTimeout>;
let playerRequest = 0;
function findPlayers(value: string) {
  clearTimeout(playerTimer);
  playerTimer = setTimeout(async () => {
    playerRequest += 1;
    const seq = playerRequest;
    playerLoading.value = true;
    try {
      const data = await api<{
        items: { id?: number; playerId?: number; name?: string; playerName?: string; level?: number }[];
      }>(`/players?q=${encodeURIComponent(value)}`);
      if (seq === playerRequest)
        playerOptions.value = data.items.map(p => ({
          value: p.playerId || p.id || 0,
          label: `${p.name || p.playerName || '角色'} · ID ${p.playerId || p.id}${p.level ? ` · Lv.${p.level}` : ''}`
        }));
    } catch (e) {
      if (seq === playerRequest) grantError.value = errorMessage(e);
    } finally {
      if (seq === playerRequest) playerLoading.value = false;
    }
  }, 250);
}
function openGrant(item?: Item) {
  if (grantOpen.value) { window.$message?.warning('请先完成当前发放草稿'); return; }
  playerRequest += 1;
  clearTimeout(playerTimer);
  playerLoading.value = false;
  batch.value = (item ? [item] : checkedItems.value).map(resource => ({ item: resource, amount: 1 }));
  Object.assign(grant, { playerId: null, amount: 1, reason: '', requestId: createRequestId() });
  grantPending.value = null;
  grantUncertain.value = false;
  playerOptions.value = [];
  grantError.value = '';
  grantOpen.value = true;
  grantGuard.capture();
}
async function submitGrant() {
  if (granting.value || !canWrite.value) return;
  if (!grantPending.value) {
    if (!grant.playerId || !validResourceBatch(batch.value) || !grant.reason.trim()) {
      grantError.value = '请选择角色、填写发放原因并核对各项数量';
      return;
    }
    grantPending.value = {
      playerId: grant.playerId,
      resources: batch.value.map(line => ({ resourceId: line.item.id, amount: line.amount })),
      reason: grant.reason.trim(), requestId: grant.requestId
    };
  }
  granting.value = true;
  grantError.value = '';
  try {
    await api<{ deliveryId: string; requestId: string }>('/grants/batch', json('POST', grantPending.value));
    window.$message?.success('发放成功，请在游戏领奖中心领取');
    checkedItems.value = checkedItems.value.filter(item => !batch.value.some(line => line.item.id === item.id));
    grantOpen.value = false;
  } catch (e) {
    grantUncertain.value = !(e instanceof ApiError) || e.status >= 500 || e.status < 400;
    if (!grantUncertain.value) { grantPending.value = null; grant.requestId = createRequestId(); }
    grantError.value = errorMessage(e);
  } finally {
    granting.value = false;
  }
}
const detailOpen = ref(false);
const detailLoading = ref(false);
const detailError = ref('');
const detail = ref<ItemDetail | null>(null);
const detailItem = ref<Item | null>(null);
// Match the detail service's section/field pairs; unrelated dynamic values stay intact.
const detailSections = computed(() => (detail.value?.sections || []).map(section => {
  if (section.title === '数据依据') return {
    title: '数值范围',
    fields: section.fields.map(field => field.label === '来源'
      ? { label: '基础数值', value: '不含角色养成加成' }
      : field)
  };
  if (section.title === '道具说明') return {
    ...section,
    fields: section.fields.filter(field => !(field.label === '来源' && field.value === '本服新增 GGO 道具'))
  };
  return {
    ...section,
    fields: section.fields.filter(field => !(section.title === '当前生效数值' && field.label === '配置来源'))
  };
}).filter(section => section.fields.length));
let detailGeneration = 0;
async function showDetail(item: Item) {
  const current = ++detailGeneration;
  detailItem.value = item; detail.value = null; detailError.value = ''; detailLoading.value = true; detailOpen.value = true;
  try { const result = await getItemDetail(item.id); if (current === detailGeneration) detail.value = result; }
  catch (e) { if (current === detailGeneration) detailError.value = errorMessage(e); }
  finally { if (current === detailGeneration) detailLoading.value = false; }
}
function closeDetail() { detailOpen.value = false; detailGeneration += 1; }
onBeforeUnmount(() => { detailGeneration += 1; playerRequest += 1; clearTimeout(playerTimer); });
const qualityColors: NonNullable<Item['qualityColor']>[] = ['gray', 'white', 'green', 'blue', 'purple', 'orange', 'red'];
function renderQuality(option: SelectOption) { return h(ItemQuality, { label: String(option.label), color: qualityColors[Number(option.value)] || 'gray' }); }
const columns: DataTableColumns<Item> = [
  { type: 'selection', disabled: item => !canWrite.value || (checkedItems.value.length >= 20 && !checkedKeys.value.includes(item.id)) },
  {
    title: '道具',
    key: 'name',
    minWidth: 220,
    render: item =>
      h('div', { class: 'item-name' }, [
        h(ResourceImage, { item, size: 44 }),
        h('div', [h(NButton, { text: true, type: 'primary', onClick: () => showDetail(item) }, () => item.name || item.id), h('div', { class: 'ops-mono ops-muted' }, item.id)])
      ])
  },
  {
    title: '分类',
    key: 'category',
    width: 130,
    render: item =>
      h(NTag, { size: 'small', bordered: false }, () => facets.value.categories.find(option => option.value === item.category)?.label || categoryLabels[item.category] || item.category || '未分类')
  },
  {
    title: '品质',
    key: 'quality',
    width: 100,
    render: item => h(ItemQuality, { item })
  },
  {
    title: '适用类型',
    key: 'applicableType',
    width: 150,
    render: item =>
      item.applicableTypeLabel ||
      item.applicableTypes
        ?.map(value => facets.value.applicableTypes.find(option => option.value === value)?.label || value)
        .join('、') ||
      item.applicableType ||
      '—'
  },
  { title: '说明', key: 'description', minWidth: 230, ellipsis: { tooltip: true } },
  {
    title: '操作',
    key: 'action',
    width: 160,
    render: item => h('div', { class: 'ops-table-actions' }, [h(NButton, { size: 'small', secondary: true, onClick: () => showDetail(item) }, () => '详情'),
      h(
        NButton,
        { size: 'small', secondary: true, disabled: !canWrite.value, onClick: () => openGrant(item) },
        () => '发放'
      )])
  }
];
onMounted(load);
</script>

<template>
  <div class="ops-page">

    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false" class="ops-panel">
      <div class="ops-filters">
        <NInput v-model:value="q" clearable placeholder="道具名称或 ID" class="ops-search" @keydown.enter="search" />
        <NSelect
          v-model:value="category"
          :options="categoryOptions"
          clearable
          placeholder="全部分类"
          class="ops-filter-select"
          @update:value="search"
        />
        <NSelect
          v-if="facets.qualities?.length"
          v-model:value="quality"
          :options="facets.qualities"
          :render-label="renderQuality"
          clearable
          placeholder="全部品质"
          class="ops-filter-select"
          @update:value="search"
        />
        <NSelect
          v-if="facets.applicableTypes?.length"
          v-model:value="applicableType"
          :options="facets.applicableTypes"
          clearable
          filterable
          placeholder="全部适用类型"
          class="ops-filter-select"
          @update:value="search"
        />
        <NSelect
          v-if="facets.subtypes?.length"
          v-model:value="subtype"
          :options="facets.subtypes"
          clearable
          filterable
          placeholder="全部子类"
          class="ops-filter-select"
          @update:value="search"
        />
        <NButton type="primary" @click="search">搜索</NButton>
        <NButton quaternary @click="clearFilters">重置</NButton>
        <span class="ops-muted">共 {{ total.toLocaleString() }} 项资源</span>
        <NButton :loading="loading" @click="load">刷新</NButton>
        <NButton v-if="canWrite" type="primary" :disabled="!checkedItems.length" @click="openGrant()">批量发放{{ checkedItems.length ? `（${checkedItems.length}）` : '' }}</NButton>
        <NButton v-if="checkedItems.length" quaternary @click="checkedItems = []">清空选择</NButton>
      </div>
      <NDataTable
        :checked-row-keys="checkedKeys"
        @update:checked-row-keys="updateChecked"
        :columns="columns"
        :data="rows"
        :loading="loading"
        :row-key="(row: Item) => row.id"
        :scroll-x="1110"
        :bordered="false"
      />
      <div class="ops-pagination">
        <NPagination
          v-model:page="page"
          v-model:page-size="pageSize"
          :item-count="total"
          show-size-picker
          :page-sizes="[25, 50, 100]"
          @update:page="load"
          @update:page-size="search"
        />
      </div>
    </NCard>
    <NDrawer :show="detailOpen" width="min(780px, 100vw)" @update:show="!$event && closeDetail()">
      <NDrawerContent :title="(detail?.item.name || detailItem?.name || '道具') + ' · 详情'" closable>
        <NSpin :show="detailLoading">
          <div class="item-detail-heading"><ResourceImage :item="detail?.item || detailItem" :size="64" /><div><h2>{{ detail?.item.name || detailItem?.name }}</h2><ItemQuality :item="detail?.item || detailItem || undefined" /><div class="ops-mono ops-muted mt-8px">{{ detailItem?.id }}</div></div></div>
          <NAlert v-if="detailError" type="error" :title="detailError" /><NButton v-if="detailError && detailItem" class="mt-12px" @click="showDetail(detailItem)">重新加载</NButton>
          <section v-for="section in detailSections" :key="section.title" class="item-detail-section"><h3>{{ section.title }}</h3><dl><template v-for="field in section.fields" :key="field.label"><dt>{{ field.label }}</dt><dd>{{ detailValue(field.value) }}</dd></template></dl></section>
        </NSpin>
        <template #footer><div class="ops-drawer-actions"><NButton @click="closeDetail">关闭</NButton><NButton v-if="canWrite && detailItem" type="primary" @click="openGrant(detailItem)">发放此道具</NButton></div></template>
      </NDrawerContent>
    </NDrawer>
    <NModal
      :show="grantOpen"
      @update:show="!$event && grantGuard.requestClose()"
      preset="card"
      title="发放道具"
      class="ops-modal grant-modal"
      :style="{ width: 'min(680px, calc(100vw - 24px))', maxHeight: 'calc(100dvh - 32px)' }"
      :content-style="{ overflowY: 'auto' }"
      :mask-closable="!granting"
      :closable="!granting"
    >
      <NAlert v-if="grantError" type="error" :title="grantError" class="mb-16px" />
      <NAlert v-if="grantUncertain" type="warning" title="提交结果待确认，请重试原请求。" class="mb-16px" />
      <ResourceBatchLines :lines="batch" :disabled="granting || grantUncertain" @amount="changeAmount" @remove="removeItem" />
      <NForm label-placement="top" :disabled="granting || grantUncertain">
        <NFormItem label="目标角色" required>
          <NSelect
            v-model:value="grant.playerId"
            :options="playerOptions"
            filterable
            remote
            :loading="playerLoading"
            placeholder="输入角色名或 ID 查找"
            @search="findPlayers"
          />
        </NFormItem>
        <NFormItem label="发放原因" required>
          <NInput
            v-model:value="grant.reason"
            type="textarea"
            :maxlength="200"
            show-count
            placeholder="本次发放的用途"
          />
        </NFormItem>
      </NForm>
      <div class="ops-muted mb-20px">道具将发送到该角色的领奖中心。</div>
      <template #footer>
        <div class="ops-drawer-actions">
          <NButton :disabled="granting" @click="grantGuard.requestClose()">取消</NButton>
          <NPopconfirm :disabled="!grant.playerId || !grant.reason.trim() || !validResourceBatch(batch) || granting" @positive-click="submitGrant">
            <template #trigger>
              <NButton type="primary" :disabled="!grant.playerId || !grant.reason.trim() || !validResourceBatch(batch)" :loading="granting">
                {{ grantUncertain ? '重试原请求' : '确认发放' }}
              </NButton>
            </template>
            向角色 ID {{ grant.playerId }} 发放 {{ batch.length }} 项道具？
          </NPopconfirm>
        </div>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
.grant-modal{display:flex;flex-direction:column}.grant-modal :deep(.n-card__content){min-height:0;overflow-y:auto}.grant-modal :deep(.n-card__footer){flex-shrink:0}
@media(max-width:600px){.ops-drawer-actions{flex-wrap:wrap}.ops-drawer-actions :deep(.n-button){flex:1}}
:deep(.item-name){display:flex;gap:10px;align-items:center}:deep(.item-name>div){min-width:0}:deep(.item-name .n-button__content){white-space:normal;text-align:left;overflow-wrap:anywhere}.item-detail-heading{display:flex;gap:16px;align-items:center;margin-bottom:20px}.item-detail-heading h2{font-size:18px;margin:0 0 8px;overflow-wrap:anywhere}.item-detail-section{padding:20px 0;border-top:1px solid rgb(var(--base-text-color)/.08)}.item-detail-section h3{font-size:15px;margin:0 0 16px}.item-detail-section dl{display:grid;grid-template-columns:160px minmax(0,1fr);gap:10px 16px;margin:0}.item-detail-section dt{color:rgb(var(--base-text-color)/.6)}.item-detail-section dd{margin:0;white-space:pre-wrap;overflow-wrap:anywhere;line-height:1.7}
@media(max-width:600px){.item-detail-heading{align-items:flex-start;gap:12px}.item-detail-heading>div{min-width:0}.item-detail-section dl{grid-template-columns:1fr;gap:6px}.item-detail-section dd{margin-bottom:10px}}
</style>
