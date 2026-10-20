<script setup lang="ts">
import { computed, h, onMounted, reactive, ref, watch } from 'vue';
import { NButton, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, formatTime } from '@/service/operations';
import type { Audit } from '@/service/operations';
import { getSecurityEvents, securityEventOutcomeLabel, securityEventOutcomeOptions } from '@/service/security-events';
import type { SecurityEvent, SecurityEventFilters, SecurityEventOutcome } from '@/service/security-events';
import { useAuthStore } from '@/store/modules/auth';
import { useOperations } from '@/composables/useOperations';
const auth = useAuthStore();
const isAdmin = computed(() => auth.userInfo.roles.includes('admin'));
const activeTab = ref<'operations' | 'security'>('operations');
const { loading: opsLoading, error: opsError, run: runOps } = useOperations();
const { loading: securityLoading, error: securityError, run: runSecurity } = useOperations();

const rows = ref<Audit[]>([]);
const page = ref(1);
const total = ref(0);
const pageSize = ref(25);
const selected = ref<Audit | null>(null);
const open = ref(false);
async function load() {
  const data = await runOps(() =>
    api<{ items: Audit[]; total: number }>(`/audit?page=${page.value}&pageSize=${pageSize.value}`)
  );
  if (data) {
    rows.value = data.items;
    total.value = data.total;
  }
}
const columns: DataTableColumns<Audit> = [
  { title: '时间（北京）', key: 'time', width: 190, render: row => formatTime(row.time) },
  { title: '操作人', key: 'actor', width: 150 },
  { title: '操作', key: 'action', minWidth: 180 },
  { title: '目标', key: 'target', minWidth: 240, ellipsis: { tooltip: true } },
  {
    title: '详情',
    key: 'detail',
    width: 90,
    render: row =>
      h(
        NButton,
        {
          size: 'small',
          quaternary: true,
          onClick: () => {
            selected.value = row;
            open.value = true;
          }
        },
        () => '查看'
      )
  }
];

const securityRows = ref<SecurityEvent[]>([]);
const securityPage = ref(1);
const securityPageSize = ref(25);
const securityTotal = ref(0);
const securityLoaded = ref(false);
const securitySelected = ref<SecurityEvent | null>(null);
const securityOpen = ref(false);
const securityFilters = reactive<SecurityEventFilters>({ action: '', outcome: '', actor: '' });
const securityDraft = reactive<{ action: string; outcome: string; actor: string }>({
  action: '',
  outcome: '',
  actor: ''
});
const securityActionOptions = [
  { label: '全部事件', value: '' },
  { label: '后台登录', value: 'admin.login' },
  { label: '后台退出', value: 'admin.logout' },
  { label: '后台操作与访问', value: 'admin.request' },
  { label: '游戏登录', value: 'game.login' }
];
const securityActionLabels = Object.fromEntries(securityActionOptions.map(item => [item.value, item.label]));
const outcomeTagTypes: Record<string, 'success' | 'error' | 'warning' | 'info' | 'default'> = {
  success: 'success',
  failure: 'error',
  denied: 'warning',
  throttled: 'info'
};
async function loadSecurity() {
  const data = await runSecurity(() => getSecurityEvents(securityPage.value, securityPageSize.value, securityFilters));
  if (data) {
    securityRows.value = data.items;
    securityTotal.value = data.total;
  }
}
function searchSecurity() {
  securityFilters.action = securityDraft.action.trim();
  securityFilters.outcome = (securityDraft.outcome || '') as SecurityEventOutcome;
  securityFilters.actor = securityDraft.actor.trim();
  securityPage.value = 1;
  securityLoaded.value = true;
  loadSecurity();
}
function resetSecurity() {
  securityDraft.action = '';
  securityDraft.outcome = '';
  securityDraft.actor = '';
  searchSecurity();
}
function refresh() {
  if (activeTab.value === 'security' && isAdmin.value) {
    securityLoaded.value = true;
    loadSecurity();
    return;
  }
  load();
}
watch(activeTab, value => {
  if (value === 'security' && isAdmin.value && !securityLoaded.value) {
    securityLoaded.value = true;
    loadSecurity();
  }
});
const securityColumns: DataTableColumns<SecurityEvent> = [
  { title: '时间（北京）', key: 'lastAt', width: 190, render: row => formatTime(row.lastAt) },
  { title: '账号', key: 'actor', width: 150 },
  { title: '来源', key: 'sourceIp', width: 160 },
  { title: '事件', key: 'action', minWidth: 180, render: row => securityActionLabels[row.action] || row.action },
  {
    title: '结果',
    key: 'outcome',
    width: 100,
    render: row =>
      h(NTag, { size: 'small', type: outcomeTagTypes[row.outcome] || 'default' }, () =>
        securityEventOutcomeLabel(row.outcome)
      )
  },
  { title: '次数', key: 'occurrences', width: 80 },
  { title: '目标', key: 'target', minWidth: 220, ellipsis: { tooltip: true } },
  {
    title: '详情',
    key: 'detail',
    width: 90,
    render: row =>
      h(
        NButton,
        {
          size: 'small',
          quaternary: true,
          onClick: () => {
            securitySelected.value = row;
            securityOpen.value = true;
          }
        },
        () => '查看'
      )
  }
];
onMounted(load);
</script>

<template>
  <div class="ops-page">
    <div class="ops-toolbar">
      <div class="ops-page-label">
        {{ activeTab === 'security' ? '安全事件' : '操作审计' }}
        <span class="ops-count">{{ (activeTab === 'security' ? securityTotal : total).toLocaleString() }}</span>
      </div>
      <NButton :loading="activeTab === 'security' ? securityLoading : opsLoading" @click="refresh">刷新</NButton>
    </div>
    <NTabs v-model:value="activeTab" type="line">
      <NTabPane name="operations" tab="运营操作" />
      <NTabPane v-if="isAdmin" name="security" tab="安全事件" />
    </NTabs>
    <template v-if="activeTab === 'operations'">
      <NAlert v-if="opsError" type="error" :title="opsError" />
      <NCard :bordered="false" class="ops-panel">
        <NDataTable
          :columns="columns"
          :data="rows"
          :loading="opsLoading"
          :row-key="(row: Audit) => row.id"
          :bordered="false"
          :scroll-x="850"
        />
        <div class="ops-pagination">
          <NPagination
            v-model:page="page"
            v-model:page-size="pageSize"
            :item-count="total"
            :page-sizes="[25, 50, 100]"
            show-size-picker
            @update:page="load"
            @update:page-size="
              page = 1;
              load();
            "
          />
        </div>
      </NCard>
    </template>
    <template v-else-if="isAdmin">
      <NAlert v-if="securityError" type="error" :title="securityError" />
      <NCard :bordered="false" class="ops-panel">
        <div class="ops-filters">
          <NSelect
            v-model:value="securityDraft.action"
            class="ops-filter-select"
            :options="securityActionOptions"
          />
          <NSelect
            v-model:value="securityDraft.outcome"
            class="ops-filter-select"
            :options="securityEventOutcomeOptions"
          />
          <NInput
            v-model:value="securityDraft.actor"
            class="ops-search"
            clearable
            placeholder="账号或后台用户名"
            @keyup.enter="searchSecurity"
          />
          <NButton type="primary" @click="searchSecurity">查询</NButton>
          <NButton @click="resetSecurity">重置</NButton>
        </div>
        <NDataTable
          :columns="securityColumns"
          :data="securityRows"
          :loading="securityLoading"
          :row-key="(row: SecurityEvent) => row.id"
          :bordered="false"
          :scroll-x="1100"
        />
        <div class="ops-pagination">
          <NPagination
            v-model:page="securityPage"
            v-model:page-size="securityPageSize"
            :item-count="securityTotal"
            :page-sizes="[25, 50, 100]"
            show-size-picker
            @update:page="loadSecurity"
            @update:page-size="
              securityPage = 1;
              loadSecurity();
            "
          />
        </div>
      </NCard>
    </template>
    <NDrawer v-model:show="open" :width="680">
      <NDrawerContent title="操作详情" closable>
        <template v-if="selected">
          <NDescriptions :column="1" label-placement="left" class="mb-24px">
            <NDescriptionsItem label="操作人">{{ selected.actor }}</NDescriptionsItem>
            <NDescriptionsItem label="操作">{{ selected.action }}</NDescriptionsItem>
            <NDescriptionsItem label="目标">{{ selected.target }}</NDescriptionsItem>
            <NDescriptionsItem label="时间">{{ formatTime(selected.time) }}</NDescriptionsItem>
          </NDescriptions>
          <pre class="ops-audit-detail">{{
            typeof selected.detail === 'string' ? selected.detail : JSON.stringify(selected.detail, null, 2)
          }}</pre>
        </template>
      </NDrawerContent>
    </NDrawer>
    <NDrawer v-model:show="securityOpen" :width="680">
      <NDrawerContent title="安全事件详情" closable>
        <template v-if="securitySelected">
          <NDescriptions :column="1" label-placement="left" class="mb-24px">
            <NDescriptionsItem label="账号">{{ securitySelected.actor || '—' }}</NDescriptionsItem>
            <NDescriptionsItem label="来源">{{ securitySelected.sourceIp || '—' }}</NDescriptionsItem>
            <NDescriptionsItem label="事件">{{ securitySelected.action }}</NDescriptionsItem>
            <NDescriptionsItem label="结果">
              {{ securityEventOutcomeLabel(securitySelected.outcome) }}
            </NDescriptionsItem>
            <NDescriptionsItem label="目标">{{ securitySelected.target || '—' }}</NDescriptionsItem>
            <NDescriptionsItem label="原因短码">{{ securitySelected.reasonCode || '—' }}</NDescriptionsItem>
            <NDescriptionsItem label="关联标识">{{ securitySelected.requestId || '—' }}</NDescriptionsItem>
            <NDescriptionsItem label="首次时间">{{ formatTime(securitySelected.firstAt) }}</NDescriptionsItem>
            <NDescriptionsItem label="最近时间">{{ formatTime(securitySelected.lastAt) }}</NDescriptionsItem>
            <NDescriptionsItem label="聚合次数">{{ securitySelected.occurrences }}</NDescriptionsItem>
          </NDescriptions>
        </template>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>
