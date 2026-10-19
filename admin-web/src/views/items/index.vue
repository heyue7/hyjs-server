<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue';
import { NButton, NTag } from 'naive-ui';
import type { DataTableColumns, SelectOption } from 'naive-ui';
import { api, errorMessage, json } from '@/service/operations';
import type { Item, ItemFacets, ItemsResponse } from '@/service/operations';
import { useOperations } from '@/composables/useOperations';
import ResourceImage from '@/components/operations/resource-image.vue';
import ItemQuality from '@/components/operations/item-quality.vue';
const { canWrite, loading, error, run } = useOperations();
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
const selectedItem = ref<Item | null>(null);
const grant = reactive({ playerId: null as number | null, amount: 1, reason: '', requestId: '' });
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
      grantError.value = errorMessage(e);
    } finally {
      if (seq === playerRequest) playerLoading.value = false;
    }
  }, 250);
}
function openGrant(item: Item) {
  selectedItem.value = item;
  Object.assign(grant, { playerId: null, amount: 1, reason: '', requestId: crypto.randomUUID() });
  playerOptions.value = [];
  grantError.value = '';
  grantOpen.value = true;
}
async function submitGrant() {
  if (!grant.playerId || !Number.isInteger(grant.amount) || grant.amount <= 0 || !grant.reason.trim()) {
    grantError.value = '请选择角色、填写正整数数量及发放原因';
    return;
  }
  granting.value = true;
  grantError.value = '';
  try {
    await api('/grants', json('POST', { ...grant, resourceId: selectedItem.value!.id }));
    window.$message?.success('发放成功，请在游戏领奖中心领取');
    grantOpen.value = false;
  } catch (e) {
    grantError.value = errorMessage(e);
  } finally {
    granting.value = false;
  }
}
const qualityColors: NonNullable<Item['qualityColor']>[] = ['gray', 'white', 'green', 'blue', 'purple', 'orange', 'red'];
function renderQuality(option: SelectOption) { return h(ItemQuality, { label: String(option.label), color: qualityColors[Number(option.value)] || 'gray' }); }
const columns: DataTableColumns<Item> = [
  {
    title: '道具',
    key: 'name',
    minWidth: 220,
    render: item =>
      h('div', { class: 'item-name' }, [
        h(ResourceImage, { item, size: 44 }),
        h('div', [h('strong', item.name || item.id), h('div', { class: 'ops-mono ops-muted' }, item.id)])
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
    width: 100,
    render: item =>
      h(
        NButton,
        { size: 'small', secondary: true, disabled: !canWrite.value, onClick: () => openGrant(item) },
        () => '发放'
      )
  }
];
onMounted(load);
</script>

<template>
  <div class="ops-page">
    <div class="ops-toolbar">
      <div class="ops-page-label">
        道具目录
        <span class="ops-count">{{ total.toLocaleString() }}</span>
      </div>
      <NButton :loading="loading" @click="load">刷新</NButton>
    </div>
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
      </div>
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :row-key="(row: Item) => row.id"
        :scroll-x="1050"
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
    <NModal
      v-model:show="grantOpen"
      preset="card"
      title="发放道具"
      class="ops-modal"
      :mask-closable="!granting"
      :closable="!granting"
    >
      <div class="grant-object">
        <ResourceImage :item="selectedItem" :size="60" />
        <strong>{{ selectedItem?.name }}</strong>
        <span class="ops-mono ops-muted">{{ selectedItem?.id }}</span>
      </div>
      <NAlert v-if="grantError" type="error" :title="grantError" class="mb-16px" />
      <NForm label-placement="top">
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
        <NFormItem label="数量" required>
          <NInputNumber v-model:value="grant.amount" :min="1" :max="1000000" :precision="0" class="w-full" />
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
          <NButton :disabled="granting" @click="grantOpen = false">取消</NButton>
          <NPopconfirm :disabled="!grant.playerId || !grant.reason.trim() || granting" @positive-click="submitGrant">
            <template #trigger>
              <NButton type="primary" :disabled="!grant.playerId || !grant.reason.trim()" :loading="granting">
                确认发放
              </NButton>
            </template>
            向角色 ID {{ grant.playerId }} 发放 {{ selectedItem?.name }} × {{ grant.amount }}？
          </NPopconfirm>
        </div>
      </template>
    </NModal>
  </div>
</template>
