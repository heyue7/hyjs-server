<script setup lang="ts">
import { computed, onMounted, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { serverScopedRoutes, useServerScopeStore } from '@/store/modules/server-scope';
const route = useRoute();
const router = useRouter();
const scope = useServerScopeStore();
const scoped = computed(() => serverScopedRoutes.has(String(route.name)));
const mixed = computed(() => route.name === 'fields');
const options = computed(() =>
  scope.servers.map(row => ({ label: `${row.id} 区 · ${row.name}${row.enabled ? '' : '（已关闭）'}`, value: row.id }))
);
let queryGeneration = 0;
async function alignQuery() {
  const value = scope.serverId ? String(scope.serverId) : undefined;
  if (route.query.serverId === value) return;
  const query = { ...route.query };
  if (value) query.serverId = value;
  else delete query.serverId;
  await router.replace({ path: route.path, query, hash: route.hash });
}
async function syncQuery() {
  if (route.query.serverId === undefined || !scope.loaded) return;
  queryGeneration += 1;
  const generation = queryGeneration;
  const raw = route.query.serverId;
  const value = typeof raw === 'string' && /^\d+$/.test(raw) ? Number(raw) : null;
  const valid = value && Number.isSafeInteger(value) && scope.servers.some(row => row.id === value) ? value : null;
  await scope.requestServerChange(valid);
  if (generation === queryGeneration) await alignQuery();
}
async function changeServer(id: number | null) {
  queryGeneration += 1;
  if (await scope.requestServerChange(id)) await alignQuery();
}
onMounted(scope.loadServers);
watch(() => [route.query.serverId, scope.loaded], syncQuery, { immediate: true });
</script>

<template>
  <div class="server-scope-control">
    <NTag size="small" :type="scoped ? 'success' : mixed ? 'warning' : 'info'" :bordered="false">
      {{ scoped ? '区服运营' : mixed ? '方案 / 区服' : '全局设置' }}
    </NTag>
    <NSelect
      :value="scope.serverId"
      :options="options"
      :loading="scope.loading"
      filterable
      clearable
      placeholder="选择区服"
      size="small"
      class="server-scope-select"
      aria-label="当前运营区服"
      @update:value="changeServer"
    />
    <NTooltip v-if="scope.error">
      <template #trigger><NButton size="tiny" type="error" @click="scope.loadServers">重试</NButton></template>
      {{ scope.error }}
    </NTooltip>
  </div>
</template>

<style scoped>
.server-scope-control {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 0 12px;
}
.server-scope-select {
  width: 200px;
}
@media (max-width: 740px) {
  .server-scope-control {
    padding: 0 4px;
    gap: 4px;
  }
  .server-scope-select {
    width: 145px;
  }
  .server-scope-control :deep(.n-tag) {
    display: none;
  }
}
</style>
