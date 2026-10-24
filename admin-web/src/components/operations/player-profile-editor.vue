<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { api, ApiError, errorMessage, json } from '@/service/operations';
import { balanceLabels, type GamePlayerDetail, type PlayerEditOptions } from '@/service/player-operations';
import { useServerScopeStore } from '@/store/modules/server-scope';
import { useOperations } from '@/composables/useOperations';
import { useDraftGuard } from '@/composables/useDraftGuard';
const props = defineProps<{ player: GamePlayerDetail | null; show: boolean }>();
const emit = defineEmits<{
  'update:show': [boolean];
  saved: [GamePlayerDetail];
}>();
const scope = useServerScopeStore();
const { canWrite } = useOperations();
const baseline = ref<GamePlayerDetail | null>(null);
const clone = <T,>(v: T): T => JSON.parse(JSON.stringify(v));
const draft = ref<GamePlayerDetail | null>(null);
const options = ref<PlayerEditOptions | null>(null);
const reason = ref('');
const busy = ref(false);
const loading = ref(false);
const error = ref('');
const conflict = ref(false);
const latest = ref<GamePlayerDetail | null>(null);
let sequence = 0;
const guard = useDraftGuard({
  active: () => props.show,
  value: () => ({ draft: draft.value, reason: reason.value }),
  busy: () => busy.value,
  writable: () => canWrite.value,
  close: () => emit('update:show', false),
  label: '玩家资料'
});
const missions = computed(() => options.value?.chapters.find(c => c.value === draft.value?.chapter)?.missions || []);
watch(
  () => [props.show, props.player?.id, scope.serverId],
  async () => {
    if (!props.show || !props.player) {
      sequence++;
      return;
    }
    const seq = ++sequence;
    const id = props.player.id;
    const server = scope.serverId;
    busy.value = false;
    draft.value = clone(props.player);
    baseline.value = clone(props.player);
    reason.value = '';
    error.value = '';
    conflict.value = false;
    latest.value = null;
    options.value = null;
    guard.capture();
    loading.value = true;
    try {
      const data = await api<PlayerEditOptions>(scope.serverQuery(`/game-players/${props.player.id}/edit-options`));
      if (seq === sequence && server === scope.serverId && id === props.player?.id) options.value = data;
    } catch (e) {
      if (seq === sequence && server === scope.serverId && id === props.player?.id) error.value = errorMessage(e);
    } finally {
      if (seq === sequence && server === scope.serverId && id === props.player?.id) loading.value = false;
    }
  }
);
async function refresh() {
  const d = draft.value,
    original = baseline.value;
  if (!d || !original || loading.value || busy.value) return;
  const seq = sequence,
    id = d.id,
    server = scope.serverId;
  const path = scope.serverQuery(`/game-players/${id}`);
  loading.value = true;
  try {
    const data = await api<GamePlayerDetail>(path);
    if (seq !== sequence || server !== scope.serverId || id !== props.player?.id) return;
    // Rebase only user-edited fields onto the latest record; untouched resources follow the latest data.
    const merged = clone(data);
    for (const key of ['name', 'level', 'exp', 'vip', 'chapter', 'mission', 'scene', 'createdAt'] as const)
      if (d[key] !== original[key]) Object.assign(merged, { [key]: d[key] });
    for (const key of Object.keys(balanceLabels))
      if (d.balances[key] !== original.balances[key]) merged.balances[key] = d.balances[key]!;
    draft.value = merged;
    baseline.value = clone(data);
    latest.value = data;
    conflict.value = false;
    error.value = '';
  } catch (e) {
    if (seq === sequence) error.value = errorMessage(e);
  } finally {
    if (seq === sequence) loading.value = false;
  }
}
async function save() {
  const d = draft.value;
  const o = options.value;
  if (!d || !o || !canWrite.value || busy.value || conflict.value) return;
  const numbers = [
    [d.level, 1, o.levelMax],
    [d.exp, 0, Number.MAX_SAFE_INTEGER],
    [d.vip, 0, o.vipMax],
    ...Object.keys(balanceLabels).map(k => [d.balances[k], 0, o.balanceLimits[k]])
  ];
  if (
    !d.name.trim() ||
    d.name
      .trim()
      .split('')
      .reduce((sum, c) => sum + (c.charCodeAt(0) > 256 ? 2 : 1), 0) > 12 ||
    !reason.value.trim() ||
    !d.createdAt ||
    numbers.some(([v, min, max]) => !Number.isSafeInteger(v) || v! < min! || v! > max!)
  ) {
    error.value = '请填写名称、操作原因及范围内的整数';
    return;
  }
  if (
    !o.chapters.some(c => c.value === d.chapter && c.missions.some(m => m.value === d.mission)) ||
    !o.scenes.some(v => v.value === d.scene)
  ) {
    error.value = '请选择有效的章节、关卡与场景';
    return;
  }
  const seq = sequence,
    id = d.id,
    server = scope.serverId;
  busy.value = true;
  error.value = '';
  try {
    const result = await api<GamePlayerDetail>(
      scope.serverQuery(`/game-players/${d.id}/profile`),
      json('PUT', {
        version: d.version,
        name: d.name.trim(),
        level: d.level,
        exp: d.exp,
        vip: d.vip,
        chapter: d.chapter,
        mission: d.mission,
        scene: d.scene,
        createdAt: d.createdAt,
        balances: Object.fromEntries(Object.keys(balanceLabels).map(k => [k, d.balances[k]])),
        reason: reason.value.trim()
      })
    );
    if (seq !== sequence || server !== scope.serverId || id !== props.player?.id) return;
    emit('saved', result);
    emit('update:show', false);
    window.$message?.success('玩家资料已保存');
  } catch (e) {
    if (seq !== sequence || server !== scope.serverId) return;
    conflict.value = e instanceof ApiError && e.status === 409;
    error.value = conflict.value ? '角色资料已更新，请读取最新数据并核对后保存' : errorMessage(e);
  } finally {
    if (seq === sequence) busy.value = false;
  }
}
</script>
<template>
  <NModal
    :show="show"
    preset="card"
    title="编辑玩家资料"
    class="ops-modal profile-modal"
    :style="{
      width: 'min(840px, calc(100vw - 24px))',
      maxHeight: 'calc(100dvh - 32px)'
    }"
    :content-style="{ overflowY: 'auto' }"
    :mask-closable="!busy"
    :closable="!busy"
    @update:show="!$event && guard.requestClose()"
  >
    <NSpin :show="loading"
      ><template v-if="draft">
        <p class="ops-muted">
          {{ scope.currentServer?.name }} · ID {{ draft.id }} · 账号
          {{ draft.username }}
        </p>
        <NAlert v-if="error" type="error" :title="error" class="mb-16px" />
        <NButton v-if="conflict" @click="refresh">读取最新数据并保留草稿</NButton>
        <NAlert v-if="latest" type="info" class="mb-16px" title="已读取最新版本，请对照当前资料后保存"
          >当前：{{ latest.name }} · {{ latest.level }} 级 / 经验 {{ latest.exp }} · VIP{{ latest.vip }} · 主线
          {{ latest.chapter }} / {{ latest.mission }} ·
          {{ options?.scenes.find(s => s.value === latest?.scene)?.label || latest.scene }}
          · {{ new Date(latest.createdAt || 0).toLocaleString() }}
          <div v-for="(label, key) in balanceLabels" :key="key">{{ label }}：{{ latest.balances[key] }}</div></NAlert
        >
        <NForm label-placement="top" :disabled="busy || !canWrite"
          ><div class="profile-grid">
            <NFormItem label="角色名称" required
              ><NInput
                v-model:value="draft.name"
                :maxlength="12"
                placeholder="最多12个英文字符或6个汉字"
                :input-props="{ 'aria-label': '角色名称' }"
            /></NFormItem>
            <NFormItem label="角色等级" required
              ><NInputNumber
                v-model:value="draft.level"
                :min="1"
                :max="options?.levelMax"
                :precision="0"
                :input-props="{ 'aria-label': '角色等级' }"
            /></NFormItem>
            <NFormItem label="本级经验"
              ><NInputNumber
                v-model:value="draft.exp"
                :min="0"
                :max="Number.MAX_SAFE_INTEGER"
                :precision="0"
                :input-props="{ 'aria-label': '本级经验' }"
            /></NFormItem>
            <NFormItem label="VIP等级"
              ><NInputNumber
                v-model:value="draft.vip"
                :min="0"
                :max="options?.vipMax"
                :precision="0"
                :input-props="{ 'aria-label': 'VIP等级' }"
            /></NFormItem>
            <NFormItem label="主线章节"
              ><NSelect v-model:value="draft.chapter" :options="options?.chapters" @update:value="draft.mission = ''"
            /></NFormItem>
            <NFormItem label="主线关卡"><NSelect v-model:value="draft.mission" :options="missions" /></NFormItem>
            <NFormItem label="所在场景"><NSelect v-model:value="draft.scene" :options="options?.scenes" /></NFormItem>
            <NFormItem label="创建时间"
              ><NDatePicker v-model:value="draft.createdAt" type="datetime" :clearable="false"
            /></NFormItem>
          </div>
          <p class="ops-muted">
            主线修改会同步通关状态，不发放关卡奖励，已领取记录保留。修改创建时间后，已记录活动与领奖历史保留。
          </p>
          <h3>当前资源</h3>
          <div class="profile-grid">
            <NFormItem v-for="(label, key) in balanceLabels" :key="key" :label="label"
              ><NInputNumber
                v-model:value="draft.balances[key]"
                :min="0"
                :max="options?.balanceLimits[key]"
                :precision="0"
                :input-props="{ 'aria-label': label }"
            /></NFormItem>
          </div>
          <NFormItem label="操作原因" required
            ><NInput
              v-model:value="reason"
              type="textarea"
              :maxlength="200"
              :input-props="{ 'aria-label': '资料操作原因' }"
          /></NFormItem> </NForm></template
    ></NSpin>
    <template #footer
      ><div class="ops-drawer-actions">
        <NButton :disabled="busy" @click="guard.requestClose()">取消</NButton
        ><NButton
          v-if="canWrite"
          type="primary"
          :loading="busy"
          :disabled="loading || !options || conflict"
          @click="save"
          >保存修改</NButton
        >
      </div></template
    >
  </NModal>
</template>
<style scoped>
.profile-modal {
  display: flex;
  flex-direction: column;
}
.profile-modal :deep(.n-card__content) {
  min-height: 0;
  overflow-y: auto;
}
.profile-modal :deep(.n-card__footer) {
  flex-shrink: 0;
}
@media (max-width: 600px) {
  .ops-drawer-actions {
    display: flex;
    flex-wrap: wrap;
  }
  .ops-drawer-actions :deep(.n-button) {
    flex: 1;
  }
}
.profile-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 16px;
}
.profile-grid :deep(.n-input-number),
.profile-grid :deep(.n-date-picker) {
  width: 100%;
}
@media (max-width: 600px) {
  .profile-grid {
    grid-template-columns: 1fr;
  }
}
.profile-modal h3 { margin: 20px 0 16px; font-size: 16px; font-weight: 600; }
</style>
