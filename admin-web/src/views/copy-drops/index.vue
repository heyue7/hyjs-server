<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue';
import { useDraftGuard } from '@/composables/useDraftGuard';
import { useOperations } from '@/composables/useOperations';
import { ApiError, api, errorMessage, json } from '@/service/operations';
interface Rule { enabled: boolean; cycle: number; version: string; source: string }
const { canWrite } = useOperations();
const saved = ref<Rule | null>(null);
const draft = reactive({ enabled: false, cycle: 4 as number | null, reason: '' });
const loading = ref(false), saving = ref(false), conflict = ref(false), error = ref('');
const guard = useDraftGuard({ active: () => Boolean(saved.value), value: () => draft, busy: () => loading.value || saving.value, writable: () => canWrite.value, label: '副本碎片保底', scope: false, close: () => reset() });
function reset() { if (!saved.value) return; draft.enabled = saved.value.enabled; draft.cycle = saved.value.cycle; draft.reason = ''; error.value = ''; guard.capture(); }
async function load() { if (loading.value || saving.value || !await guard.canLeave()) return; loading.value = true; error.value = ''; try { saved.value = await api<Rule>('/copy-drops'); conflict.value = false; reset(); } catch(e) { error.value = errorMessage(e); } finally { loading.value = false; } }
async function save() { if (!saved.value || saving.value || loading.value || conflict.value || !canWrite.value || !guard.dirty.value) return; if (!Number.isInteger(draft.cycle) || (draft.cycle ?? 0) < 1 || (draft.cycle ?? 0) > 10000 || !draft.reason.trim()) { error.value = '请填写1～10000的整数周期和修改原因'; return; } saving.value = true; error.value = ''; try { saved.value = await api<Rule>('/copy-drops', json('PUT', { ...draft, reason: draft.reason.trim(), version: saved.value.version })); reset(); window.$message?.success('副本碎片保底已保存'); } catch(e) { error.value = errorMessage(e); if(e instanceof ApiError && e.status === 409) conflict.value = true; } finally { saving.value = false; } }
onMounted(load);
</script>
<template>
 <div class="ops-page copy-drops-page">
  <div class="ops-filters"><NButton :loading="loading" :disabled="saving" @click="load">刷新</NButton></div>
  <NAlert v-if="error && !conflict" type="error" :title="error" />
  <NAlert v-if="conflict" type="warning" title="配置已更新，当前输入已保留。请记录修改内容后刷新。" />
  <NCard :bordered="false" class="ops-panel"><NSpin :show="loading"><template v-if="saved">
   <h2>副本碎片保底</h2><p class="ops-muted">原关卡碎片按成功次数累计；普通战斗和扫荡共用进度。默认每4次成功发放一次，失败不累计。四件原整装、红装备和红饰品碎片保持禁发。</p>
   <NForm label-placement="top" :disabled="saving || !canWrite" @submit.prevent="save">
    <NFormItem label="启用碎片保底"><NSwitch v-model:value="draft.enabled" aria-label="启用碎片保底" /></NFormItem>
    <NFormItem label="成功次数周期" feedback="新进入的战斗与扫荡使用保存后的周期，已进入的战斗保留原规则。"><NInputNumber v-model:value="draft.cycle" :min="1" :max="10000" :precision="0" :input-props="{ 'aria-label': '成功次数周期' }" class="cycle-input"><template #suffix>次</template></NInputNumber></NFormItem>
    <NFormItem v-if="canWrite" label="修改原因"><NInput v-model:value="draft.reason" type="textarea" :maxlength="500" :autosize="{ minRows: 2, maxRows: 4 }" /></NFormItem>
    <div v-if="canWrite" class="actions"><NButton :disabled="saving || !guard.dirty.value" @click="reset">还原修改</NButton><NButton type="primary" attr-type="submit" :loading="saving" :disabled="loading || conflict || !guard.dirty.value">保存</NButton></div><p v-else class="ops-muted">当前账号可查看设置。</p>
   </NForm>
  </template><NEmpty v-else-if="!loading" description="配置读取失败，请刷新重试" /></NSpin></NCard>
 </div>
</template>
<style scoped>
.copy-drops-page { max-width: 900px; }
h2 { font-size: 18px; margin: 0 0 12px; }
.ops-muted { max-width: 65ch; line-height: 1.7; margin-bottom: 24px; }
.n-form { max-width: 620px; }
.cycle-input { width: 260px; max-width: 100%; }
.actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 12px; }
@media(max-width:640px) { .cycle-input { width: 100%; } }
@media(width < 640px) { h2 { display: none; } }
</style>
