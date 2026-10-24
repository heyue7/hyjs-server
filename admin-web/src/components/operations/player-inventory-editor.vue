<script setup lang="ts">
import { h, ref, watch } from 'vue';
import { NButton } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, ApiError, errorMessage, json, type Item } from '@/service/operations';
import {
  inventoryCategories,
  inventoryQualities,
  type PlayerInventoryRow,
  type PlayerInventoryResponse
} from '@/service/player-inventory';
import { useOperations } from '@/composables/useOperations';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { useDraftGuard } from '@/composables/useDraftGuard';
import { createRequestId } from '@/utils/request-id';
import ResourceImage from './resource-image.vue';
import ResourceBatchLines from './resource-batch-lines.vue';
import { validResourceBatch, type ResourceBatchLine } from '@/service/resource-batch';
const props = defineProps<{
  show: boolean;
  playerId: number;
  playerName: string;
  version: string;
  operation: 'add' | 'update' | 'remove';
  row: PlayerInventoryRow | null;
}>();
const emit = defineEmits<{ 'update:show': [boolean]; saved: [] }>();
const scope = useServerScopeStore();
const { canWrite } = useOperations();
const currentRow = ref<PlayerInventoryRow | null>(null);
const baseline = ref<PlayerInventoryRow | null>(null);
const pendingBody = ref<Record<string, unknown> | null>(null);
const uncertain = ref(false);
const category = ref('');
const q = ref('');
const quality = ref<number | null>(null);
const page = ref(1);
const items = ref<Item[]>([]);
const total = ref(0);
const batch = ref<ResourceBatchLine[]>([]);
function toggleItem(item: Item) {
  if (busy.value || uncertain.value) return;
  if (batch.value.some(line => line.item.id === item.id)) batch.value = batch.value.filter(line => line.item.id !== item.id);
  else if (batch.value.length < 20) batch.value.push({ item, amount: 1 });
  else error.value = '每批最多选择20项资源';
}
function changeAmount(id: string, value: number | null) {
  const line = batch.value.find(line => line.item.id === id);
  if (line) line.amount = value;
}
function removeItem(id: string) { batch.value = batch.value.filter(line => line.item.id !== id); }
const amount = ref<number | null>(1);
const attributes = ref<Record<string, number>>({});
const reason = ref('');
const version = ref('');
const requestId = ref('');
const busy = ref(false);
const loading = ref(false);
const error = ref('');
const conflict = ref(false);
const latest = ref<PlayerInventoryRow | null>(null);
const refreshed = ref(false);
const refreshedResources = ref<{ id: string; name: string; total: number; amount: number }[]>([]);
let sequence = 0;
let catalogueSequence = 0;
const guard = useDraftGuard({
  active: () => props.show,
  value: () => ({
    batch: batch.value,
    amount: amount.value,
    attributes: attributes.value,
    reason: reason.value
  }),
  busy: () => busy.value,
  writable: () => canWrite.value,
  close: () => emit('update:show', false),
  label: '背包操作'
});
defineExpose({ canLeave: guard.canLeave });
const columns: DataTableColumns<Item> = [
  {
    title: '资源',
    key: 'name',
    minWidth: 180,
    render: r =>
      h('div', { class: 'catalogue-item' }, [
        h(ResourceImage, { item: r, size: 36 }),
        h('div', [h('strong', r.name), h('div', { class: 'ops-muted ops-mono' }, r.id)])
      ])
  },
  { title: '品质', key: 'qualityLabel', width: 80 },
  {
    title: '类型',
    key: 'category',
    width: 85,
    render: r => inventoryCategories.find(c => c.value === r.category)?.label || '其他资源'
  },
  {
    title: '选择',
    key: 'action',
    width: 72,
    fixed: 'right',
    render: r =>
      h(
        NButton,
        {
          size: 'small',
          type: batch.value.some(line => line.item.id === r.id) ? 'primary' : 'default',
          disabled: busy.value || uncertain.value || !canWrite.value,
          onClick: () => toggleItem(r)
        },
        () => (batch.value.some(line => line.item.id === r.id) ? '取消' : '选择')
      )
  }
];
async function catalogue() {
  if (busy.value || uncertain.value) return;
  const seq = sequence,
    read = ++catalogueSequence,
    id = props.playerId,
    server = scope.serverId;
  loading.value = true;
  try {
    const data = await api<{ items: Item[]; total: number }>(
      scope.serverQuery(
        `/game-players/${props.playerId}/inventory/catalogue?${new URLSearchParams({ category: category.value, q: q.value, quality: quality.value === null ? '' : String(quality.value), page: String(page.value), pageSize: '10' })}`
      )
    );
    if (seq === sequence && read === catalogueSequence && id === props.playerId && server === scope.serverId) {
      items.value = data.items;
      total.value = data.total;
    }
  } catch (e) {
    if (seq === sequence && read === catalogueSequence) error.value = errorMessage(e);
  } finally {
    if (seq === sequence && read === catalogueSequence) loading.value = false;
  }
}
function search() {
  page.value = 1;
  catalogue();
}
watch(
  () => [props.show, props.playerId, scope.serverId],
  () => {
    sequence++;
    if (!props.show) return;
    busy.value = false;
    loading.value = false;
    currentRow.value = props.row ? JSON.parse(JSON.stringify(props.row)) : null;
    baseline.value = props.row ? JSON.parse(JSON.stringify(props.row)) : null;
    pendingBody.value = null;
    uncertain.value = false;
    category.value = props.row?.category || '';
    q.value = '';
    quality.value = null;
    page.value = 1;
    batch.value = [];
    items.value = [];
    amount.value = props.row?.amount || 1;
    attributes.value = Object.fromEntries((props.row?.attributes || []).map(a => [a.key, a.value]));
    reason.value = '';
    version.value = props.version;
    requestId.value = createRequestId();
    error.value = '';
    conflict.value = false;
    latest.value = null;
    refreshed.value = false;
    refreshedResources.value = [];
    guard.capture();
    if (props.operation === 'add') catalogue();
  }
);
async function refresh() {
  const seq = sequence,
    id = props.playerId,
    server = scope.serverId;
  loading.value = true;
  try {
    const summaries: { id: string; name: string; total: number; amount: number }[] = [];
    if (props.operation === 'add') {
      for (const line of batch.value) {
        const result = await api<PlayerInventoryResponse>(scope.serverQuery(`/game-players/${id}/inventory?${new URLSearchParams({ category: line.item.category, q: line.item.id, page: '1', pageSize: '100' })}`));
        if (seq !== sequence || server !== scope.serverId || id !== props.playerId) return;
        summaries.push({ id: line.item.id, name: line.item.name, total: result.total, amount: result.items.filter(item => item.resourceId === line.item.id).reduce((sum, item) => sum + item.amount, 0) });
      }
    }
    const data = await api<PlayerInventoryResponse>(
      scope.serverQuery(
        `/game-players/${id}/inventory?${new URLSearchParams({ category: currentRow.value?.category || '', q: currentRow.value?.guid || currentRow.value?.resourceId || '', page: '1', pageSize: '100' })}`
      )
    );
    if (seq !== sequence || server !== scope.serverId || id !== props.playerId) return;
    const found =
      data.items.find(r => r.key === currentRow.value?.key) ||
      null;
    latest.value = found;
    refreshedResources.value = summaries;
    version.value = data.version;
    refreshed.value = true;
    conflict.value = false;
    error.value = '';
    if (found && props.operation === 'update') {
      const values: Record<string, number> = {};
      for (const a of found.attributes) {
        const old = baseline.value?.attributes.find(v => v.key === a.key);
        values[a.key] = old && attributes.value[a.key] !== old.value ? attributes.value[a.key]! : a.value;
      }
      if (amount.value === baseline.value?.amount) amount.value = found.amount;
      attributes.value = values;
      currentRow.value = found;
      baseline.value = JSON.parse(JSON.stringify(found));
    }
    if (!uncertain.value) {
      pendingBody.value = null;
      requestId.value = createRequestId();
    }
  } catch (e) {
    if (seq === sequence) error.value = errorMessage(e);
  } finally {
    if (seq === sequence) loading.value = false;
  }
}
function newIntent() {
  if (!refreshed.value || busy.value) return;
  pendingBody.value = null;
  uncertain.value = false;
  requestId.value = createRequestId();
  refreshed.value = false;
  error.value = '';
}
async function save() {
  if (!canWrite.value || busy.value || conflict.value) return;
  const row = currentRow.value;
  if (!pendingBody.value) {
    if (!reason.value.trim()) {
      error.value = '请填写操作原因';
      return;
    }
    if (props.operation === 'add' && !validResourceBatch(batch.value)) {
      error.value = '请选择1至20项资源，并核对每项整数数量';
      return;
    }
    if (
      props.operation === 'update' &&
      !row?.attributes.length &&
      (!Number.isSafeInteger(amount.value) ||
        amount.value! < 1 ||
        amount.value! > 2147483647)
    ) {
      error.value = '请输入范围内的整数数量';
      return;
    }
    if (
      props.operation === 'update' &&
      (row?.attributes || []).some(
        a =>
          !Number.isSafeInteger(attributes.value[a.key]) ||
          attributes.value[a.key]! < a.min ||
          attributes.value[a.key]! > a.max
      )
    ) {
      error.value = '请核对属性范围';
      return;
    }
    const body: Record<string, unknown> = {
      version: version.value,
      requestId: requestId.value,
      operation: props.operation,
      category: row?.category,
      reason: reason.value.trim()
    };
    if (props.operation === 'add') {
      delete body.operation;
      delete body.category;
      body.resources = batch.value.map(line => ({ category: line.item.category, resourceId: line.item.id, amount: line.amount }));
    } else {
      body.key = row?.key;
      if (props.operation === 'update') {
        if (row?.attributes.length) body.attributes = { ...attributes.value };
        else body.amount = amount.value;
      }
    }
    pendingBody.value = body;
  }
  const seq = sequence,
    id = props.playerId,
    server = scope.serverId,
    path = scope.serverQuery(`/game-players/${id}/inventory${props.operation === 'add' ? '/batch' : ''}`);
  busy.value = true;
  error.value = '';
  try {
    await api(path, json('PUT', pendingBody.value));
    if (seq !== sequence || server !== scope.serverId || id !== props.playerId) return;
    emit('saved');
    emit('update:show', false);
    window.$message?.success('背包已更新');
  } catch (e) {
    if (seq !== sequence || server !== scope.serverId || id !== props.playerId) return;
    conflict.value = e instanceof ApiError && e.status === 409;
    uncertain.value = !(e instanceof ApiError) || e.status >= 500 || e.status < 400;
    if (!uncertain.value && !conflict.value) {
      pendingBody.value = null;
      requestId.value = createRequestId();
    }
    error.value = conflict.value ? '背包已更新，请读取最新版本并核对后保存' : errorMessage(e);
  } finally {
    if (seq === sequence) busy.value = false;
  }
}
</script>
<template>
  <NModal
    :show="show"
    preset="card"
    :title="operation === 'add' ? '添加背包资源' : operation === 'update' ? '修改背包属性' : '移除背包资源'"
    class="ops-modal inventory-modal"
    :style="{
      width: 'min(760px, calc(100vw - 24px))',
      maxHeight: 'calc(100dvh - 32px)'
    }"
    :content-style="{ overflowY: 'auto' }"
    :mask-closable="!busy"
    :closable="!busy"
    @update:show="!$event && guard.requestClose()"
  >
    <p class="ops-muted">{{ scope.currentServer?.name }} · {{ playerName }} · ID {{ playerId }}</p>
    <NAlert v-if="error" type="error" :title="error" class="mb-16px" /><NAlert
      v-if="uncertain"
      type="warning"
      title="上次提交结果尚未确认。重试会发送原请求；修改前请读取最新背包并核对是否已经生效。"
      class="mb-16px"
    /><NButton v-if="conflict || uncertain" :loading="loading" @click="refresh">读取最新版本并保留草稿</NButton>
    <NAlert v-if="refreshed" type="info" title="已读取最新版本，请核对草稿后保存" class="mb-16px"
      ><template v-if="operation === 'add'"
        >请核对各项资源当前数量。
        <div v-for="resource in refreshedResources" :key="resource.id">{{ resource.name }} · 当前{{ resource.total > 100 ? '前100行' : '' }}数量 {{ resource.amount }}</div></template
      ><template v-else-if="latest"
        >当前 {{ latest.name }} · 数量 {{ latest.amount }} · {{ latest.guid }}
        <div v-for="a in latest.attributes" :key="a.key">{{ a.label }}：{{ a.value }}</div></template
      ><template v-else>当前对象已不存在，请关闭后重新选择</template></NAlert
    >
    <NButton v-if="uncertain && refreshed" class="mb-16px" @click="newIntent">已核对结果，开始新的操作</NButton>
    <template v-if="operation === 'add'"
      ><div class="inventory-picker-filters">
        <NSelect
          v-model:value="category"
          :options="[{ value: '', label: '全部分类' }, ...inventoryCategories]"
          @update:value="search"
        /><NInput
          v-model:value="q"
          placeholder="名称或资源 ID"
          clearable
          :input-props="{ 'aria-label': '添加资源搜索' }"
          @keydown.enter="search"
        /><NSelect
          v-model:value="quality"
          :options="inventoryQualities"
          clearable
          placeholder="全部品质"
          @update:value="search"
        /><NButton :loading="loading" @click="search">搜索</NButton>
      </div>
      <NDataTable
        :columns="columns"
        :data="items"
        :loading="loading"
        :bordered="false"
        :scroll-x="440"
        :row-key="(r: Item) => r.id"
      />
      <div class="ops-pagination">
        <NPagination v-model:page="page" :page-size="10" :item-count="total" :page-slot="5" @update:page="catalogue" />
      </div>
      <ResourceBatchLines :lines="batch" :disabled="busy || uncertain || !canWrite" @amount="changeAmount" @remove="removeItem" /></template
    >
    <template v-else
      ><p>
        <strong>{{ currentRow?.name }}</strong> · {{ currentRow?.resourceId }}
      </p>
      <p class="ops-mono">{{ currentRow?.guid || '堆叠资源' }} · 当前数量 {{ currentRow?.amount }}</p>
      <NAlert v-if="operation === 'remove'" type="warning" title="将移除所选整行资源，无出售返还。" class="mb-16px"
    /></template>
    <NForm label-placement="top" :disabled="busy || !canWrite || uncertain"
      ><div v-if="operation === 'update' && currentRow?.attributes.length" class="attribute-grid">
        <NFormItem v-for="a in currentRow.attributes" :key="a.key" :label="a.label"
          ><NInputNumber
            v-model:value="attributes[a.key]"
            :min="a.min"
            :max="a.max"
            :precision="0"
            :input-props="{ 'aria-label': a.label }"
            class="w-full"
        /></NFormItem>
      </div>
      <NFormItem v-else-if="operation === 'update'" label="数量" required
        ><NInputNumber
          v-model:value="amount"
          :min="1"
          :max="2147483647"
          :precision="0"
          :input-props="{ 'aria-label': '资源数量' }"
          class="w-full" /></NFormItem
      ><NFormItem label="操作原因" required
        ><NInput
          v-model:value="reason"
          type="textarea"
          :maxlength="200"
          :input-props="{ 'aria-label': '背包操作原因' }" /></NFormItem
    ></NForm>
    <template #footer
      ><div class="ops-drawer-actions">
        <NButton :disabled="busy" @click="guard.requestClose()">取消</NButton
        ><NButton
          v-if="canWrite"
          :type="operation === 'remove' ? 'error' : 'primary'"
          :loading="busy"
          :disabled="loading || conflict || (operation === 'add' && !uncertain && !validResourceBatch(batch)) || (!uncertain && refreshed && operation !== 'add' && !latest)"
          @click="save"
          >{{ uncertain ? '重试原请求' : operation === 'remove' ? '确认移除' : operation === 'add' ? '添加资源' : '保存修改' }}</NButton
        >
      </div></template
    >
  </NModal>
</template>
<style scoped>
.inventory-modal {
  display: flex;
  flex-direction: column;
}
.inventory-modal :deep(.n-card__content) {
  min-height: 0;
  overflow-y: auto;
}
.inventory-modal :deep(.n-card__footer) {
  flex-shrink: 0;
}
.attribute-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}
@media (max-width: 600px) {
  .attribute-grid {
    grid-template-columns: 1fr;
  }
  .ops-drawer-actions {
    display: flex;
    flex-wrap: wrap;
  }
  .ops-drawer-actions :deep(.n-button) {
    flex: 1;
  }
}
.inventory-picker-filters {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.5fr) minmax(0, 1fr) auto;
  gap: 8px;
  margin-bottom: 12px;
}
:deep(.catalogue-item) {
  display: flex;
  gap: 8px;
  align-items: center;
}
@media (max-width: 600px) {
  .inventory-picker-filters {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }
}
</style>
