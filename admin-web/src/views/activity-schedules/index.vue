<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { api, errorMessage, json } from '@/service/operations';
import { activityDay, calendarDate, calendarMidnight, expandActivityPattern } from '@/service/activity-schedules';
import type {
  ActivityDefinition,
  ActivityOccurrence,
  ActivityPattern,
  ActivityPatternEntry,
  ActivitySchedule,
  SchedulePreview
} from '@/service/activity-schedules';
import type { ActivityMode } from '@/service/activity-operations';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { useOperations } from '@/composables/useOperations';

const scope = useServerScopeStore();
const { canWrite, loading, error, run } = useOperations();
const definitions = ref<ActivityDefinition[]>([]);
const patterns = ref<ActivityPattern[]>([]);
const modes = ref<ActivityMode[]>([]);
const items = ref<ActivityOccurrence[]>([]);
const version = ref('');
const baseline = ref('');
const saving = ref(false);
const checking = ref(false);
const confirmingPublish = ref(false);
const managed = ref(false);
const calendarWarnings = ref<string[]>([]);
const gameNow = ref(Date.now());
let initializedDates = false;
const saveError = ref('');
const preview = ref<SchedulePreview | null>(null);
const startDate = ref(calendarMidnight(Date.now()) - 3 * activityDay);
const visibleDays = ref(28);
const dayWidth = 48;
const libraryTab = ref('patterns');
const librarySearch = ref('');
const selectedKey = ref<string | null>(null);
const repeats = ref(1);
const fillDate = ref(calendarMidnight(Date.now()));
const patternDialog = ref(false);
const patternName = ref('');
const cycleDays = ref(14);
const editingPattern = ref<ActivityPattern | null>(null);
const patternEntries = ref<ActivityPatternEntry[]>([]);
const patternSaving = ref(false);
const patternBaseline = ref('');
const patternSourceStart = ref<number | null>(null);
const patternDraft = computed(() =>
  JSON.stringify({ name: patternName.value, cycleDays: cycleDays.value, entries: patternEntries.value })
);
const patternDirty = computed(
  () => patternDialog.value && canWrite.value && patternDraft.value !== patternBaseline.value
);
const busy = computed(() => saving.value || checking.value || confirmingPublish.value || patternSaving.value);
let requestGeneration = 0;
let removeScopeGuard: (() => void) | undefined;
const draftBody = computed(() => ({
  version: version.value,
  items: items.value.map(({ draftKey: _draftKey, ...item }) => item)
}));
const dirty = computed(() => version.value !== '' && JSON.stringify(draftBody.value.items) !== baseline.value);
const ready = computed(() => Boolean(scope.serverId && version.value && !busy.value && !loading.value));
const canPublish = computed(() => ready.value && canWrite.value && (dirty.value || !managed.value));
const selected = computed(() => items.value.find(item => keyOf(item) === selectedKey.value));
const usableDefinitions = computed(() => definitions.value.filter(item => item.enabled && item.availability.usable));
const templateOptions = computed(() =>
  definitions.value.map(item => ({
    label: item.name + (!item.enabled || !item.availability.usable ? '（暂不可排期）' : ''),
    value: item.id,
    disabled: !item.enabled || !item.availability.usable
  }))
);
const modeMap = computed(
  () => new Map([...modes.value.map(mode => [mode.mode, mode.label] as const), ['opening-14', '开服14天原版任务']])
);
const filteredPatterns = computed(() =>
  patterns.value.filter(
    pattern => !librarySearch.value || pattern.name.toLowerCase().includes(librarySearch.value.toLowerCase())
  )
);
const filteredDefinitions = computed(() =>
  usableDefinitions.value.filter(
    item => !librarySearch.value || item.name.toLowerCase().includes(librarySearch.value.toLowerCase())
  )
);
const dates = computed(() =>
  Array.from({ length: visibleDays.value }, (_, index) => startDate.value + index * activityDay)
);
const timelineWidth = computed(() => visibleDays.value * dayWidth);
const rows = computed(() => {
  const max = Math.min(31, Math.max(3, ...items.value.map(item => item.lane)) + 1);
  return Array.from({ length: max + 1 }, (_, lane) => {
    const ends: number[] = [];
    const bars = items.value
      .filter(
        item =>
          item.lane === lane &&
          item.claimEndAt > startDate.value &&
          item.startAt < startDate.value + visibleDays.value * activityDay
      )
      .sort((a, b) => a.startAt - b.startAt || keyOf(a).localeCompare(keyOf(b)))
      .map(item => {
        let track = ends.findIndex(end => end <= item.startAt);
        if (track < 0) track = ends.length;
        ends[track] = item.claimEndAt;
        return { item, track };
      });
    return { lane, bars, height: Math.max(1, ends.length) * 58 + 14 };
  });
});
function keyOf(item: ActivityOccurrence): string {
  return item.id || item.draftKey || '';
}
function changed() {
  preview.value = null;
  saveError.value = '';
}
function clone<T>(value: T): T {
  return JSON.parse(JSON.stringify(value));
}
function confirmDiscard(): Promise<boolean> {
  if (!dirty.value && !patternDirty.value) return Promise.resolve(true);
  return new Promise(resolve => {
    if (!window.$dialog) {
      resolve(false);
      return;
    }
    window.$dialog.warning({
      title: '有未保存的区服排期',
      content: '离开或切服会丢弃未保存的排期或方案草稿。已发布活动保持原状态。',
      positiveText: '放弃并离开',
      negativeText: '继续编辑',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false)
    });
  });
}
async function loadCalendar(checkDraft = false) {
  if (checkDraft && !(await confirmDiscard())) return;
  const serverId = scope.serverId;
  requestGeneration += 1;
  const generation = requestGeneration;
  selectedKey.value = null;
  preview.value = null;
  saveError.value = '';
  if (!serverId) {
    items.value = [];
    version.value = '';
    baseline.value = '';
    return;
  }
  const out = await run(() => api<ActivitySchedule>(`/activity-schedules?serverId=${serverId}`));
  if (!out || generation !== requestGeneration || scope.serverId !== serverId) return;
  items.value = out.items;
  managed.value = out.managed;
  calendarWarnings.value = out.warnings;
  gameNow.value = out.gameTimeMs;
  if (!initializedDates) {
    startDate.value = calendarMidnight(gameNow.value) - 3 * activityDay;
    fillDate.value = calendarMidnight(gameNow.value);
    initializedDates = true;
  }
  version.value = out.version;
  baseline.value = JSON.stringify(draftBody.value.items);
}
async function loadLibrary() {
  const result = await run(() =>
    Promise.all([
      api<{ items: ActivityDefinition[] }>('/activity-templates'),
      api<{ items: ActivityPattern[] }>('/activity-patterns'),
      api<{ items: ActivityMode[] }>('/activity-modes')
    ])
  );
  if (!result) return;
  definitions.value = result[0].items;
  patterns.value = result[1].items;
  modes.value = result[2].items;
}
function firstFreeLane(startAt: number, endAt: number): number {
  for (let lane = 0; lane < 32; lane += 1)
    if (!items.value.some(item => item.lane === lane && item.startAt < endAt && item.claimEndAt > startAt)) return lane;
  return 31;
}
function addDefinition(definition: ActivityDefinition, startAt: number, lane?: number) {
  if (!ready.value || !canWrite.value) return;
  const endAt = calendarMidnight(startAt) + definition.defaultDays * activityDay;
  items.value.push({
    id: '',
    templateId: definition.id,
    templateVersion: definition.version,
    name: definition.name,
    mode: definition.mode,
    startAt: calendarMidnight(startAt),
    endAt,
    claimEndAt: endAt + definition.claimDays * activityDay,
    lane: lane ?? firstFreeLane(startAt, endAt),
    enabled: true,
    frozen: false,
    version: '',
    consumerId: '',
    draftKey: `draft-${Date.now()}-${Math.random()}`
  });
  changed();
}
function applyPattern(pattern: ActivityPattern, date: number, laneOffset = 0) {
  if (!ready.value || !canWrite.value) return;
  try {
    const expanded = expandActivityPattern(pattern, definitions.value, { anchor: date, repeats: repeats.value || 1 });
    if (expanded.length + items.value.length > 1000) throw new Error('一次排期最多1000项，请减少循环次数');
    if (expanded.some(item => item.lane + laneOffset > 31))
      throw new Error('拖入后的方案超出32条轨道，请选择更靠上的轨道或修改方案');
    expanded.forEach(item => {
      item.lane += laneOffset;
    });
    items.value.push(...expanded);
    changed();
    window.$message?.success(`已填入 ${expanded.length} 项草稿，检查冲突后统一发布`);
  } catch (e) {
    window.$message?.error(errorMessage(e));
  }
}
function startLibraryDrag(event: DragEvent, kind: string, id: string) {
  if (!event.dataTransfer || !ready.value || !canWrite.value) return;
  event.dataTransfer.setData('application/hyjs-activity', JSON.stringify({ kind, id }));
  event.dataTransfer.effectAllowed = 'copy';
}
function dropLibrary(event: DragEvent, lane: number) {
  event.preventDefault();
  if (!ready.value || !canWrite.value || !event.dataTransfer) return;
  try {
    const data = JSON.parse(event.dataTransfer.getData('application/hyjs-activity')) as { kind: string; id: string };
    const box = (event.currentTarget as HTMLElement).getBoundingClientRect();
    const day = Math.max(0, Math.min(visibleDays.value - 1, Math.floor((event.clientX - box.left) / dayWidth)));
    const date = startDate.value + day * activityDay;
    if (data.kind === 'template') {
      const definition = definitions.value.find(item => item.id === data.id);
      if (definition) addDefinition(definition, date, lane);
    }
    if (data.kind === 'pattern') {
      const pattern = patterns.value.find(item => item.id === data.id);
      if (pattern) applyPattern(pattern, date, lane);
    }
  } catch {
    window.$message?.warning('请从左侧活动或方案库拖入');
  }
}
const pointer = ref<{
  key: string;
  pointerId: number;
  x: number;
  start: number;
  end: number;
  claim: number;
  kind: 'move' | 'start' | 'end';
} | null>(null);
function startPointer(event: PointerEvent, item: ActivityOccurrence, kind: 'move' | 'start' | 'end') {
  if (!ready.value || !canWrite.value || item.frozen || event.button !== 0) return;
  event.preventDefault();
  event.stopPropagation();
  (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
  pointer.value = {
    key: keyOf(item),
    pointerId: event.pointerId,
    x: event.clientX,
    start: item.startAt,
    end: item.endAt,
    claim: item.claimEndAt,
    kind
  };
  selectedKey.value = null;
}
function movePointer(event: PointerEvent) {
  const state = pointer.value;
  if (!state || state.pointerId !== event.pointerId) return;
  const item = items.value.find(candidate => keyOf(candidate) === state.key);
  if (!item) return;
  const shift = Math.round((event.clientX - state.x) / dayWidth) * activityDay;
  if (state.kind === 'move') {
    item.startAt = state.start + shift;
    item.endAt = state.end + shift;
    item.claimEndAt = state.claim + shift;
  }
  if (state.kind === 'start')
    item.startAt = Math.max(state.end - 366 * activityDay, Math.min(state.end - activityDay, state.start + shift));
  if (state.kind === 'end') {
    item.endAt = Math.min(state.start + 366 * activityDay, Math.max(state.start + activityDay, state.end + shift));
    item.claimEndAt = item.endAt + state.claim - state.end;
  }
  changed();
}
function endPointer(event: PointerEvent) {
  if (pointer.value?.pointerId !== event.pointerId) return;
  const target = event.currentTarget as HTMLElement;
  if (target.hasPointerCapture(event.pointerId)) target.releasePointerCapture(event.pointerId);
  pointer.value = null;
}
function keyboardMove(event: KeyboardEvent, item: ActivityOccurrence) {
  if (item.frozen || !canWrite.value || !ready.value || !['ArrowLeft', 'ArrowRight'].includes(event.key)) return;
  event.preventDefault();
  const delta = (event.key === 'ArrowRight' ? 1 : -1) * (event.shiftKey ? 7 : 1) * activityDay;
  item.startAt += delta;
  item.endAt += delta;
  item.claimEndAt += delta;
  changed();
}
function barStyle(item: ActivityOccurrence, track: number) {
  return {
    left: `${((item.startAt - startDate.value) / activityDay) * dayWidth}px`,
    top: `${track * 58 + 8}px`,
    width: `${Math.max(16, ((item.endAt - item.startAt) / activityDay) * dayWidth - 4)}px`
  };
}
function removeSelected() {
  if (!selected.value || !canWrite.value || !ready.value || selected.value.frozen) return;
  items.value = items.value.filter(item => keyOf(item) !== selectedKey.value);
  selectedKey.value = null;
  changed();
}
function copySelected() {
  if (!selected.value || !canWrite.value || !ready.value) return;
  const item = clone(selected.value);
  const definition = definitions.value.find(value => value.id === item.templateId);
  if (!definition?.enabled || !definition.availability.usable) {
    window.$message?.warning('请先补齐并启用对应全局模板');
    return;
  }
  item.name = definition.name;
  item.mode = definition.mode;
  item.templateVersion = definition.version;
  item.id = '';
  item.version = '';
  item.consumerId = '';
  item.frozen = false;
  item.draftKey = `copy-${Date.now()}`;
  const delta = item.endAt - item.startAt;
  item.startAt += delta;
  item.endAt += delta;
  item.claimEndAt += delta;
  items.value.push(item);
  selectedKey.value = keyOf(item);
  changed();
}
async function checkCalendar() {
  if (!ready.value || !scope.serverId) return;
  const serverId = scope.serverId;
  const request = clone(draftBody.value);
  const generation = requestGeneration;
  checking.value = true;
  saveError.value = '';
  preview.value = null;
  try {
    const out = await api<SchedulePreview>(`/activity-schedules/preview?serverId=${serverId}`, json('POST', request));
    if (scope.serverId === serverId && generation === requestGeneration) preview.value = out;
  } catch (e) {
    saveError.value = errorMessage(e);
  } finally {
    checking.value = false;
  }
}
async function publish() {
  if (!canPublish.value || !scope.serverId) return;
  const targetServer = scope.serverId;
  if (!managed.value) {
    confirmingPublish.value = true;
    const accepted = await new Promise<boolean>(resolve => {
      if (!window.$dialog) {
        resolve(false);
        return;
      }
      window.$dialog.warning({
        title: '首次启用本服活动排期',
        content: items.value.length
          ? '发布后，本轮支持的旧全局活动入口将交由本服日历控制；未安排的活动停止开放。已发奖励和历史领取记录保留。'
          : '当前日历为空。启用后，本轮支持的旧全局活动入口将停止开放；之后需要在本服日历安排活动才会开放。已发奖励和历史领取记录保留。',
        positiveText: '确认启用并发布',
        negativeText: '继续编排',
        onPositiveClick: () => resolve(true),
        onNegativeClick: () => resolve(false),
        onClose: () => resolve(false),
        onMaskClick: () => resolve(false)
      });
    });
    confirmingPublish.value = false;
    if (!accepted || !canPublish.value || scope.serverId !== targetServer) return;
  }
  // Lock the editor and submit the exact calendar that passed preview, including its realm identity.
  const serverId = scope.serverId;
  const request = clone(draftBody.value);
  saving.value = true;
  saveError.value = '';
  preview.value = null;
  try {
    preview.value = await api<SchedulePreview>(
      `/activity-schedules/preview?serverId=${serverId}`,
      json('POST', request)
    );
    if (!preview.value.valid || scope.serverId !== serverId) return;
    const out = await api<ActivitySchedule>(`/activity-schedules?serverId=${serverId}`, json('PUT', request));
    if (scope.serverId !== serverId) return;
    items.value = out.items;
    managed.value = out.managed;
    calendarWarnings.value = out.warnings;
    gameNow.value = out.gameTimeMs;
    version.value = out.version;
    baseline.value = JSON.stringify(draftBody.value.items);
    preview.value = null;
    window.$message?.success('本服活动排期已原子发布，原玩法入口同步生效');
  } catch (e) {
    saveError.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
function openPattern(pattern?: ActivityPattern, fromCalendar = false) {
  editingPattern.value = pattern || null;
  patternName.value = pattern?.name || '';
  cycleDays.value = pattern?.cycleDays || 14;
  patternSourceStart.value = null;
  if (pattern) patternEntries.value = clone(pattern.entries);
  else if (fromCalendar) {
    const current = items.value.filter(
      item =>
        item.enabled &&
        item.startAt >= startDate.value &&
        item.startAt < startDate.value + visibleDays.value * activityDay
    );
    const anchor = current.length ? calendarMidnight(Math.min(...current.map(item => item.startAt))) : startDate.value;
    patternSourceStart.value = anchor;
    patternEntries.value = current.map(item => ({
      templateId: item.templateId,
      startDay: Math.floor((item.startAt - anchor) / activityDay),
      durationDays: Math.ceil((item.endAt - item.startAt) / activityDay),
      claimDays: Math.ceil((item.claimEndAt - item.endAt) / activityDay),
      lane: item.lane
    }));
    // Include claim windows in the initial cycle so repeating a copied singleton cannot overlap by default.
    cycleDays.value = Math.min(
      366,
      Math.max(1, ...current.map(item => Math.ceil((item.claimEndAt - anchor) / activityDay)))
    );
  } else patternEntries.value = [];
  patternBaseline.value = patternDraft.value;
  patternDialog.value = true;
}
async function closePattern() {
  if (patternSaving.value) return;
  if (patternDirty.value) {
    const accepted = await new Promise<boolean>(resolve => {
      if (!window.$dialog) {
        resolve(false);
        return;
      }
      window.$dialog.warning({
        title: '放弃未保存的排期方案？',
        content: '方案编辑尚未保存。当前区服日历不会受到影响。',
        positiveText: '放弃修改',
        negativeText: '继续编辑',
        onPositiveClick: () => resolve(true),
        onNegativeClick: () => resolve(false),
        onClose: () => resolve(false),
        onMaskClick: () => resolve(false)
      });
    });
    if (!accepted) return;
  }
  patternDialog.value = false;
}
function addPatternEntry() {
  if (usableDefinitions.value[0])
    patternEntries.value.push({
      templateId: usableDefinitions.value[0].id,
      startDay: 0,
      durationDays: 1,
      claimDays: 0,
      lane: 0
    });
}
async function savePattern() {
  if (!canWrite.value || patternSaving.value) return;
  if (!patternName.value.trim() || !patternEntries.value.length) {
    window.$message?.warning('请填写方案名称并添加至少一个活动');
    return;
  }
  patternSaving.value = true;
  try {
    await api(
      editingPattern.value ? `/activity-patterns/${editingPattern.value.id}` : '/activity-patterns',
      json(editingPattern.value ? 'PUT' : 'POST', {
        name: patternName.value.trim(),
        cycleDays: cycleDays.value,
        entries: patternEntries.value,
        version: editingPattern.value?.version || ''
      })
    );
    patternDialog.value = false;
    await loadLibrary();
    window.$message?.success('方案已保存，可反复拖入任何区服日期');
  } catch (e) {
    window.$message?.error(errorMessage(e));
  } finally {
    patternSaving.value = false;
  }
}
function archivePattern(pattern: ActivityPattern) {
  window.$dialog?.warning({
    title: '归档这个排期方案？',
    content: '已复制到各服的活动和历史记录保留。',
    positiveText: '归档',
    negativeText: '取消',
    onPositiveClick: async () => {
      try {
        await api(`/activity-patterns/${pattern.id}`, json('DELETE', { version: pattern.version }));
        await loadLibrary();
      } catch (e) {
        window.$message?.error(errorMessage(e));
      }
    }
  });
}
watch(
  () => scope.serverId,
  () => {
    patternDialog.value = false;
    items.value = [];
    managed.value = false;
    calendarWarnings.value = [];
    version.value = '';
    baseline.value = '';
    loadCalendar();
  }
);
function beforeUnload(event: BeforeUnloadEvent) {
  if (dirty.value || patternDirty.value || busy.value) {
    event.preventDefault();
    event.returnValue = '';
  }
}
onBeforeRouteLeave(() => (busy.value ? false : confirmDiscard()));
onMounted(async () => {
  window.addEventListener('beforeunload', beforeUnload);
  removeScopeGuard = scope.addSwitchGuard(() => (busy.value ? false : confirmDiscard()));
  await scope.loadServers();
  await loadLibrary();
  await loadCalendar();
});
onUnmounted(() => {
  requestGeneration += 1;
  window.removeEventListener('beforeunload', beforeUnload);
  removeScopeGuard?.();
});
</script>

<template>
  <div class="ops-page schedule-page">
    <div class="ops-toolbar mb-20px">
      <div>
        <h1>
          活动排期
          <NTag v-if="scope.currentServer" :bordered="false" type="info">{{ scope.currentServer.name }}</NTag>
        </h1>
        <p class="ops-muted">全局活动模板 → 本服时间轴 · 拖动移动日期，两端拉伸持续时间，按天吸附。</p>
      </div>
      <NSpace>
        <NButton :disabled="busy" :loading="loading" @click="loadCalendar(true)">刷新本服</NButton>
        <NButton v-if="canWrite" :disabled="!ready" @click="openPattern(undefined, true)">可见活动存为方案</NButton>
        <NButton v-if="canWrite" type="primary" :disabled="!canPublish" :loading="saving" @click="publish">
          发布本服排期
        </NButton>
      </NSpace>
    </div>
    <NAlert
      v-if="!scope.serverId"
      type="info"
      title="请先在顶部选择区服，再安排活动。全局模板与方案库可跨服复用。"
      class="mb-20px"
    />
    <NAlert v-if="error || saveError" type="error" :title="saveError || error" class="mb-16px" />
    <NAlert
      v-if="scope.serverId && version"
      :type="managed ? 'info' : 'warning'"
      :title="managed ? '本服已使用独立活动排期' : '本服尚未启用排期，当前仍沿用旧活动入口'"
      class="mb-16px"
    >
      {{
        managed
          ? '仅按本服已发布日历开放活动；空档停止开放。草稿需要发布后才生效。'
          : '首次发布将切换为本服日历控制。可以发布空日历停用旧入口，之后再逐项安排活动。'
      }}
    </NAlert>
    <div class="schedule-layout">
      <NCard :bordered="false" class="schedule-library">
        <NTabs v-model:value="libraryTab" type="segment">
          <NTab name="patterns">排期方案</NTab>
          <NTab name="templates">单个活动</NTab>
        </NTabs>
        <NInput v-model:value="librarySearch" class="mb-16px mt-16px" clearable placeholder="搜索活动或方案" />
        <div class="library-fill">
          <label>
            填充起日
            <NDatePicker v-model:value="fillDate" type="date" :clearable="false" />
          </label>
          <label>
            循环次数
            <NInputNumber v-model:value="repeats" :min="1" :max="52" />
          </label>
          <NButton
            v-if="scope.currentServer?.openAt"
            size="small"
            @click="fillDate = calendarMidnight(scope.currentServer.openAt)"
          >
            从本服开服日开始
          </NButton>
        </div>
        <p class="ops-muted library-help">拖到某一天可直接填充；也可点击“填入草稿”。循环间隔由方案周期决定。</p>
        <template v-if="libraryTab === 'patterns'">
          <NButton v-if="canWrite" block secondary class="mb-12px" :disabled="busy" @click="openPattern()">
            ＋ 新建排期方案
          </NButton>
          <article
            v-for="pattern in filteredPatterns"
            :key="pattern.id"
            class="library-card"
            :draggable="ready && canWrite"
            @dragstart="startLibraryDrag($event, 'pattern', pattern.id)"
          >
            <strong>{{ pattern.name }}</strong>
            <span>{{ pattern.entries.length }} 个活动 · 每 {{ pattern.cycleDays }} 天循环</span>
            <NSpace :size="6">
              <NButton size="tiny" :disabled="!ready || !canWrite" @click="applyPattern(pattern, fillDate)">
                填入草稿
              </NButton>
              <NButton size="tiny" quaternary @click="openPattern(pattern)">{{ canWrite ? '编辑' : '查看' }}</NButton>
              <NButton v-if="canWrite" size="tiny" quaternary @click="archivePattern(pattern)">归档</NButton>
            </NSpace>
          </article>
          <NEmpty v-if="!filteredPatterns.length" description="保存一份方案后，可拖入日期反复使用" />
        </template>
        <template v-else>
          <article
            v-for="definition in filteredDefinitions"
            :key="definition.id"
            class="library-card"
            :draggable="ready && canWrite"
            @dragstart="startLibraryDrag($event, 'template', definition.id)"
          >
            <strong>{{ definition.name }}</strong>
            <span>{{ modeMap.get(definition.mode) || definition.mode }} · {{ definition.defaultDays }}天</span>
            <NButton size="tiny" :disabled="!ready || !canWrite" @click="addDefinition(definition, fillDate)">
              填入草稿
            </NButton>
          </article>
          <NEmpty v-if="!filteredDefinitions.length" description="先在全局活动设置补齐并允许排期" />
        </template>
      </NCard>
      <div class="schedule-main">
        <NCard :bordered="false">
          <div class="calendar-toolbar">
            <NSpace>
              <NButton size="small" @click="startDate -= visibleDays * activityDay">←</NButton>
              <NDatePicker v-model:value="startDate" type="date" :clearable="false" />
              <NButton size="small" @click="startDate += visibleDays * activityDay">→</NButton>
              <NButton size="small" @click="startDate = calendarMidnight(gameNow) - 3 * activityDay">游戏今天</NButton>
            </NSpace>
            <NSpace>
              <NSelect
                v-model:value="visibleDays"
                :options="[
                  { label: '两周', value: 14 },
                  { label: '四周', value: 28 },
                  { label: '八周', value: 56 }
                ]"
                class="w-100px"
              />
              <NButton :disabled="!ready" :loading="checking" @click="checkCalendar">检查冲突</NButton>
            </NSpace>
          </div>
          <div class="calendar-scroll" :class="{ 'calendar-disabled': !scope.serverId }">
            <div class="calendar-canvas" :style="{ width: `${timelineWidth + 72}px` }">
              <div class="calendar-head">
                <div class="lane-label">并行轨道</div>
                <div
                  v-for="date in dates"
                  :key="date"
                  class="date-cell"
                  :class="{ today: date === calendarMidnight(gameNow) }"
                  :style="{ width: `${dayWidth}px` }"
                >
                  {{ calendarDate(date) }}
                  <small>
                    {{ ['日', '一', '二', '三', '四', '五', '六'][new Date(date + 8 * 3600000).getUTCDay()] }}
                  </small>
                </div>
              </div>
              <div v-for="row in rows" :key="row.lane" class="calendar-row" :style="{ height: `${row.height}px` }">
                <div class="lane-label">轨道 {{ row.lane + 1 }}</div>
                <div
                  class="calendar-lane"
                  :style="{ width: `${timelineWidth}px`, backgroundSize: `${dayWidth}px 100%` }"
                  @dragover.prevent
                  @drop="dropLibrary($event, row.lane)"
                >
                  <div
                    v-for="bar in row.bars"
                    :key="keyOf(bar.item)"
                    class="activity-bar"
                    :class="{
                      frozen: bar.item.frozen,
                      off: !bar.item.enabled,
                      draft: !bar.item.id,
                      opening: bar.item.mode === 'opening-14'
                    }"
                    :style="barStyle(bar.item, bar.track)"
                    role="button"
                    tabindex="0"
                    :aria-label="`${bar.item.name}，${calendarDate(bar.item.startAt)}至${calendarDate(bar.item.endAt)}，回车编辑，左右键移动一天`"
                    @pointerdown="startPointer($event, bar.item, 'move')"
                    @pointermove="movePointer"
                    @pointerup="endPointer"
                    @pointercancel="endPointer"
                    @keydown="keyboardMove($event, bar.item)"
                    @keydown.enter="selectedKey = keyOf(bar.item)"
                    @dblclick="selectedKey = keyOf(bar.item)"
                  >
                    <span
                      v-if="!bar.item.frozen && canWrite"
                      class="resize-handle left"
                      @pointerdown="startPointer($event, bar.item, 'start')"
                      @pointermove.stop="movePointer"
                      @pointerup.stop="endPointer"
                      @pointercancel.stop="endPointer"
                    />
                    <span
                      v-if="bar.item.claimEndAt > bar.item.endAt"
                      class="claim-tail"
                      :style="{ width: `${((bar.item.claimEndAt - bar.item.endAt) / activityDay) * dayWidth}px` }"
                      :title="`领奖期至 ${calendarDate(bar.item.claimEndAt)}（不含）`"
                    >
                      领奖
                    </span>
                    <div class="bar-title">{{ bar.item.name }}</div>
                    <small>
                      {{ calendarDate(bar.item.startAt) }} — {{ calendarDate(bar.item.endAt)
                      }}{{ bar.item.frozen ? ' · 已冻结' : '' }}
                    </small>
                    <button
                      class="bar-edit"
                      title="查看或精确修改日期"
                      @pointerdown.stop
                      @click.stop="selectedKey = keyOf(bar.item)"
                    >
                      ⋯
                    </button>
                    <span
                      v-if="!bar.item.frozen && canWrite"
                      class="resize-handle right"
                      @pointerdown="startPointer($event, bar.item, 'end')"
                      @pointermove.stop="movePointer"
                      @pointerup.stop="endPointer"
                      @pointercancel.stop="endPointer"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div class="calendar-legend">
            <span class="legend-dot" />
            已发布
            <span class="legend-dot draft" />
            未保存草稿
            <span class="legend-dot frozen" />
            已冻结 · 保留原时间
            <span class="legend-dot claim" />
            额外领奖期
            <span class="ops-muted">游戏日期 {{ calendarDate(gameNow) }} · 北京时间，结束日不包含在开放区间内。</span>
          </div>
        </NCard>
        <NAlert
          v-if="preview"
          :type="preview.valid ? 'success' : 'warning'"
          :title="preview.valid ? '未发现阻断冲突，可以发布本服排期' : '请先调整以下排期冲突'"
          class="mt-16px"
        >
          <ul v-if="preview.conflicts.length">
            <li v-for="(conflict, index) in preview.conflicts" :key="index">
              {{ conflict.index >= 0 ? items[conflict.index]?.name + '：' : '' }}{{ conflict.message }}
            </li>
          </ul>
        </NAlert>
        <NAlert v-for="warning in calendarWarnings" :key="warning" type="info" :title="warning" class="mt-16px" />
        <NAlert type="info" title="规则与历史保护" class="mt-16px">
          不同玩法可并行；转盘及原无实例福利同模式不能重叠，领奖宽限也计入检查。已冻结活动保留时间和奖励，可停用或复制新一期。开服14天任务保留每角色第1天与原领取记录。
        </NAlert>
      </div>
    </div>
    <div v-if="dirty || (version && !managed)" class="schedule-draft-footer">
      <span>{{ !managed ? '本服排期尚未启用' : '本服排期有未发布修改' }} · {{ items.length }} 项活动</span>
      <NButton type="primary" :disabled="!canPublish" :loading="saving" @click="publish">检查并发布</NButton>
    </div>
    <NDrawer :show="Boolean(selected)" :width="440" @update:show="selectedKey = $event ? selectedKey : null">
      <NDrawerContent v-if="selected" :title="selected.name" closable>
        <NForm label-placement="top" :disabled="!canWrite || busy">
          <NTag :bordered="false">{{ modeMap.get(selected.mode) || selected.mode }}</NTag>
          <NAlert
            v-if="selected.frozen"
            type="info"
            title="已冻结的活动不能移动或改时，原奖励和领取状态继续保留。"
            class="mb-16px mt-16px"
          />
          <NFormItem label="开始日期">
            <NDatePicker
              v-model:value="selected.startAt"
              type="datetime"
              :clearable="false"
              :disabled="selected.frozen || !canWrite || busy"
              class="w-full"
              @update:value="changed"
            />
          </NFormItem>
          <NFormItem label="结束日期（不含）">
            <NDatePicker
              v-model:value="selected.endAt"
              type="datetime"
              :clearable="false"
              :disabled="selected.frozen || !canWrite || busy"
              class="w-full"
              @update:value="changed"
            />
          </NFormItem>
          <NFormItem label="领奖截止">
            <NDatePicker
              v-model:value="selected.claimEndAt"
              type="datetime"
              :clearable="false"
              :disabled="selected.frozen || !canWrite || busy"
              class="w-full"
              @update:value="changed"
            />
          </NFormItem>
          <NFormItem label="并行轨道">
            <NInputNumber
              :value="selected.lane + 1"
              :min="1"
              :max="32"
              @update:value="
                selected.lane = ($event || 1) - 1;
                changed();
              "
            />
          </NFormItem>
          <NFormItem label="发布开放"><NSwitch v-model:value="selected.enabled" @update:value="changed" /></NFormItem>
          <p class="ops-muted">{{ selected.id ? '原玩法实例：' + selected.consumerId : '未保存的新排期' }}</p>
        </NForm>
        <template #footer>
          <NSpace>
            <NButton v-if="canWrite" @click="copySelected">复制到下一期</NButton>
            <NButton v-if="canWrite && !selected.frozen" type="error" secondary @click="removeSelected">
              从草稿移除
            </NButton>
            <NButton @click="selectedKey = null">完成</NButton>
          </NSpace>
        </template>
      </NDrawerContent>
    </NDrawer>
    <NModal
      :show="patternDialog"
      preset="card"
      :title="editingPattern ? '编辑排期方案' : '保存排期方案'"
      class="pattern-modal"
      :mask-closable="!patternSaving"
      :close-on-esc="!patternSaving"
      @update:show="!$event && closePattern()"
    >
      <NAlert
        type="info"
        title="方案以第1天为起点保存相对日序。拖入日期时展开；重复填充按周期相隔，不影响之前发布的活动。"
        class="mb-20px"
      />
      <NAlert
        v-if="patternSourceStart !== null"
        type="info"
        :title="`已取可视区间内启用的活动；${calendarDate(patternSourceStart)} 对应方案第1天，默认周期覆盖最后领奖日。`"
        class="mb-16px"
      />
      <NForm label-placement="top" :disabled="!canWrite || patternSaving">
        <div class="pattern-top">
          <NFormItem label="方案名称"><NInput v-model:value="patternName" :maxlength="80" /></NFormItem>
          <NFormItem label="循环周期（天）"><NInputNumber v-model:value="cycleDays" :min="1" :max="366" /></NFormItem>
        </div>
        <article v-for="(entry, index) in patternEntries" :key="index" class="pattern-entry">
          <NFormItem label="活动">
            <NSelect v-model:value="entry.templateId" :options="templateOptions" filterable />
          </NFormItem>
          <NFormItem label="第几天开始">
            <NInputNumber
              :value="entry.startDay + 1"
              :min="1"
              :max="366"
              @update:value="entry.startDay = ($event || 1) - 1"
            />
          </NFormItem>
          <NFormItem label="持续天数">
            <NInputNumber v-model:value="entry.durationDays" :min="1" :max="366" />
          </NFormItem>
          <NFormItem label="领奖宽限"><NInputNumber v-model:value="entry.claimDays" :min="0" :max="90" /></NFormItem>
          <NFormItem label="并行轨道">
            <NInputNumber :value="entry.lane + 1" :min="1" :max="32" @update:value="entry.lane = ($event || 1) - 1" />
          </NFormItem>
          <NButton v-if="canWrite" quaternary type="error" @click="patternEntries.splice(index, 1)">移除</NButton>
        </article>
        <NButton v-if="canWrite" :disabled="!usableDefinitions.length" @click="addPatternEntry">＋ 添加活动</NButton>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton :disabled="patternSaving" @click="closePattern">取消</NButton>
          <NButton v-if="canWrite" type="primary" :loading="patternSaving" @click="savePattern">保存方案</NButton>
        </NSpace>
      </template>
    </NModal>
  </div>
</template>

<style scoped>
h1 {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 0 0 8px;
  font-size: 22px;
  font-weight: 600;
}
.schedule-layout {
  display: grid;
  grid-template-columns: 265px minmax(0, 1fr);
  gap: 20px;
  align-items: start;
}
.schedule-library :deep(.n-card__content) {
  padding: 18px 16px;
}
.schedule-main {
  min-width: 0;
}
.library-fill {
  display: grid;
  grid-template-columns: 1fr 90px;
  gap: 10px;
}
.library-fill label {
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-size: 12px;
  color: rgb(var(--base-text-color) / 0.65);
}
.library-fill > .n-button {
  grid-column: 1 / -1;
  justify-self: start;
}
.library-help {
  font-size: 12px;
  line-height: 1.8;
  margin: 14px 0;
}
.library-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  border: 1px solid rgb(var(--base-text-color) / 0.1);
  border-radius: 9px;
  padding: 14px 12px;
  margin-top: 12px;
  cursor: grab;
}
.library-card[draggable='false'] {
  cursor: default;
}
.library-card:hover {
  border-color: rgb(var(--primary-color) / 0.45);
}
.library-card strong {
  font-size: 13px;
  font-weight: 500;
  line-height: 1.5;
}
.library-card > span {
  font-size: 12px;
  color: rgb(var(--base-text-color) / 0.5);
}
.calendar-toolbar {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 20px;
}
.calendar-toolbar :deep(.n-date-picker) {
  width: 150px;
}
.calendar-scroll {
  overflow: auto;
  max-height: 650px;
  border: 1px solid rgb(var(--base-text-color) / 0.09);
  border-radius: 8px;
}
.calendar-disabled {
  opacity: 0.5;
  pointer-events: none;
}
.calendar-head {
  display: flex;
  height: 58px;
  position: sticky;
  top: 0;
  z-index: 5;
  background: rgb(var(--container-bg-color));
  border-bottom: 1px solid rgb(var(--base-text-color) / 0.09);
}
.lane-label {
  width: 72px;
  flex: 0 0 72px;
  position: sticky;
  left: 0;
  z-index: 4;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgb(var(--container-bg-color));
  border-right: 1px solid rgb(var(--base-text-color) / 0.09);
  font-size: 12px;
  color: rgb(var(--base-text-color) / 0.55);
}
.date-cell {
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  gap: 5px;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  border-right: 1px solid rgb(var(--base-text-color) / 0.04);
}
.date-cell small {
  color: rgb(var(--base-text-color) / 0.4);
}
.date-cell.today {
  background: rgb(var(--primary-color) / 0.08);
  color: rgb(var(--primary-color));
}
.calendar-row {
  display: flex;
  border-bottom: 1px solid rgb(var(--base-text-color) / 0.07);
}
.calendar-lane {
  flex: 0 0 auto;
  position: relative;
  overflow: hidden;
  background-image: linear-gradient(
    to right,
    transparent calc(100% - 1px),
    rgb(var(--base-text-color) / 0.055) calc(100% - 1px)
  );
}
.activity-bar {
  position: absolute;
  height: 46px;
  padding: 6px 23px 6px 12px;
  background: rgb(var(--primary-color) / 0.12);
  color: rgb(var(--primary-color));
  border: 1px solid rgb(var(--primary-color) / 0.3);
  border-radius: 6px;
  cursor: grab;
  touch-action: none;
  user-select: none;
  min-width: 20px;
}
.activity-bar:focus-visible {
  outline: 2px solid rgb(var(--primary-color));
  outline-offset: 2px;
}
.activity-bar.draft {
  border-style: dashed;
}
.activity-bar.frozen {
  color: #64748b;
  background: rgb(100 116 139 / 0.1);
  border-color: rgb(100 116 139 / 0.3);
  cursor: default;
}
.activity-bar.opening {
  color: #9a6700;
  background: rgb(217 167 0 / 0.1);
  border-color: rgb(217 167 0 / 0.35);
}
.activity-bar.off {
  opacity: 0.5;
}
.bar-title {
  font-size: 12px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  line-height: 17px;
}
.claim-tail {
  position: absolute;
  left: calc(100% + 4px);
  top: 0;
  bottom: 0;
  display: flex;
  align-items: center;
  padding-left: 5px;
  font-size: 10px;
  border: 1px dashed currentColor;
  border-radius: 0 5px 5px 0;
  opacity: 0.45;
  pointer-events: none;
  white-space: nowrap;
  overflow: hidden;
}
.legend-dot.claim {
  border-style: dashed;
  opacity: 0.5;
}
.activity-bar small {
  font-size: 10px;
  white-space: nowrap;
  display: block;
  overflow: hidden;
}
.resize-handle {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 7px;
  cursor: ew-resize;
}
.resize-handle.left {
  left: 0;
}
.resize-handle.right {
  right: 0;
}
.bar-edit {
  position: absolute;
  right: 6px;
  top: 9px;
  padding: 0;
  width: 17px;
  background: transparent;
  border: 0;
  color: inherit;
  cursor: pointer;
  font-size: 18px;
}
.calendar-legend {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  font-size: 11px;
  color: rgb(var(--base-text-color) / 0.6);
  margin-top: 16px;
}
.legend-dot {
  width: 9px;
  height: 9px;
  border: 1px solid rgb(var(--primary-color) / 0.5);
  background: rgb(var(--primary-color) / 0.12);
  border-radius: 2px;
}
.legend-dot.draft {
  border-style: dashed;
}
.legend-dot.frozen {
  border-color: #94a3b8;
  background: #e2e8f0;
}
.schedule-draft-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 16px 20px;
  margin-top: 16px;
  position: sticky;
  bottom: 0;
  background: rgb(var(--container-bg-color));
  border: 1px solid rgb(var(--primary-color) / 0.15);
  border-radius: 8px;
  z-index: 8;
}
.pattern-modal {
  width: min(1000px, calc(100vw - 40px));
  max-height: calc(100vh - 64px);
  overflow: auto;
}
.pattern-top {
  display: grid;
  grid-template-columns: 1fr 180px;
  gap: 20px;
}
.pattern-entry {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) repeat(4, 110px) 55px;
  gap: 12px;
  align-items: center;
  padding-top: 16px;
  border-top: 1px solid rgb(var(--base-text-color) / 0.08);
}
@media (max-width: 1150px) {
  .schedule-layout {
    grid-template-columns: 230px minmax(0, 1fr);
  }
  .schedule-main :deep(.n-card__content) {
    padding: 16px;
  }
}
@media (max-width: 760px) {
  .schedule-layout {
    display: block;
  }
  .schedule-library {
    margin-bottom: 18px;
    max-height: 330px;
    overflow: auto;
  }
  .pattern-top,
  .pattern-entry {
    grid-template-columns: 1fr 1fr;
  }
  .pattern-entry > :first-child {
    grid-column: 1 / -1;
  }
  .schedule-draft-footer {
    gap: 12px;
    flex-wrap: wrap;
  }
}
</style>
