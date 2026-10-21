<script setup lang="ts">
import { computed, h, onBeforeUnmount, ref, watch } from 'vue';
import { useWindowSize } from '@vueuse/core';
import { NButton, NSpace } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, errorMessage } from '@/service/operations';
import {
  inventoryCategories,
  inventoryQualities,
  type PlayerInventoryResponse,
  type PlayerInventoryRow
} from '@/service/player-inventory';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { useOperations } from '@/composables/useOperations';
import ResourceImage from './resource-image.vue';
import PlayerInventoryEditor from './player-inventory-editor.vue';
const props = defineProps<{ playerId: number; playerName: string }>();
const emit = defineEmits<{ changed: [] }>();
const scope = useServerScopeStore();
const { canWrite } = useOperations();
const { width } = useWindowSize();
const editorRef = ref<InstanceType<typeof PlayerInventoryEditor> | null>(null);
defineExpose({
  canLeave: async () => !editor.value || (await editorRef.value?.canLeave())
});
const category = ref('');
const q = ref('');
const quality = ref<number | null>(null);
const page = ref(1);
const pageSize = ref(25);
const data = ref<PlayerInventoryResponse>({
  items: [],
  total: 0,
  version: '',
  categories: []
});
const loading = ref(false);
const error = ref('');
const editor = ref(false);
const operation = ref<'add' | 'update' | 'remove'>('add');
const selected = ref<PlayerInventoryRow | null>(null);
let sequence = 0;
const categories = computed(() => [
  {
    value: '',
    label: `全部分类（${data.value.categories.reduce((s, c) => s + c.count, 0)}）`
  },
  ...(data.value.categories.length ? data.value.categories : inventoryCategories).map(c => ({
    value: c.value,
    label: `${c.label}${'count' in c ? `（${c.count}）` : ''}`
  }))
]);
async function load() {
  const seq = ++sequence;
  loading.value = true;
  error.value = '';
  try {
    const result = await api<PlayerInventoryResponse>(
      scope.serverQuery(
        `/game-players/${props.playerId}/inventory?${new URLSearchParams({ category: category.value, q: q.value, quality: quality.value === null ? '' : String(quality.value), page: String(page.value), pageSize: String(pageSize.value) })}`
      )
    );
    if (seq === sequence) data.value = result;
  } catch (e) {
    if (seq === sequence) error.value = errorMessage(e);
  } finally {
    if (seq === sequence) loading.value = false;
  }
}
function search() {
  page.value = 1;
  load();
}
function edit(kind: 'add' | 'update' | 'remove', row: PlayerInventoryRow | null = null) {
  selected.value = row;
  operation.value = kind;
  editor.value = true;
}
async function saved() {
  await load();
  if (page.value > 1 && !data.value.items.length) {
    page.value--;
    await load();
  }
  emit('changed');
}
const columns = computed<DataTableColumns<PlayerInventoryRow>>(() => [
  {
    title: '资源',
    key: 'name',
    minWidth: 210,
    render: r =>
      h('div', { class: 'inventory-resource' }, [
        h(ResourceImage, {
          item: {
            id: r.resourceId,
            name: r.name,
            category: r.category,
            iconUrl: r.iconUrl
          },
          size: 38
        }),
        h('div', [h('strong', r.name), h('div', { class: 'ops-muted ops-mono' }, r.resourceId)])
      ])
  },
  {
    title: '分类',
    key: 'category',
    width: 100,
    render: r => inventoryCategories.find(c => c.value === r.category)?.label || '其他资源'
  },
  { title: '品质', key: 'qualityLabel', width: 80 },
  {
    title: '数量 / 等级',
    key: 'amount',
    width: 115,
    render: r => `${r.amount.toLocaleString()}${r.level ? ` / Lv.${r.level}` : ''}`
  },
  { title: '实例', key: 'guid', minWidth: 160, ellipsis: { tooltip: true } },
  {
    title: '状态',
    key: 'blockedReason',
    minWidth: 155,
    render: r => r.blockedReason || '可操作'
  },
  {
    title: '操作',
    key: 'action',
    fixed: 'right',
    width: width.value < 600 ? 95 : 180,
    render: r =>
      canWrite.value
        ? h(NSpace, { size: 6 }, () => [
            h(
              NButton,
              {
                size: 'small',
                secondary: true,
                disabled: !r.editable,
                onClick: () => edit('update', r)
              },
              () => '修改属性'
            ),
            h(
              NButton,
              {
                size: 'small',
                type: 'error',
                secondary: true,
                disabled: !r.removable,
                onClick: () => edit('remove', r)
              },
              () => '移除'
            )
          ])
        : '只读'
  }
]);
watch(
  () => [props.playerId, scope.serverId],
  () => {
    sequence++;
    editor.value = false;
    selected.value = null;
    category.value = '';
    q.value = '';
    quality.value = null;
    page.value = 1;
    data.value = { items: [], total: 0, version: '', categories: [] };
    load();
  },
  { immediate: true }
);
onBeforeUnmount(() => {
  sequence++;
});
</script>
<template>
  <div class="inventory-toolbar">
    <NSelect v-model:value="category" :options="categories" @update:value="search" /><NInput
      v-model:value="q"
      placeholder="名称、资源 ID 或实例 GUID"
      clearable
      :input-props="{ 'aria-label': '背包搜索' }"
      @keydown.enter="search"
    /><NSelect
      v-model:value="quality"
      :options="inventoryQualities"
      clearable
      placeholder="全部品质"
      @update:value="search"
    /><NButton @click="search">搜索</NButton><NButton :loading="loading" @click="load">刷新</NButton
    ><NButton v-if="canWrite" type="primary" :disabled="loading || !data.version" @click="edit('add')"
      >添加资源</NButton
    >
  </div>
  <NAlert v-if="error" type="error" :title="error" class="mb-16px" />
  <p class="ops-muted">当前筛选共 {{ data.total.toLocaleString() }} 项</p>
  <NDataTable
    :columns="columns"
    :data="data.items"
    :loading="loading"
    :row-key="(r: PlayerInventoryRow) => r.key"
    :bordered="false"
    :scroll-x="950"
  />
  <div class="ops-pagination">
    <NPagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :item-count="data.total"
      :page-sizes="[25, 50, 100]"
      :page-slot="width < 600 ? 4 : 7"
      show-size-picker
      @update:page="load"
      @update:page-size="search"
    />
  </div>
  <PlayerInventoryEditor
    ref="editorRef"
    v-model:show="editor"
    :player-id="playerId"
    :player-name="playerName"
    :version="data.version"
    :operation="operation"
    :row="selected"
    @saved="saved"
  />
</template>
<style scoped>
.inventory-toolbar {
  display: grid;
  grid-template-columns: 170px minmax(160px, 1fr) 120px auto auto auto;
  gap: 8px;
  margin: 16px 0;
}
:deep(.inventory-resource) {
  display: flex;
  gap: 10px;
  align-items: center;
}
:deep(.inventory-resource strong) {
  overflow-wrap: anywhere;
}
@media (max-width: 700px) {
  .inventory-toolbar {
    grid-template-columns: 1fr 1fr;
  }
  .inventory-toolbar > :nth-child(2) {
    grid-column: 1/-1;
    grid-row: 2;
  }
}
</style>
