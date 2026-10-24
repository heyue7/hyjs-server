<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from 'vue';
import { NButton, NCheckbox } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, errorMessage } from '@/service/operations';
import type { BroadcastAudience } from '@/service/broadcast';
import type { GamePlayer } from '@/service/player-operations';

const props = withDefaults(defineProps<{ value: BroadcastAudience; serverId: number | null; disabled?: boolean }>(), { disabled: false });
const emit = defineEmits<{ 'update:value': [value: BroadcastAudience] }>();

const selected = computed(() => new Set(props.value.playerIds));
const q = ref('');
const page = ref(1);
const pageSize = ref(20);
const total = ref(0);
const rows = ref<GamePlayer[]>([]);
const loading = ref(false);
const error = ref('');
let generation = 0;

const pageAll = computed(() => rows.value.length > 0 && rows.value.every(row => selected.value.has(row.id)));
const pageSome = computed(() => rows.value.some(row => selected.value.has(row.id)) && !pageAll.value);

function setMode(value: string | number | boolean) {
  const mode: 'all' | 'players' = value === 'players' ? 'players' : 'all';
  if (props.disabled || mode === props.value.mode) return;
  // The server accepts explicit ids only in players mode, so switching clears the other branch.
  emit('update:value', { mode, playerIds: mode === 'players' ? [...props.value.playerIds] : [] });
}
function setIds(playerIds: number[]) {
  emit('update:value', { mode: 'players', playerIds });
}
function toggle(id: number, checked: boolean) {
  const next = new Set(selected.value);
  if (checked) next.add(id);
  else next.delete(id);
  setIds([...next]);
}
function togglePage(checked: boolean) {
  const next = new Set(selected.value);
  for (const row of rows.value) {
    if (checked) next.add(row.id);
    else next.delete(row.id);
  }
  setIds([...next]);
}
function clearSelection() {
  setIds([]);
}
async function load() {
  generation += 1;
  const current = generation;
  if (!props.serverId || props.value.mode !== 'players') {
    rows.value = [];
    total.value = 0;
    return;
  }
  loading.value = true;
  error.value = '';
  try {
    const result = await api<{ items: GamePlayer[]; total: number }>(
      `/game-players?${new URLSearchParams({ serverId: String(props.serverId), q: q.value.trim(), page: String(page.value), pageSize: String(pageSize.value) })}`
    );
    if (current === generation) {
      rows.value = result.items;
      total.value = result.total;
    }
  } catch (e) {
    if (current === generation) error.value = errorMessage(e);
  } finally {
    if (current === generation) loading.value = false;
  }
}
function search() {
  page.value = 1;
  load();
}
watch(() => [props.serverId, props.value.mode], () => {
  page.value = 1;
  q.value = '';
  load();
});
onMounted(load);

const columns: DataTableColumns<GamePlayer> = [
  {
    key: 'selection',
    width: 54,
    title: () =>
      h(NCheckbox, {
        checked: pageAll.value,
        indeterminate: pageSome.value,
        disabled: props.disabled || !rows.value.length,
        onUpdateChecked: value => togglePage(Boolean(value))
      }),
    render: row =>
      h(NCheckbox, {
        checked: selected.value.has(row.id),
        disabled: props.disabled,
        onUpdateChecked: value => toggle(row.id, Boolean(value))
      })
  },
  { title: '角色ID', key: 'id', width: 100 },
  { title: '角色名', key: 'name', minWidth: 180, ellipsis: { tooltip: true } },
  { title: '等级', key: 'level', width: 80 },
  { title: 'VIP', key: 'vip', width: 70 }
];
</script>

<template>
  <div class="audience-select">
    <NRadioGroup :value="value.mode" :disabled="disabled" @update:value="setMode">
      <NRadioButton value="all">全部玩家（跨分页）</NRadioButton>
      <NRadioButton value="players">指定玩家</NRadioButton>
    </NRadioGroup>
    <p v-if="value.mode === 'all'" class="audience-hint">
      “全部玩家”会在预览时冻结当前区服全部已创建角色，跨分页生效，不受当前列表页限制；冻结账号和保护账号自动排除，预览后新建角色不追加。
    </p>
    <template v-else>
      <div class="audience-toolbar">
        <NInput v-model:value="q" :disabled="disabled" clearable placeholder="按角色ID或昵称搜索" @keyup.enter="search" />
        <NButton :disabled="disabled" @click="search">搜索</NButton>
      </div>
      <div class="audience-summary">
        <span>已选 <strong>{{ value.playerIds.length }}</strong> 名玩家，跨分页保留</span>
        <NButton text type="primary" :disabled="disabled || !rows.length" @click="togglePage(!pageAll)">{{ pageAll ? '取消本页选择' : '全选本页' }}</NButton>
        <NButton text type="primary" :disabled="disabled || !value.playerIds.length" @click="clearSelection">清空已选</NButton>
      </div>
      <NAlert v-if="error" type="error" :title="error" class="mb-12px" />
      <NDataTable :columns="columns" :data="rows" :loading="loading" :row-key="(row: GamePlayer) => row.id" :bordered="false" size="small" :max-height="320" :scroll-x="500" />
      <div class="audience-pagination">
        <NPagination v-model:page="page" v-model:page-size="pageSize" :page-sizes="[20, 50, 100]" :item-count="total" show-size-picker @update:page="load" @update:page-size="page = 1; load()" />
      </div>
      <p class="audience-hint">勾选会跨分页保留；表头复选框只作用于本页当前 {{ rows.length }} 名玩家。冻结账号和保护账号即使被勾选也会在预览时排除。</p>
    </template>
  </div>
</template>

<style scoped>
.audience-select { display: flex; flex-direction: column; gap: 10px; width: 100%; min-width: 0; }
.audience-hint { margin: 0; color: rgb(var(--base-text-color) / .65); font-size: 13px; line-height: 1.6; }
.audience-toolbar { display: flex; gap: 8px; }
.audience-toolbar :deep(.n-input) { flex: 1; min-width: 0; }
.audience-toolbar > .n-button { flex-shrink: 0; }
.audience-summary { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 12px; color: rgb(var(--base-text-color) / .75); font-size: 13px; }
.audience-summary strong { color: rgb(var(--base-text-color)); }
.audience-pagination { display: flex; justify-content: flex-end; max-width: 100%; overflow-x: auto; }
@media (max-width: 600px) { .audience-summary > span { flex-basis: 100%; } .audience-select :deep(.n-radio-group) { display: flex; flex-wrap: wrap; } }
</style>
