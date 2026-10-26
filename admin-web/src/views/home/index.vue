<script setup lang="ts">
import { onMounted, ref } from 'vue';
import OnlinePlayers from '@/components/operations/online-players.vue';
import { auditActionLabel, auditActorLabel } from '@/service/audit-labels';
import { useAuthStore } from '@/store/modules/auth';
import { createRequestId } from '@/utils/request-id';
import { api, json, ApiError, errorMessage, formatTime } from '@/service/operations';
import type { Audit } from '@/service/operations';
import { useOperations } from '@/composables/useOperations';
const { loading, error, run } = useOperations();
const overview = ref<{
  server: { version: string; status: string; policyVersion: string };
  counts: Record<string, number>;
  recentAudit: Audit[];
} | null>(null);
const auth = useAuthStore();
const controlBusy = ref(false);
const controlRequest = ref<{ id: number; version: string; requestId: string; action: string; reason: string } | null>(null);
async function control(action: 'server_stop' | 'server_start') {
  if (controlBusy.value || !overview.value || !auth.userInfo.roles.includes('admin')) return;
  controlBusy.value = true;
  controlRequest.value ??= { id: 0, version: overview.value.server.policyVersion, requestId: createRequestId(), action, reason: action === 'server_stop' ? '运营概览关服维护' : '运营概览启动游戏服务' };
  error.value = '';
  try {
    await api('/client-updates/action', json('POST', controlRequest.value));
    controlRequest.value = null;
    await load();
  } catch (e) {
    error.value = errorMessage(e);
    if (e instanceof ApiError && e.status >= 400 && e.status < 500 && ![408, 429].includes(e.status)) controlRequest.value = null;
  } finally { controlBusy.value = false; }
}
const metrics = [
  { key: 'players', label: '游戏角色', icon: 'ph:users' },
  { key: 'items', label: '资源目录', icon: 'ph:treasure-chest' },
  { key: 'activities', label: '活动配置', icon: 'ph:calendar-blank' },
  { key: 'announcements', label: '公告', icon: 'ph:megaphone' }
];
async function load() {
  const data = await run(() => api<typeof overview.value>('/overview'));
  if (data) overview.value = data;
}
onMounted(load);
</script>

<template>
  <div class="ops-page">
    <div class="ops-filters"><NButton :loading="loading" @click="load">刷新</NButton></div>
    <NAlert v-if="error" type="error" :title="error" />
    <NSpin :show="loading">
      <template v-if="overview">
        <div class="overview-metrics">
          <div v-for="metric in metrics" :key="metric.key" class="metric">
            <SvgIcon :icon="metric.icon" class="metric-icon" />
            <div>
              <div class="metric-label">{{ metric.label }}</div>
              <div class="metric-value">{{ overview.counts[metric.key]?.toLocaleString() ?? '—' }}</div>
            </div>
          </div>
        </div>
        <OnlinePlayers />
        <NCard title="服务状态" :bordered="false" class="ops-panel">
          <div v-if="auth.userInfo.roles.includes('admin')" class="ops-toolbar mb-16px">
            <span class="ops-muted">关服后拦截游戏登录并断开游戏连接，后台可继续使用。</span>
            <NButton type="error" :disabled="controlBusy || (controlRequest && controlRequest.action !== 'server_stop') || (!controlRequest && overview.server.status === 'stopped')" :loading="controlBusy && controlRequest?.action === 'server_stop'" @click="control('server_stop')">{{ controlRequest?.action === 'server_stop' ? '重试关服' : '关服' }}</NButton>
            <NButton type="primary" :disabled="controlBusy || (controlRequest && controlRequest.action !== 'server_start') || (!controlRequest && overview.server.status !== 'stopped')" :loading="controlBusy && controlRequest?.action === 'server_start'" @click="control('server_start')">{{ controlRequest?.action === 'server_start' ? '重试启动' : '启动' }}</NButton>
          </div>
          <NDescriptions :column="1" label-placement="left">
            <NDescriptionsItem label="运行状态">
              <NTag
                :type="overview.server.status === 'running' || overview.server.status === 'ok' ? 'success' : 'default'"
                size="small"
              >
                {{ ({ running: '运行正常', ok: '运行正常', stopped: '已关服 · 维护中', error: '运行异常' } as Record<string, string>)[overview.server.status] || '状态待确认' }}
              </NTag>
            </NDescriptionsItem>
            <NDescriptionsItem label="运行版本">
              <span class="ops-mono">{{ overview.server.version || '未提供' }}</span>
            </NDescriptionsItem>
          </NDescriptions>
        </NCard>
        <NCard title="最近操作" :bordered="false" class="ops-panel">
          <NEmpty v-if="!overview.recentAudit?.length" description="暂无运营操作" />
          <div v-for="entry in overview.recentAudit" :key="entry.id" class="audit-line">
            <div>
              <strong>{{ auditActorLabel(entry.actor) }}</strong>
              <span :title="entry.action">{{ auditActionLabel(entry.action) }}</span>
              <span v-if="entry.target" class="ops-mono ops-muted" :title="entry.target">{{ entry.target }}</span>
            </div>
            <time>{{ formatTime(entry.time) }}</time>
          </div>
          <div class="ops-section-footer"><RouterLink to="/audit">查看全部操作 →</RouterLink></div>
        </NCard>
      </template>
      <NEmpty v-else-if="!loading && !error" description="暂无概览数据" />
    </NSpin>
  </div>
</template>
