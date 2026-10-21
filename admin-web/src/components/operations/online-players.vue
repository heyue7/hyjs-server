<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { errorMessage, formatTime } from '@/service/operations';
import { fetchOnlinePlayers } from '@/service/online-players';
import type { OnlinePlayersResponse } from '@/service/online-players';
import ContextHelp from './context-help.vue';

const snapshot = ref<OnlinePlayersResponse | null>(null);
const loading = ref(false);
const error = ref('');
const show = ref(false);
const serverId = ref<number | null>(null);
const search = ref('');
const query = ref('');
const page = ref(1);
const pageSize = 20;
let request = 0;
const serverOptions = computed(() => [
  { label: '全部区服', value: 0 },
  ...(snapshot.value?.servers.map(row => ({ label: row.name, value: row.serverId })) || [])
]);
const selectedServer = computed({ get: () => serverId.value || 0, set: (value: number) => { serverId.value = value || null; page.value = 1; void load(); } });
const serverName = (id: number) => snapshot.value?.servers.find(row => row.serverId === id)?.name || `${id}区`;
async function load() {
  const current = ++request;
  loading.value = true;
  error.value = '';
  try {
    const result = await fetchOnlinePlayers(serverId.value, query.value, page.value, pageSize);
    if (current === request) snapshot.value = result;
  } catch (cause) {
    if (current === request) error.value = errorMessage(cause);
  } finally {
    if (current === request) loading.value = false;
  }
}
function open(id: number) {
  serverId.value = id;
  search.value = '';
  query.value = '';
  page.value = 1;
  show.value = true;
  void load();
}
function submitSearch() { query.value = search.value.trim(); page.value = 1; void load(); }
function changePage(value: number) { page.value = value; void load(); }
onMounted(load);
onBeforeUnmount(() => { request += 1; });
</script>

<template>
  <NCard :bordered="false" class="ops-panel">
    <template #header>
      <div class="online-heading">
        <span>在线玩家</span>
        <ContextHelp icon-only title="在线人数统计口径" text="按当前有效游戏连接统计，进入副本仍计入在线。数据为刷新时快照；未接入区服暂不统计。" />
      </div>
    </template>
    <template #header-extra><NButton size="small" :loading="loading" :disabled="loading" @click="load">刷新</NButton></template>
    <NAlert v-if="error" type="error" class="online-error" :title="error">
      {{ snapshot ? '当前保留上次快照，可刷新重试。' : '请刷新重试。' }}
    </NAlert>
    <NSpin :show="loading && !snapshot">
      <div class="online-table-wrap">
        <NTable :bordered="false" :single-line="true" size="small">
          <thead><tr><th>区服</th><th>在线人数</th><th class="online-action">操作</th></tr></thead>
          <tbody>
            <tr v-for="row in snapshot?.servers || []" :key="row.serverId">
              <td>{{ row.name }}</td>
              <td><NButton v-if="row.connected" text type="primary" @click="open(row.serverId)">{{ row.online.toLocaleString() }}</NButton><NTag v-else size="small">未接入</NTag></td>
              <td class="online-action"><NButton size="small" :disabled="!row.connected" @click="open(row.serverId)">查看玩家</NButton></td>
            </tr>
          </tbody>
        </NTable>
      </div>
      <NEmpty v-if="snapshot && !snapshot.servers.length" description="暂无区服" />
    </NSpin>
    <div v-if="snapshot" class="ops-muted online-time">快照时间：{{ formatTime(snapshot.observedAt) }}</div>
  </NCard>
  <NDrawer v-model:show="show" :width="760" style="max-width: 100vw">
    <NDrawerContent title="在线玩家" closable>
      <div class="online-filters">
        <NSelect v-model:value="selectedServer" :options="serverOptions" aria-label="筛选区服" class="online-server" />
        <NInput v-model:value="search" placeholder="昵称或角色ID" aria-label="昵称或角色ID" clearable :maxlength="128" @keyup.enter="submitSearch" />
        <NButton @click="submitSearch">查询</NButton>
        <NButton :loading="loading" :disabled="loading" @click="load">刷新</NButton>
      </div>
      <NAlert v-if="error" type="error" class="online-error" :title="error">保留上次快照，请刷新重试。</NAlert>
      <div v-if="snapshot" class="ops-muted online-time">快照时间：{{ formatTime(snapshot.observedAt) }} · 共 {{ snapshot.total }} 名</div>
      <NSpin :show="loading">
        <div class="online-table-wrap">
          <NTable :bordered="false" size="small" class="online-player-table">
            <thead><tr><th>角色ID</th><th>昵称</th><th>区服</th><th>等级</th><th>VIP</th></tr></thead>
            <tbody><tr v-for="row in snapshot?.items || []" :key="row.id"><td class="ops-mono">{{ row.id }}</td><td>{{ row.name }}</td><td>{{ serverName(row.serverId) }}</td><td>{{ row.level }}</td><td>{{ row.vip }}</td></tr></tbody>
          </NTable>
        </div>
        <NEmpty v-if="snapshot && !snapshot.items.length" :description="snapshot.servers.find(row => row.serverId === serverId)?.connected === false ? '该区服尚未接入在线统计' : query ? '没有匹配的在线玩家' : '当前暂无在线玩家'" class="online-empty" />
      </NSpin>
      <template #footer><NPagination :page="page" :page-size="pageSize" :item-count="snapshot?.total || 0" :page-slot="4" @update:page="changePage" /></template>
    </NDrawerContent>
  </NDrawer>
</template>

<style scoped>
.online-heading { display: flex; align-items: center; gap: 6px; }
.online-action { text-align: right; }
.online-time { margin-top: 12px; margin-bottom: 12px; font-size: 12px; }
.online-error { margin-bottom: 12px; }
.online-filters { display: flex; gap: 8px; align-items: center; }
.online-server { flex: 0 0 150px; }
.online-table-wrap { overflow-x: auto; }
.online-player-table { min-width: 520px; }
.online-empty { padding: 24px 0; }
@media (max-width: 600px) {
  .online-filters { flex-wrap: wrap; }
  .online-server { flex: 1 1 100%; }
  .online-filters :deep(.n-input) { flex: 1 1 120px; width: auto; }
}
</style>
