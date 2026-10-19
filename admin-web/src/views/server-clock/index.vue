<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { ApiError, api, errorMessage, formatTime, json } from '@/service/operations';
import type { ServerClock, ServerClockChange } from '@/service/server-clock';
import { useAuthStore } from '@/store/modules/auth';
const auth = useAuthStore();
const canChange = computed(() => auth.userInfo.roles.includes('admin'));
const clock = ref<ServerClock | null>(null);
const loading = ref(false);
const changing = ref(false);
const needsRefresh = ref(false);
const error = ref('');
const pending = ref<ServerClockChange | null>(null);
const blocked = computed(() => loading.value || changing.value || needsRefresh.value || !clock.value?.enabled || !canChange.value);
async function load() {
  if (changing.value) return;
  loading.value = true;
  error.value = '';
  try {
    clock.value = await api<ServerClock>('/server-clock');
    needsRefresh.value = false;
  } catch (e) {
    error.value = errorMessage(e);
  } finally {
    loading.value = false;
  }
}
async function change(deltaDays: -1 | 1) {
  if (!clock.value || blocked.value || (pending.value && pending.value.deltaDays !== deltaDays)) return;
  const request = pending.value || { deltaDays, revision: clock.value.revision, requestId: crypto.randomUUID() };
  pending.value = request;
  changing.value = true;
  error.value = '';
  try {
    clock.value = await api<ServerClock>('/server-clock', json('POST', request));
    pending.value = null;
    window.$message?.success('全服游戏日期已更新');
  } catch (e) {
    error.value = errorMessage(e);
    if (e instanceof ApiError && e.status === 409) {
      pending.value = null;
      needsRefresh.value = true;
    }
  } finally {
    changing.value = false;
  }
}
function serverTime(value: number) {
  try {
    return new Intl.DateTimeFormat('zh-CN', { timeZone: clock.value?.timezone, dateStyle: 'medium', timeStyle: 'medium' }).format(value);
  } catch {
    return formatTime(value);
  }
}
onMounted(load);
</script>

<template>
  <div class="ops-page">
    <div class="ops-toolbar">
      <div class="ops-page-label">游戏时间</div>
      <NButton :loading="loading" :disabled="changing" @click="load">刷新</NButton>
    </div>
    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false" class="ops-panel server-clock-card">
      <NSpin :show="loading">
        <template v-if="clock">
          <div class="server-clock-heading">
            <div>
              <div class="ops-muted mb-8px">全服游戏日期</div>
              <div class="server-clock-date">{{ serverTime(clock.gameTimeMs) }}</div>
            </div>
            <NTag :type="clock.enabled ? 'success' : 'default'" :bordered="false">{{ clock.enabled ? '全服生效' : '调整未启用' }}</NTag>
          </div>
          <NDescriptions :column="2" label-placement="top" class="mt-28px mb-28px">
            <NDescriptionsItem label="运行区服">{{ clock.serverId }} 区</NDescriptionsItem>
            <NDescriptionsItem label="偏移天数">{{ clock.offsetDays > 0 ? '+' : '' }}{{ clock.offsetDays }} 天</NDescriptionsItem>
            <NDescriptionsItem label="实际时间">{{ serverTime(clock.realTimeMs) }}</NDescriptionsItem>
            <NDescriptionsItem label="时区">{{ clock.timezone }}</NDescriptionsItem>
          </NDescriptions>
          <p class="ops-muted mb-24px">日期调整对全服玩法生效。账单、支付和登录仍按实际时间记录。</p>
          <NAlert v-if="!clock.enabled" type="info" title="服务器暂未启用游戏日期调整" />
          <NAlert v-else-if="!canChange" type="info" title="当前账号可查看服务器时间，调整需要管理员权限。" />
          <template v-else>
            <NAlert v-if="needsRefresh" type="info" title="服务器时间已被其他操作更新，请先刷新。" class="mb-20px" />
            <NSpace>
              <NPopconfirm @positive-click="change(-1)">
                <template #trigger>
                  <NButton :disabled="blocked || Boolean(pending && pending.deltaDays !== -1)" :loading="changing && pending?.deltaDays === -1">
                    {{ pending?.deltaDays === -1 && error ? '重试 -1 天' : '-1 天' }}
                  </NButton>
                </template>
                确认将全服游戏日期后退一天？
              </NPopconfirm>
              <NPopconfirm @positive-click="change(1)">
                <template #trigger>
                  <NButton type="primary" :disabled="blocked || Boolean(pending && pending.deltaDays !== 1)" :loading="changing && pending?.deltaDays === 1">
                    {{ pending?.deltaDays === 1 && error ? '重试 +1 天' : '+1 天' }}
                  </NButton>
                </template>
                确认将全服游戏日期前进一天？
              </NPopconfirm>
            </NSpace>
          </template>
        </template>
      </NSpin>
    </NCard>
  </div>
</template>

<style scoped>
.server-clock-card { max-width: 900px; }
.server-clock-heading { display: flex; align-items: center; justify-content: space-between; gap: 20px; }
.server-clock-date { font-size: 26px; line-height: 1.4; font-weight: 600; font-variant-numeric: tabular-nums; }
@media (max-width: 640px) {
  .server-clock-heading { align-items: flex-start; flex-direction: column; }
  .server-clock-date { font-size: 22px; }
}
</style>
