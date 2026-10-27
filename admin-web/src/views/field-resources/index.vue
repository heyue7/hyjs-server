<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useDraftGuard } from '@/composables/useDraftGuard';
import { useOperations } from '@/composables/useOperations';
import { ApiError, errorMessage } from '@/service/operations';
import { getFieldResources, saveFieldResources } from '@/service/field-resources';
import type { FieldResources } from '@/service/field-resources';

const { canWrite } = useOperations();
const saved = ref<FieldResources | null>(null);
const draft = reactive({ unlimited: true, count: null as number | null, reason: '', gatherRefreshHours: [0, 12] as number[] });
const hourOptions = Array.from({ length: 24 }, (_, hour) => ({ label: `${String(hour).padStart(2, '0')}:00`, value: hour }));
const loading = ref(false);
const saving = ref(false);
const conflict = ref(false);
const error = ref('');
const guard = useDraftGuard({ active: () => Boolean(saved.value), value: () => draft,
  busy: () => loading.value || saving.value, writable: () => canWrite.value, label: '野外资源设置', scope: false, close: () => reset() });
const valid = computed(() => draft.gatherRefreshHours.length > 0 && (draft.unlimited || Number.isInteger(draft.count) && draft.count !== null && draft.count >= 0 && draft.count <= 1000000));
function reset() {
  if (!saved.value) return;
  draft.unlimited = saved.value.bossDailyRewards === -1;
  draft.count = draft.unlimited ? null : saved.value.bossDailyRewards;
  draft.gatherRefreshHours = [...saved.value.gatherRefreshHours];
  draft.reason = ''; error.value = ''; guard.capture();
}
async function load() {
  if (loading.value || saving.value || !await guard.canLeave()) return;
  loading.value = true; error.value = '';
  try { saved.value = await getFieldResources(); conflict.value = false; reset(); }
  catch (e) { error.value = errorMessage(e); }
  finally { loading.value = false; }
}
async function save() {
  if (!saved.value || !canWrite.value || saving.value || loading.value || conflict.value || !guard.dirty.value) return;
  if (!valid.value) { error.value = '请选择采集刷新时刻并填写有效领奖次数'; return; }
  if (!draft.reason.trim()) { error.value = '请填写修改原因'; return; }
  saving.value = true; error.value = '';
  try {
    saved.value = await saveFieldResources({ version: saved.value.version, gatherRefreshHours: [...draft.gatherRefreshHours].sort((a, b) => a - b), bossDailyRewards: draft.unlimited ? -1 : draft.count!, reason: draft.reason.trim() });
    reset(); window.$message?.success('野外资源设置已保存');
  } catch (e) { error.value = errorMessage(e); if (e instanceof ApiError && e.status === 409) conflict.value = true; }
  finally { saving.value = false; }
}
onMounted(load);
</script>

<template>
  <div class="ops-page field-resource-page">
    <div class="ops-filters"><NButton :loading="loading" :disabled="saving" @click="load">刷新</NButton></div>
    <NAlert v-if="error" type="error" :title="error" />
    <NAlert v-if="conflict" type="warning" title="设置已被其他人修改。当前输入已保留，请记录后刷新。" />
    <NCard :bordered="false" class="ops-panel">
      <NSpin :show="loading">
        <template v-if="saved">
          <NForm label-placement="top" :disabled="saving || !canWrite" @submit.prevent="save">
            <section class="resource-group" aria-labelledby="gather-heading">
              <h3 id="gather-heading">矿点与鱼点刷新</h3>
              <NFormItem label="每日刷新时刻（北京时间）">
                <NSelect v-model:value="draft.gatherRefreshHours" multiple :options="hourOptions" :clearable="false" placeholder="至少选择一个时刻" aria-label="每日采集刷新时刻" />
              </NFormItem>
              <p class="ops-muted">到时恢复所有采集点的次数并重新生成资源。正在进行的采集结束后刷新该点。</p>
            </section>
            <section class="resource-group" aria-labelledby="boss-heading">
              <h3 id="boss-heading">野外 Boss 每日奖励</h3>
              <p class="ops-muted">每个角色在所有野外地图共用总次数。世界 Boss 使用独立规则。</p>
            <NFormItem label="不限领奖次数"><NSwitch v-model:value="draft.unlimited" aria-label="不限领奖次数" /></NFormItem>
            <NFormItem v-if="!draft.unlimited" label="每天可领奖次数">
              <NInputNumber v-model:value="draft.count" :min="0" :max="1000000" :precision="0" :input-props="{ 'aria-label': '每天可领奖次数' }" />
            </NFormItem>
            <p class="ops-muted">一场战斗的参与奖与击杀奖合计计 1 次。0 表示不发放奖励，战斗仍可进行。</p>
            <p class="ops-muted">次数按游戏日期每日重置。保存后新开战生效，进行中的战斗及已领取回执保留。</p>
            </section>
            <NFormItem v-if="canWrite" label="修改原因"><NInput v-model:value="draft.reason" type="textarea" placeholder="填写本次调整原因" :maxlength="500" :autosize="{ minRows: 2, maxRows: 4 }" /></NFormItem>
            <div v-if="canWrite" class="resource-actions"><NButton :disabled="saving || !guard.dirty.value" @click="reset">还原修改</NButton><NButton type="primary" attr-type="submit" :loading="saving" :disabled="loading || conflict || !valid || !guard.dirty.value">保存设置</NButton></div>
            <p v-else class="ops-muted">当前账号可查看设置。</p>
          </NForm>
        </template>
        <NEmpty v-else-if="!loading" description="野外资源设置读取失败，请刷新重试" />
      </NSpin>
    </NCard>
  </div>
</template>

<style scoped>
.field-resource-page { max-width: 900px; }
h3 { font-size: 16px; font-weight: 600; margin: 0 0 12px; }
p { margin: 0 0 20px; line-height: 1.6; }
.field-resource-page .n-form { max-width: 720px; }
.resource-group { margin-bottom: 24px; }
.resource-group + .resource-group { padding-top: 20px; border-top: 1px solid rgb(var(--base-text-color) / .09); }
.n-select { max-width: 560px; }
.n-input-number { width: 220px; max-width: 100%; }
.resource-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 12px; }
@media (max-width: 640px) { .resource-actions > button { flex: 1; } }
</style>
