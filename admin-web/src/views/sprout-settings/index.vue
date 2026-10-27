<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useDraftGuard } from '@/composables/useDraftGuard';
import { useOperations } from '@/composables/useOperations';
import { ApiError, api, errorMessage, json } from '@/service/operations';
import type { SproutSettings } from '@/service/sprout-settings';

const { canWrite } = useOperations();
const sproutIcon = `${import.meta.env.BASE_URL}media/items/ggo_sprout.png`;
const saved = ref<SproutSettings | null>(null);
const draft = reactive({ enabled: false, days: null as number | null, newCharacterEnabled: true, returnEnabled: false, returnOfflineDays: null as number | null, returnDays: null as number | null, reason: '' });
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const conflict = ref(false);
const guard = useDraftGuard({
  active: () => Boolean(saved.value), value: () => draft, busy: () => loading.value || saving.value,
  writable: () => canWrite.value, label: '小芽设置', scope: false, close: () => reset()
});
const eligible = computed(() => draft.days === null ? '请填写小芽有效天数' : `角色创建后的前 ${draft.days} 天内显示小芽标识`);
const validDays = (v: number | null) => v !== null && Number.isInteger(v) && v >= 1 && v <= 3650;
const valid = computed(() => (!draft.enabled || !draft.newCharacterEnabled || validDays(draft.days)) && (!draft.returnEnabled || (validDays(draft.returnDays) && validDays(draft.returnOfflineDays))) && [draft.days, draft.returnDays, draft.returnOfflineDays].every(v => v === null || validDays(v)));
function reset() {
  if (!saved.value) return;
  draft.enabled = saved.value.enabled;
  draft.days = saved.value.days;
  draft.newCharacterEnabled = saved.value.newCharacterEnabled ?? true;
  draft.returnEnabled = saved.value.returnEnabled ?? false;
  draft.returnOfflineDays = saved.value.returnOfflineDays;
  draft.returnDays = saved.value.returnDays;
  draft.reason = '';
  error.value = '';
  guard.capture();
}
async function load() {
  if (loading.value || saving.value || !await guard.canLeave()) return;
  loading.value = true;
  error.value = '';
  try {
    saved.value = await api<SproutSettings>('/sprout-settings');
    conflict.value = false;
    reset();
  } catch (e) { error.value = errorMessage(e); }
  finally { loading.value = false; }
}
async function save() {
  if (!saved.value || saving.value || loading.value || conflict.value || !canWrite.value || !guard.dirty.value) return;
  if (!valid.value) { error.value = '请为已开启的新角色或回归规则填写 1～3650 的整数天数'; return; }
  if (!draft.reason.trim()) { error.value = '请填写修改原因'; return; }
  saving.value = true;
  error.value = '';
  try {
    saved.value = await api<SproutSettings>('/sprout-settings', json('PUT', {
      enabled: draft.enabled, days: draft.days, newCharacterEnabled: draft.newCharacterEnabled, returnEnabled: draft.returnEnabled, returnOfflineDays: draft.returnOfflineDays, returnDays: draft.returnDays, version: saved.value.version, reason: draft.reason.trim()
    }));
    reset();
    window.$message?.success('小芽设置已保存');
  } catch (e) {
    error.value = errorMessage(e);
    if (e instanceof ApiError && e.status === 409) conflict.value = true;
  } finally { saving.value = false; }
}
onMounted(load);
</script>

<template>
  <div class="ops-page sprout-page">
    <div class="ops-filters"><NButton :loading="loading" :disabled="saving" @click="load">刷新</NButton></div>
    <NAlert v-if="error && !conflict" type="error" :title="error" />
    <NAlert v-if="conflict" type="warning" title="配置已更新，当前输入已保留。请记录修改内容后刷新，再重新保存。" />
    <NCard :bordered="false" class="ops-panel">
      <NSpin :show="loading">
        <template v-if="saved">
          <div class="sprout-preview">
            <img :src="sproutIcon" alt="小芽标识" width="44" height="44" />
            <div><strong>小芽标识</strong><p class="ops-muted">新角色与回归角色分别配置，全区服共用。</p></div>
            <NTag :bordered="false" :type="draft.enabled ? 'success' : 'default'">{{ draft.enabled ? '规则已启用' : '规则未启用' }}</NTag>
          </div>

          <NForm label-placement="top" :disabled="saving || !canWrite" @submit.prevent="save">
            <NFormItem label="启用小芽规则"><NSwitch v-model:value="draft.enabled" aria-label="启用小芽规则" /></NFormItem>
            <div class="sprout-group"><h3>新角色</h3>
            <NFormItem label="新角色赠送小芽"><NSwitch v-model:value="draft.newCharacterEnabled" aria-label="新角色赠送小芽" /></NFormItem>
            <NFormItem v-if="draft.newCharacterEnabled" label="新角色有效天数" :feedback="eligible">
              <NInputNumber v-model:value="draft.days" :min="1" :max="3650" :precision="0" clearable placeholder="待设定" :input-props="{ 'aria-label': '小芽有效天数' }" class="sprout-days">
                <template #suffix>天</template>
              </NInputNumber>
            </NFormItem>
            </div>
            <div class="sprout-group"><h3>回归角色</h3>
              <NFormItem label="回归赠送小芽"><NSwitch v-model:value="draft.returnEnabled" aria-label="回归赠送小芽" /></NFormItem>
              <template v-if="draft.returnEnabled">
                <NFormItem label="连续离线门槛" feedback="按最后成功登录和场景活动时间中较晚者计算；首次未知历史不补发。"><NInputNumber v-model:value="draft.returnOfflineDays" :input-props="{ 'aria-label': '连续离线门槛' }" :min="1" :max="3650" :precision="0" clearable placeholder="待设定" class="sprout-days"><template #suffix>天</template></NInputNumber></NFormItem>
                <NFormItem label="回归有效天数" feedback="满足离线门槛后，从成功登录时刻起计算。"><NInputNumber v-model:value="draft.returnDays" :input-props="{ 'aria-label': '回归有效天数' }" :min="1" :max="3650" :precision="0" clearable placeholder="待设定" class="sprout-days"><template #suffix>天</template></NInputNumber></NFormItem>
              </template>
            </div>
            <NFormItem v-if="canWrite" label="修改原因"><NInput v-model:value="draft.reason" placeholder="填写本次调整原因" :maxlength="500" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" :input-props="{ 'aria-label': '修改原因' }" /></NFormItem>
            <div v-if="canWrite" class="sprout-actions">
              <NButton :disabled="saving || loading || !guard.dirty.value" @click="reset">还原修改</NButton>
              <NButton type="primary" :loading="saving" :disabled="loading || conflict || !guard.dirty.value" attr-type="submit">保存</NButton>
            </div>
            <p v-else class="ops-muted">当前账号可查看设置。</p>
          </NForm>
        </template>
        <NEmpty v-else-if="!loading" description="小芽设置读取失败，请刷新重试" />
      </NSpin>
    </NCard>
  </div>
</template>

<style scoped>
.sprout-page { max-width: 900px; }
.sprout-preview { display: grid; grid-template-columns: 44px minmax(0, 1fr) auto; align-items: center; gap: 14px; margin-bottom: 24px; }
.sprout-preview > div { min-width: 0; }
.sprout-preview img { object-fit: contain; flex-shrink: 0; }
.sprout-preview strong { font-size: 16px; font-weight: 600; }
.sprout-preview p { margin: 4px 0 0; line-height: 1.6; }
.sprout-preview .n-tag { margin-left: auto; }
.sprout-page .n-form { max-width: 620px; }
.sprout-group { border-top: 1px solid var(--n-border-color, #e5e7eb); padding-top: 16px; margin-bottom: 20px; }
.sprout-group h3 { font-size: 15px; margin: 0 0 16px; font-weight: 600; }
.sprout-days { width: 260px; max-width: 100%; }
.sprout-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 12px; margin-top: 8px; }
@media (max-width: 640px) { .sprout-preview { grid-template-columns: 36px minmax(0, 1fr); gap: 10px 12px; } .sprout-preview img { width: 36px; height: 36px; } .sprout-preview .n-tag { grid-column: 2; justify-self: start; margin-left: 0; } .sprout-days { width: 100%; } }
</style>
