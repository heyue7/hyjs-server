<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { NList, NListItem } from 'naive-ui';
import type { DialogReactive } from 'naive-ui';
import { useOperations } from '@/composables/useOperations';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { errorMessage } from '@/service/operations';
import { getPersonalOpeningActivities, savePersonalOpeningActivity } from '@/service/personal-opening-activities';
import type { PersonalOpeningActivity } from '@/service/personal-opening-activities';
import ActivityMilestones from './activity-milestones.vue';

const scope = useServerScopeStore();
const { canWrite } = useOperations();
const rows = ref<PersonalOpeningActivity[]>([]);
const selected = ref<PersonalOpeningActivity | null>(null);
const baseline = ref('');
const loading = ref(false);
const saving = ref(false);
const drawerOpen = ref(false);
const error = ref('');
const saveError = ref('');
const drawerWidth = 'min(760px, calc(100vw - 48px))';
let loadedServer: number | null = null;
let lastFocused: HTMLElement | null = null;
let generation = 0;
let removeGuard: (() => void) | undefined;
const serverOptions = computed(() => scope.servers.map(row => ({ value: row.id, label: row.name })));
const dirty = computed(() => Boolean(selected.value && JSON.stringify(selected.value) !== baseline.value));
const disabled = computed(() => !canWrite.value || loading.value || saving.value);
const busy = computed(() => loading.value || saving.value);
function apply(row: PersonalOpeningActivity) {
  const draft: PersonalOpeningActivity = JSON.parse(JSON.stringify(row));
  draft.form.milestones ||= [];
  selected.value = draft;
  baseline.value = JSON.stringify(draft);
  saveError.value = '';
}
function clearEditor() {
  drawerOpen.value = false;
  selected.value = null;
  baseline.value = '';
  saveError.value = '';
}
let pendingDiscard: { promise: Promise<boolean>; dispose: () => void } | null = null;
function confirmDiscard(): Promise<boolean> {
  if (!dirty.value) return Promise.resolve(true);
  if (pendingDiscard) return pendingDiscard.promise;
  if (!window.$dialog) return Promise.resolve(false);
  let resolveConfirmation!: (accepted: boolean) => void;
  const promise = new Promise<boolean>(resolve => { resolveConfirmation = resolve; });
  let accepted = false;
  let decided = false;
  let settled = false;
  let dialog: DialogReactive;
  const settle = (value: boolean) => {
    if (settled) return;
    settled = true;
    pendingDiscard = null;
    resolveConfirmation(value);
  };
  const dismiss = (value: boolean) => {
    if (!decided) { accepted = value; decided = true; }
    dialog.destroy();
    return false;
  };
  dialog = window.$dialog.warning({
    title: '放弃未保存的个人活动修改？',
    content: '只丢弃本次输入，已保存配置和玩家进度不变。',
    positiveText: '放弃修改',
    negativeText: '继续编辑',
    onPositiveClick: () => dismiss(true),
    onNegativeClick: () => dismiss(false),
    onClose: () => dismiss(false),
    onMaskClick: () => dismiss(false),
    onEsc: () => dismiss(false),
    onAfterLeave: () => settle(accepted)
  });
  pendingDiscard = { promise, dispose: () => { dialog.destroy(); settle(false); } };
  return promise;
}
async function openRow(row: PersonalOpeningActivity) {
  if (busy.value) return;
  if (drawerOpen.value && selected.value?.id === row.id) return;
  if (!(await confirmDiscard())) return;
  lastFocused = document.activeElement as HTMLElement | null;
  apply(row);
  drawerOpen.value = true;
  await nextTick();
}
async function closeDrawer() {
  if (saving.value) return;
  if (!(await confirmDiscard())) return;
  drawerOpen.value = false;
}
function drawerShow(value: boolean) {
  if (!value) void closeDrawer();
}
function afterDrawerLeave() {
  if (drawerOpen.value) return;
  selected.value = null;
  baseline.value = '';
  saveError.value = '';
  if (lastFocused?.isConnected) lastFocused.focus();
}
async function leaveWorkspace(): Promise<boolean> {
  if (saving.value) return false;
  if (!(await confirmDiscard())) return false;
  clearEditor();
  return true;
}
async function load() {
  const current = ++generation;
  const server = scope.serverId;
  clearEditor();
  error.value = '';
  if (!server) {
    rows.value = [];
    loadedServer = null;
    loading.value = false;
    return;
  }
  if (loadedServer !== server) rows.value = [];
  loading.value = true;
  try {
    const result = await getPersonalOpeningActivities(server);
    if (current !== generation || scope.serverId !== server) return;
    rows.value = result.items;
    loadedServer = server;
  } catch (cause) {
    if (current === generation) error.value = errorMessage(cause);
  } finally {
    if (current === generation) loading.value = false;
  }
}
async function refresh() {
  if (busy.value) return;
  if (!(await confirmDiscard())) return;
  await load();
}
async function save() {
  if (!selected.value || !scope.serverId || disabled.value || !dirty.value) return;
  const server = scope.serverId;
  const current = generation;
  saving.value = true;
  saveError.value = '';
  try {
    const result = await savePersonalOpeningActivity(server, selected.value);
    if (server !== scope.serverId || current !== generation) return;
    rows.value = rows.value.map(row => row.id === result.id ? result : row);
    apply(result);
    window.$message?.success('个人活动已保存');
  } catch (cause) {
    saveError.value = errorMessage(cause);
  } finally {
    saving.value = false;
  }
}
function beforeUnload(event: BeforeUnloadEvent) {
  if (dirty.value || saving.value) {
    event.preventDefault();
    event.returnValue = '';
  }
}
watch(() => scope.serverId, load);
onBeforeRouteLeave(() => leaveWorkspace());
onMounted(async () => {
  removeGuard = scope.addSwitchGuard(leaveWorkspace);
  window.addEventListener('beforeunload', beforeUnload);
  await scope.loadServers();
  await load();
});
onBeforeUnmount(() => {
  generation += 1;
  removeGuard?.();
  pendingDiscard?.dispose();
  window.removeEventListener('beforeunload', beforeUnload);
});
defineExpose({ beforeLeave: leaveWorkspace, dirty, saving });
</script>

<template>
  <section class="personal-opening ops-page">
    <NAlert v-if="error" type="error" :title="error">
      <NButton text size="small" :loading="loading" :disabled="saving" @click="refresh">重试</NButton>
    </NAlert>
    <NCard :bordered="false" class="ops-panel">
      <div class="ops-filters">
        <NSelect
          :value="scope.serverId"
          :options="serverOptions"
          :disabled="busy"
          placeholder="选择区服"
          aria-label="个人活动所属区服"
          class="ops-filter-select personal-server"
          @update:value="scope.requestServerChange"
        />
        <span class="ops-muted personal-count">{{ !scope.serverId ? '未选择区服' : loading ? '正在载入…' : error && !rows.length ? '载入失败' : `${rows.length} 项活动` }}</span>
        <NButton :loading="loading" :disabled="!scope.serverId || saving" @click="refresh">刷新</NButton>
      </div>
      <NSpin :show="loading">
        <div class="personal-body">
          <NEmpty v-if="!scope.serverId" description="选择区服后查看个人开服活动" />
          <NEmpty v-else-if="!loading && !error && !rows.length" description="本服尚无个人开服活动">
            <template #extra>
              <NButton size="small" @click="refresh">重新加载</NButton>
            </template>
          </NEmpty>
          <NList v-else-if="rows.length" show-divider class="personal-list">
            <NListItem v-for="row in rows" :key="row.id">
              <div class="personal-row">
                <div class="personal-identity">
                  <strong>{{ row.name }}</strong>
                  <NTag size="small" :bordered="false" :type="row.enabled ? 'success' : 'default'">{{ row.enabled ? '已开启' : '已停用' }}</NTag>
                </div>
                <span class="personal-time">第 {{ row.startDay }} 天开始</span>
                <span class="personal-time">持续 {{ row.durationDays }} 天</span>
                <span class="personal-time">领奖宽限 {{ row.claimDays }} 天</span>
                <NButton
                  size="small"
                  secondary
                  class="personal-config"
                  :disabled="busy"
                  :aria-label="`配置${row.name}`"
                  @click="openRow(row)"
                >
                  配置
                </NButton>
              </div>
            </NListItem>
          </NList>
        </div>
      </NSpin>
    </NCard>
    <NDrawer
      :show="drawerOpen"
      :width="drawerWidth"
      placement="right"
      class="personal-opening-drawer"
      :mask-closable="!saving"
      :close-on-esc="!saving"
      :trap-focus="true"
      :auto-focus="true"
      @update:show="drawerShow"
      @after-leave="afterDrawerLeave"
    >
      <NDrawerContent v-if="selected" :title="selected.name" closable :native-scrollbar="false">
        <div class="personal-drawer-head">
          <div class="qualification-line personal-enable">
            <span>开放状态</span>
            <NSwitch v-model:value="selected.enabled" :disabled="disabled" aria-label="开放状态" />
          </div>
        </div>
        <NAlert v-if="saveError" type="error" :title="saveError" class="mb-16px" />
        <NForm label-placement="top" :disabled="disabled">
          <NGrid cols="1 m:3" responsive="screen" :x-gap="20" class="personal-fields">
            <NGi><NFormItem label="开始日（角色创建后第几天）"><NInputNumber v-model:value="selected.startDay" :input-props="{ 'aria-label': '开始日' }" :min="1" :max="366" :precision="0" /></NFormItem></NGi>
            <NGi><NFormItem label="持续天数"><NInputNumber v-model:value="selected.durationDays" :input-props="{ 'aria-label': '持续天数' }" :min="1" :max="366" :precision="0" /></NFormItem></NGi>
            <NGi><NFormItem label="领奖宽限（天）"><NInputNumber v-model:value="selected.claimDays" :input-props="{ 'aria-label': '领奖宽限天数' }" :min="0" :max="90" :precision="0" /></NFormItem></NGi>
          </NGrid>
          <p class="personal-rule-note">按角色创建日计算；保存后只影响之后生成的个人实例。</p>
          <ActivityMilestones v-model="selected.form.milestones" :mode="selected.mode" :disabled="disabled" :max="31" />
        </NForm>
        <template #footer>
          <div class="ops-drawer-actions">
            <span class="ops-muted">{{ saving ? '正在保存…' : dirty ? '有尚未保存的修改' : '配置已载入' }}</span>
            <NButton :disabled="saving" @click="closeDrawer">取消</NButton>
            <NButton v-if="canWrite" type="primary" :loading="saving" :disabled="disabled || !dirty" @click="save">保存配置</NButton>
          </div>
        </template>
      </NDrawerContent>
    </NDrawer>
  </section>
</template>

<style scoped>
.ops-filters .personal-server.n-select { width: 220px; }
.personal-count { margin-left: auto; font-variant-numeric: tabular-nums; }
.personal-body { min-height: 140px; display: flex; flex-direction: column; justify-content: center; }
.personal-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 128px 112px 132px auto;
  align-items: center;
  gap: 16px;
  width: 100%;
}
.personal-identity { display: flex; align-items: center; gap: 10px; min-width: 0; flex-wrap: wrap; }
.personal-identity strong { font-size: 14px; font-weight: 500; overflow-wrap: anywhere; }
.personal-time { color: rgb(var(--base-text-color) / .65); font-size: 13px; font-variant-numeric: tabular-nums; white-space: nowrap; }
.personal-config { justify-self: end; width: 76px; }
.personal-list :deep(.n-list-item__main) { width: 100%; min-width: 0; }
.personal-drawer-head {
  padding-bottom: 8px;
  margin-bottom: 16px;
  border-bottom: 1px solid rgb(var(--base-text-color) / .09);
}
.personal-enable { margin: 0; gap: 12px; }
.personal-enable span { color: rgb(var(--base-text-color) / .65); font-size: 13px; }
.personal-fields :deep(.n-input-number) { width: 100%; }
.personal-rule-note { margin: -4px 0 20px; font-size: 12px; line-height: 1.6; color: rgb(var(--base-text-color) / .55); }
:global(.n-drawer.personal-opening-drawer) { max-width: 100vw; }
:global(.personal-opening-drawer .n-drawer-body-content-wrapper) { padding: 24px 28px; }
:global(.personal-opening-drawer .n-drawer-header) { padding: 20px 28px 16px; }
:global(.personal-opening-drawer .n-drawer-footer) { padding: 16px 28px; }
:global(.personal-opening-drawer .ops-drawer-actions) { border-top: 0; padding-top: 0; }
@media (max-width: 800px) {
  .ops-filters .personal-server.n-select { width: 100%; }
  .personal-count { margin-left: 0; }
  .personal-body { min-height: 120px; }
  .personal-row { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 14px; }
  .personal-identity { flex: 1 1 100%; }
  .personal-time { white-space: normal; }
  .personal-config { margin-left: auto; }
  :global(.n-drawer.personal-opening-drawer) { width: 100vw !important; }
  :global(.personal-opening-drawer .n-drawer-body-content-wrapper) { padding: 18px 16px; }
  :global(.personal-opening-drawer .n-drawer-header),
  :global(.personal-opening-drawer .n-drawer-footer) { padding-left: 16px; padding-right: 16px; }
}
</style>
