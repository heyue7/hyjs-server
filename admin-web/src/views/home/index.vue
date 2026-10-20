<script setup lang="ts">
import { onMounted, ref } from 'vue';
import { auditActionLabel, auditActorLabel } from '@/service/audit-labels';
import { api, formatTime } from '@/service/operations';
import type { Audit } from '@/service/operations';
import { useOperations } from '@/composables/useOperations';
const { loading, error, run } = useOperations();
const overview = ref<{
  server: { version: string; status: string };
  counts: Record<string, number>;
  recentAudit: Audit[];
} | null>(null);
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
        <NCard title="服务状态" :bordered="false" class="ops-panel">
          <NDescriptions :column="1" label-placement="left">
            <NDescriptionsItem label="运行状态">
              <NTag
                :type="overview.server.status === 'running' || overview.server.status === 'ok' ? 'success' : 'default'"
                size="small"
              >
                {{ ({ running: '运行正常', ok: '运行正常', stopped: '已停止', error: '运行异常' } as Record<string, string>)[overview.server.status] || '状态待确认' }}
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
