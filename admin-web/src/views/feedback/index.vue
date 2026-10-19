<script setup lang="ts">
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue';
import { NButton, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { onBeforeRouteLeave } from 'vue-router';
import { ApiError, api, errorMessage, formatTime, json } from '@/service/operations';
import type { FeedbackTicket } from '@/service/management-operations';
import { useOperations } from '@/composables/useOperations';
import { useServerScopeStore } from '@/store/modules/server-scope';
const scope = useServerScopeStore();
const { canWrite, loading, error, run } = useOperations();
const q = ref('');
const state = ref<string | null>(null);
const page = ref(1);
const pageSize = ref(20);
const total = ref(0);
const rows = ref<FeedbackTicket[]>([]);
const states = [
  { value: 'open', label: '待处理' },
  { value: 'processing', label: '处理中' },
  { value: 'replied', label: '已回复' },
  { value: 'closed', label: '已关闭' }
];
const stateLabel = (value: string) => states.find((row) => row.value === value)?.label || value;
const categoryLabel = (value: string) =>
  (
    ({
      bug: '问题反馈',
      suggestion: '建议',
      account: '账号问题',
      other: '其他'
    }) as Record<string, string>
  )[value] ||
  value ||
  '其他';
const stateType = (value: string): 'warning' | 'success' | 'info' | 'default' =>
  (({ open: 'warning', processing: 'info', replied: 'success' }) as const)[value as 'open'] || 'default';
const detail = ref<FeedbackTicket | null>(null);
const detailLoading = ref(false);
const detailError = ref('');
const reply = ref('');
const replyRequestId = ref('');
const replyPending = ref(false);
const replySentUncertain = ref(false);
const newState = ref('');
const changingState = ref(false);
const actionError = ref('');
const busy = computed(() => replyPending.value || changingState.value);
let detailGeneration = 0;
async function load() {
  const params = new URLSearchParams({
    q: q.value,
    page: String(page.value),
    pageSize: String(pageSize.value)
  });
  if (state.value) params.set('state', state.value);
  const data = await run(() => api<{ items: FeedbackTicket[]; total: number }>(scope.serverQuery(`/feedback?${params}`)));
  if (data) {
    rows.value = data.items;
    total.value = data.total;
  }
}
function search() {
  page.value = 1;
  load();
}
async function fetchDetail(row: FeedbackTicket) {
  const current = ++detailGeneration;
  detailLoading.value = true;
  detailError.value = '';
  try {
    const result = await api<FeedbackTicket>(scope.serverQuery(`/feedback/${encodeURIComponent(row.id)}`));
    if (current === detailGeneration) {
      detail.value = result;
      newState.value = result.state;
    }
  } catch (e) {
    if (current === detailGeneration) detailError.value = errorMessage(e);
  } finally {
    if (current === detailGeneration) detailLoading.value = false;
  }
}
function select(row: FeedbackTicket) {
  if (busy.value || detail.value?.id === row.id) return;
  const change = () => {
    reply.value = '';
    replyRequestId.value = '';
    replySentUncertain.value = false;
    actionError.value = '';
    detail.value = row;
    fetchDetail(row);
  };
  if (reply.value.trim()) {
    window.$dialog?.warning({
      title: '切换反馈？',
      content: '当前未发送的回复将被清空。',
      positiveText: '切换',
      negativeText: '继续回复',
      onPositiveClick: change
    });
  } else change();
}
async function sendReply() {
  if (!detail.value || !canWrite.value || busy.value) return;
  if (!reply.value.trim()) {
    actionError.value = '请填写回复内容';
    return;
  }
  replyPending.value = true;
  actionError.value = '';
  if (!replyRequestId.value) replyRequestId.value = crypto.randomUUID();
  try {
    await api(
      scope.serverQuery(`/feedback/${encodeURIComponent(detail.value.id)}/replies`),
      json('POST', {
        version: detail.value.version,
        content: reply.value.trim(),
        requestId: replyRequestId.value
      })
    );
    reply.value = '';
    replyRequestId.value = '';
    replySentUncertain.value = false;
    window.$message?.success('回复已发送');
    await fetchDetail(detail.value);
    await load();
  } catch (e) {
    actionError.value = errorMessage(e);
    replySentUncertain.value = !(e instanceof ApiError && e.status >= 400 && e.status < 500);
    if (!replySentUncertain.value) replyRequestId.value = '';
  } finally {
    replyPending.value = false;
  }
}
async function changeState() {
  if (!detail.value || !canWrite.value || busy.value || newState.value === detail.value.state) return;
  changingState.value = true;
  actionError.value = '';
  try {
    await api(
      scope.serverQuery(`/feedback/${encodeURIComponent(detail.value.id)}/state`),
      json('PUT', { version: detail.value.version, state: newState.value })
    );
    await fetchDetail(detail.value);
    await load();
    window.$message?.success('处理状态已更新');
  } catch (e) {
    actionError.value = errorMessage(e);
  } finally {
    changingState.value = false;
  }
}
function replyChanged(value: string) {
  reply.value = value;
  if (!replySentUncertain.value) replyRequestId.value = '';
}
const columns: DataTableColumns<FeedbackTicket> = [
  {
    title: '反馈',
    key: 'subject',
    minWidth: 200,
    render: (row) =>
      h('div', [
        h('strong', row.subject || '玩家反馈'),
        h('div', { class: 'ops-muted mt-4px' }, `${row.playerName || '未命名玩家'} · ${row.playerId}`)
      ])
  },
  {
    title: '状态',
    key: 'state',
    width: 95,
    render: (row) =>
      h(NTag, { size: 'small', bordered: false, type: stateType(row.state) }, () => stateLabel(row.state))
  },
  {
    title: '更新',
    key: 'updatedAt',
    width: 170,
    render: (row) => formatTime(row.updatedAt || row.createdAt)
  },
  {
    title: '操作',
    key: 'action',
    width: 85,
    render: (row) =>
      h(
        NButton,
        {
          size: 'small',
          secondary: true,
          disabled: busy.value,
          onClick: () => select(row)
        },
        () => (detail.value?.id === row.id ? '已选' : '查看')
      )
  }
];
function leaveGuard() {
  if (!reply.value.trim()) return true;
  return new Promise<boolean>((resolve) =>
    window.$dialog?.warning({
      title: '离开客服反馈？',
      content: '当前未发送的回复将被清空。',
      positiveText: '离开',
      negativeText: '继续回复',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false)
    })
  );
}
onBeforeRouteLeave(leaveGuard);
const removeSwitchGuard = scope.addSwitchGuard(() => busy.value ? false : leaveGuard());
function beforeUnload(event: BeforeUnloadEvent) {
  if (reply.value.trim()) {
    event.preventDefault();
    event.returnValue = '';
  }
}
window.addEventListener('beforeunload', beforeUnload);
onBeforeUnmount(() => {
  removeSwitchGuard();
  detailGeneration += 1;
  window.removeEventListener('beforeunload', beforeUnload);
});
onMounted(load);
</script>

<template>
  <div class="ops-page feedback-workspace">
    <div class="ops-toolbar">
      <div>
        <div class="ops-page-label">
          客服反馈
          <span class="ops-count">{{ total }}</span>
        </div>
        <p class="ops-muted mt-6px">按反馈查看玩家消息、回复与处理状态</p>
      </div>
      <NButton :loading="loading" @click="load">刷新列表</NButton>
    </div>
    <NAlert v-if="error" type="error" :title="error" />
    <div class="feedback-layout">
      <NCard :bordered="false" class="ops-panel feedback-list">
        <div class="ops-filters">
          <NInput
            v-model:value="q"
            clearable
            placeholder="搜索主题、玩家或反馈内容"
            class="ops-search"
            @keyup.enter="search"
          />
          <NSelect
            v-model:value="state"
            :options="states"
            clearable
            placeholder="全部状态"
            class="ops-filter-select"
            @update:value="search"
          />
          <NButton :loading="loading" @click="search">搜索</NButton>
        </div>
        <NDataTable
          :columns="columns"
          :data="rows"
          :loading="loading"
          :row-key="(row: FeedbackTicket) => row.id"
          :bordered="false"
          :scroll-x="540"
        />
        <div class="ops-pagination">
          <NPagination
            v-model:page="page"
            v-model:page-size="pageSize"
            :item-count="total"
            :page-sizes="[20, 50]"
            show-size-picker
            @update:page="load"
            @update:page-size="search"
          />
        </div>
      </NCard>
      <NCard :bordered="false" class="ops-panel feedback-detail">
        <NEmpty v-if="!detail" description="从列表选择一条反馈" />
        <template v-else>
          <div class="feedback-detail-heading">
            <div>
              <h2>{{ detail.subject || '玩家反馈' }}</h2>
              <p class="ops-muted">
                {{ detail.playerName || '未命名玩家' }} · 角色 {{ detail.playerId }} ·
                {{ categoryLabel(detail.category) }}
              </p>
            </div>
            <NButton size="small" :disabled="busy" :loading="detailLoading" @click="fetchDetail(detail)">刷新</NButton>
          </div>
          <div class="feedback-meta">
            <NTag :type="stateType(detail.state)" :bordered="false">{{ stateLabel(detail.state) }}</NTag>
            <span class="ops-muted">{{ formatTime(detail.createdAt) }}</span>
            <span class="ops-mono ops-muted">#{{ detail.id }}</span>
          </div>
          <NAlert v-if="detailError" type="error" :title="detailError" class="mb-20px" />
          <NSpin :show="detailLoading">
            <div class="feedback-conversation">
              <article
                v-for="message in detail.messages || []"
                :key="message.id"
                class="feedback-message"
                :class="{ support: message.sender === 'support' }"
              >
                <div class="feedback-message-label">
                  <strong>{{ message.sender === 'support' ? '客服回复' : '玩家' }}</strong>
                  <time>{{ formatTime(message.createdAt) }}</time>
                </div>
                <p>{{ message.content }}</p>
              </article>
              <NEmpty v-if="!detailLoading && !detail.messages?.length" description="暂无可读消息" />
            </div>
          </NSpin>
          <div v-if="canWrite" class="feedback-actions">
            <NAlert v-if="actionError" type="error" :title="actionError" class="mb-16px" />
            <div class="feedback-state">
              <NSelect
                v-model:value="newState"
                :options="states"
                :disabled="busy || detailLoading"
                aria-label="处理状态"
              />
              <NButton
                :loading="changingState"
                :disabled="busy || detailLoading || newState === detail.state"
                @click="changeState"
              >
                更新状态
              </NButton>
            </div>
            <label class="feedback-reply-label" for="feedback-reply">回复玩家</label>
            <NInput
              id="feedback-reply"
              :value="reply"
              :disabled="busy || detailLoading || replySentUncertain"
              type="textarea"
              :maxlength="2000"
              show-count
              :autosize="{ minRows: 5, maxRows: 12 }"
              placeholder="输入回复，发送后记录到本条反馈"
              @update:value="replyChanged"
            />
            <p v-if="replySentUncertain" class="ops-muted mt-10px">
              上次发送结果尚未确认，请刷新查看后再重试。
            </p>
            <div class="feedback-send">
              <NButton
                type="primary"
                :loading="replyPending"
                :disabled="busy || detailLoading || !reply.trim()"
                @click="sendReply"
              >
                {{ replySentUncertain ? '重试同一回复' : '发送回复' }}
              </NButton>
              <NButton
                v-if="replySentUncertain"
                :disabled="busy"
                @click="
                  replySentUncertain = false;
                  replyRequestId = '';
                  reply = '';
                "
              >
                清空输入
              </NButton>
            </div>
          </div>
          <p v-else class="ops-muted mt-20px">当前账号可查看反馈与回复记录</p>
        </template>
      </NCard>
    </div>
  </div>
</template>

<style scoped>
.feedback-layout {
  display: grid;
  grid-template-columns: minmax(480px, 1.15fr) minmax(360px, 1fr);
  gap: 22px;
  align-items: start;
}
.feedback-list,
.feedback-detail {
  min-width: 0;
}
.feedback-list .ops-filters {
  gap: 10px;
}
.feedback-list .ops-search {
  flex: 1;
  min-width: 180px;
}
.feedback-list .ops-filter-select {
  width: 130px;
}
.feedback-detail-heading {
  display: flex;
  justify-content: space-between;
  align-items: start;
  gap: 14px;
}
.feedback-detail-heading h2 {
  font-size: 18px;
  font-weight: 600;
  margin: 0 0 7px;
  overflow-wrap: anywhere;
}
.feedback-detail-heading p {
  margin: 0;
  line-height: 1.6;
}
.feedback-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
  margin: 20px 0 24px;
}
.feedback-conversation {
  display: flex;
  flex-direction: column;
  gap: 18px;
  max-height: 560px;
  overflow: auto;
}
.feedback-message {
  padding: 17px;
  background: rgb(var(--base-text-color) / 0.035);
  border-radius: 9px;
}
.feedback-message.support {
  background: rgb(var(--primary-color) / 0.065);
  border-left: 2px solid rgb(var(--primary-color) / 0.45);
}
.feedback-message-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
  color: rgb(var(--base-text-color) / 0.55);
}
.feedback-message-label strong {
  font-weight: 500;
}
.feedback-message p {
  margin: 11px 0 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: 1.8;
}
.feedback-actions {
  border-top: 1px solid rgb(var(--base-text-color) / 0.08);
  padding-top: 22px;
  margin-top: 28px;
}
.feedback-state {
  display: flex;
  gap: 12px;
  margin-bottom: 24px;
}
.feedback-state > .n-select {
  flex: 1;
  max-width: 200px;
}
.feedback-reply-label {
  display: block;
  font-size: 13px;
  margin-bottom: 10px;
}
.feedback-send {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 14px;
}
@media (max-width: 1150px) {
  .feedback-layout {
    grid-template-columns: minmax(0, 1fr);
  }
  .feedback-detail {
    scroll-margin-top: 15px;
  }
  .feedback-conversation {
    max-height: none;
  }
}
</style>
