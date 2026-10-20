<script setup lang="ts">
import { useDraftGuard } from '@/composables/useDraftGuard';
import { h, onMounted, reactive, ref } from 'vue';
import { NButton, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, errorMessage, formatTime, json } from '@/service/operations';
import type { Announcement } from '@/service/operations';
import { useOperations } from '@/composables/useOperations';
const { canWrite, loading, error, run } = useOperations();
const rows = ref<Announcement[]>([]);
const open = ref(false);
const saving = ref(false);
const saveError = ref('');
const selected = ref<Announcement | null>(null);
const draft = reactive({ title: '', content: '', enabled: false, startAt: 0, endAt: 0 });
const editorGuard = useDraftGuard({ active: () => open.value, value: () => draft, busy: () => saving.value, writable: () => canWrite.value, label: '公告修改', close: () => { open.value = false; } });
const preview = ref(false);
async function load() {
  const result = await run(() => api<{ items: Announcement[] }>('/announcements'));
  if (result) rows.value = result.items;
}
function edit(row: Announcement | null) {
  selected.value = row;
  Object.assign(
    draft,
    row
      ? { title: row.title, content: row.content, enabled: row.enabled, startAt: row.startAt, endAt: row.endAt }
      : { title: '', content: '', enabled: false, startAt: 0, endAt: 0 }
  );
  preview.value = false;
  saveError.value = '';
  editorGuard.capture();
  open.value = true;
}
async function save() {
  if (saving.value || !canWrite.value) return;
  if (!draft.title.trim() || !draft.content.trim()) {
    saveError.value = '请填写公告标题和正文';
    return;
  }
  if (draft.startAt && draft.endAt && draft.endAt <= draft.startAt) {
    saveError.value = '结束时间需要晚于开始时间';
    return;
  }
  saving.value = true;
  saveError.value = '';
  try {
    await api(
      selected.value ? `/announcements/${encodeURIComponent(selected.value.id)}` : '/announcements',
      json(selected.value ? 'PUT' : 'POST', { ...draft, version: selected.value?.version || '' })
    );
    window.$message?.success('公告已保存');
    open.value = false;
    await load();
  } catch (e) {
    saveError.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
const columns: DataTableColumns<Announcement> = [
  {
    title: '公告',
    key: 'title',
    minWidth: 260,
    render: row =>
      h('div', [h('strong', row.title), h('div', { class: 'ops-muted announcement-excerpt' }, row.content)])
  },
  {
    title: '状态',
    key: 'enabled',
    width: 100,
    render: row =>
      h(NTag, { type: row.enabled ? 'success' : 'default', bordered: false, size: 'small' }, () =>
        row.enabled ? '已发布' : '草稿'
      )
  },
  { title: '开始', key: 'startAt', width: 180, render: row => (row.startAt ? formatTime(row.startAt) : '立即') },
  { title: '结束', key: 'endAt', width: 180, render: row => (row.endAt ? formatTime(row.endAt) : '不限') },
  {
    title: '操作',
    key: 'action',
    width: 100,
    render: row =>
      h(NButton, { size: 'small', secondary: true, onClick: () => edit(row) }, () => (canWrite.value ? '编辑' : '查看'))
  }
];
onMounted(load);
</script>

<template>
  <div class="ops-page">
    <div class="ops-filters">
      <NSpace>
        <NButton :loading="loading" @click="load">刷新</NButton>
        <NButton v-if="canWrite" type="primary" @click="edit(null)">新建公告</NButton>
      </NSpace>
    </div>
    <NAlert v-if="error" type="error" :title="error" />
    <NCard :bordered="false" class="ops-panel">
      <NDataTable
        :columns="columns"
        :data="rows"
        :loading="loading"
        :row-key="(row: Announcement) => row.id"
        :bordered="false"
        :scroll-x="850"
      />
      <div class="ops-section-footer ops-muted">所有时间均为北京时间</div>
    </NCard>
    <NDrawer :show="open" @update:show="!$event && editorGuard.requestClose()" :width="680" :mask-closable="!saving">
      <NDrawerContent :title="selected ? '编辑公告' : '新建公告'" :closable="!saving">
        <NAlert v-if="saveError" type="error" :title="saveError" class="mb-20px" />
        <NForm label-placement="top" :disabled="!canWrite || saving">
          <NFormItem label="标题" required>
            <NInput v-model:value="draft.title" :maxlength="120" show-count placeholder="公告标题" />
          </NFormItem>
          <NFormItem label="正文" required>
            <NInput
              v-model:value="draft.content"
              type="textarea"
              :autosize="{ minRows: 10, maxRows: 24 }"
              :maxlength="10000"
              show-count
              placeholder="填写在游戏中展示的公告内容"
            />
          </NFormItem>
          <div class="ops-date-grid">
            <NFormItem label="开始时间">
              <NDatePicker
                :value="draft.startAt || null"
                clearable
                type="datetime"
                placeholder="立即生效"
                @update:value="draft.startAt = $event || 0"
              />
            </NFormItem>
            <NFormItem label="结束时间">
              <NDatePicker
                :value="draft.endAt || null"
                clearable
                type="datetime"
                placeholder="不限"
                @update:value="draft.endAt = $event || 0"
              />
            </NFormItem>
          </div>
          <NFormItem label="发布公告">
            <NSwitch v-model:value="draft.enabled" aria-label="发布公告" />
          </NFormItem>
        </NForm>
        <NButton text @click="preview = !preview">{{ preview ? '收起预览' : '预览公告' }}</NButton>
        <article v-if="preview" class="announcement-preview">
          <h3>{{ draft.title || '公告标题' }}</h3>
          <p>{{ draft.content || '公告正文' }}</p>
        </article>
        <template #footer>
          <div class="ops-drawer-actions">
            <NButton :disabled="saving" @click="editorGuard.requestClose()">关闭</NButton>
            <NButton v-if="canWrite" type="primary" :loading="saving" @click="save">保存公告</NButton>
          </div>
        </template>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>
