<script setup lang="ts">
import { h, onMounted, ref } from 'vue';
import { NButton } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, formatTime } from '@/service/operations';
import type { Audit } from '@/service/operations';
import { useOperations } from '@/composables/useOperations';
const { loading, error, run } = useOperations();
const rows = ref<Audit[]>([]);
const page = ref(1);
const total = ref(0);
const pageSize = ref(25);
const selected = ref<Audit | null>(null);
const open = ref(false);
async function load() {
  const data = await run(() =>
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
onMounted(load);
</script>

<template>
  <div class="ops-page">
    <div class="ops-toolbar">
      <div class="ops-page-label">
        操作审计
        <span class="ops-count">{{ total.toLocaleString() }}</span>
      </div>
      <NButton :loading="loading" @click="load">刷新</NButton>
    </div>
    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false" class="ops-panel">
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
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
  </div>
</template>
