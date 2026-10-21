<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
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
const editing = ref(false);
const error = ref('');
const dirty = computed(() => Boolean(selected.value && JSON.stringify(selected.value) !== baseline.value));
const disabled = computed(() => !canWrite.value || loading.value || saving.value);
const options = computed(() => rows.value.map(row => ({ value: row.id, label: `${row.name} · ${row.enabled ? '已开启' : '已停用'}` })));
const serverOptions = computed(() => scope.servers.map(row => ({ value: row.id, label: row.name })));
let generation = 0;
let removeGuard: (() => void) | undefined;
let discard: Promise<boolean> | null = null;
let disposeDiscard: (() => void) | undefined;
function apply(row: PersonalOpeningActivity | null) {
  selected.value = row ? JSON.parse(JSON.stringify(row)) : null;
  baseline.value = JSON.stringify(selected.value);
}
async function beforeLeave(): Promise<boolean> {
  if (saving.value) return false;
  if (!dirty.value) return true;
  if (discard) return discard;
  if (!window.$dialog) return false;
  discard = new Promise<boolean>(resolve => {
    let settled = false;
    const finish = (ok: boolean) => { if (settled) return; settled = true; discard = null; disposeDiscard = undefined; if (ok) apply(rows.value.find(row => row.id === selected.value?.id) || null); resolve(ok); };
    const dialog = window.$dialog!.warning({ title: '放弃未保存的个人活动配置？', positiveText: '放弃修改', negativeText: '继续编辑', onPositiveClick: () => finish(true), onNegativeClick: () => finish(false), onClose: () => finish(false), onMaskClick: () => finish(false), onEsc: () => finish(false) });
    disposeDiscard = () => { finish(false); dialog.destroy(); };
  });
  return discard;
}
async function load() {
  const current = ++generation;
  const server = scope.serverId;
  rows.value = []; apply(null); editing.value = false; error.value = '';
  if (!server) return;
  loading.value = true;
  try {
    const result = await getPersonalOpeningActivities(server);
    if (current !== generation || scope.serverId !== server) return;
    rows.value = result.items; apply(rows.value[0] || null);
  } catch (cause) { if (current === generation) error.value = errorMessage(cause); }
  finally { if (current === generation) loading.value = false; }
}
async function choose(id: string) {
  if (disabled.value || !(await beforeLeave())) return;
  apply(rows.value.find(row => row.id === id) || null); editing.value = false;
}
async function closeEditor() {
  if (await beforeLeave()) editing.value = false;
}
async function save() {
  if (!selected.value || !scope.serverId || disabled.value || !dirty.value) return;
  const server = scope.serverId;
  const current = generation;
  saving.value = true; error.value = '';
  try {
    const result = await savePersonalOpeningActivity(server, selected.value);
    if (server !== scope.serverId || current !== generation) return;
    rows.value = rows.value.map(row => row.id === result.id ? result : row); apply(result);
    window.$message?.success('个人活动已保存，已生成实例保留原规则');
  } catch (cause) { error.value = errorMessage(cause); }
  finally { saving.value = false; }
}
function beforeUnload(event: BeforeUnloadEvent) { if (dirty.value || saving.value) { event.preventDefault(); event.returnValue = ''; } }
watch(() => scope.serverId, load);
onBeforeRouteLeave(beforeLeave);
onMounted(async () => { removeGuard = scope.addSwitchGuard(beforeLeave); window.addEventListener('beforeunload', beforeUnload); await scope.loadServers(); await load(); });
onBeforeUnmount(() => { generation += 1; removeGuard?.(); disposeDiscard?.(); window.removeEventListener('beforeunload', beforeUnload); });
defineExpose({ beforeLeave, dirty, saving });
</script>

<template>
  <section class="personal-opening">
    <div class="personal-heading"><h3>个人开服活动</h3><NSelect :value="scope.serverId" :options="serverOptions" :disabled="saving || loading" placeholder="选择区服" aria-label="个人活动所属区服" class="server-select" @update:value="scope.requestServerChange" /></div>
    <p class="ops-inline-note">从角色创建日计算，过期不补开。调整规则仅影响之后生成的个人实例；全服限时活动仍在排期中配置。</p>
    <NAlert v-if="error" type="error" class="mb-12px">{{ error }}</NAlert>
    <NSpin :show="loading">
      <NEmpty v-if="!scope.serverId" description="选择区服后查看个人活动" />
      <NEmpty v-else-if="!loading && !rows.length" description="本服尚无个人开服活动配置"><template #extra><NButton size="small" @click="load">重新加载</NButton></template></NEmpty>
      <template v-else-if="selected">
        <div class="personal-selection">
          <NSelect :value="selected.id" :options="options" :disabled="saving || loading" aria-label="选择个人开服活动" @update:value="choose" />
          <NButton v-if="!editing" @click="editing = true">配置个人活动</NButton>
          <NButton v-else :disabled="saving" @click="closeEditor">收起配置</NButton>
        </div>
        <p v-if="!editing" class="personal-summary">第 {{ selected.startDay }} 天开始，持续 {{ selected.durationDays }} 天，领奖宽限 {{ selected.claimDays }} 天</p>
        <NForm v-if="editing" label-placement="top" :disabled="disabled">
          <div class="personal-fields">
            <NFormItem label="开始日"><NInputNumber v-model:value="selected.startDay" :min="1" :max="366" :precision="0" /></NFormItem>
            <NFormItem label="持续天数"><NInputNumber v-model:value="selected.durationDays" :min="1" :max="366" :precision="0" /></NFormItem>
            <NFormItem label="领奖宽限天数"><NInputNumber v-model:value="selected.claimDays" :min="0" :max="90" :precision="0" /></NFormItem>
            <NFormItem label="开放状态"><NSwitch v-model:value="selected.enabled" :disabled="disabled" /></NFormItem>
          </div>
          <ActivityMilestones v-model="selected.form.milestones" :mode="selected.mode" :disabled="disabled" :max="31" />
        </NForm>
        <div v-if="editing" class="personal-actions"><NButton :disabled="!dirty || saving" @click="beforeLeave">取消修改</NButton><NButton v-if="canWrite" type="primary" :disabled="!dirty || disabled" :loading="saving" @click="save">保存个人活动</NButton></div>
      </template>
    </NSpin>
  </section>
</template>

<style scoped>
.personal-opening { margin-bottom: 28px; padding-bottom: 24px; border-bottom: 1px solid var(--n-border-color, #e5e7eb); }
.personal-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 8px; }
.personal-heading h3 { margin: 0; font-size: 16px; font-weight: 600; }
.server-select { width: 220px; }
.personal-selection { display: flex; gap: 12px; align-items: center; margin: 16px 0; }
.personal-selection :deep(.n-select) { flex: 1; min-width: 0; }
.personal-summary { margin: 0; color: var(--ops-text-secondary, #667085); }
.personal-fields { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 16px; }
.personal-fields :deep(.n-input-number) { width: 100%; }
.personal-actions { display: flex; justify-content: flex-end; gap: 12px; padding-top: 16px; }
@media (max-width: 640px) { .personal-heading { align-items: stretch; flex-direction: column; } .server-select { width: 100%; } .personal-selection { align-items: stretch; flex-direction: column; } .personal-fields { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px 12px; } }
</style>
