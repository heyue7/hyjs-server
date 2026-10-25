<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useDraftGuard } from '@/composables/useDraftGuard';
import { useOperations } from '@/composables/useOperations';
import { ApiError, errorMessage } from '@/service/operations';
import { getTrialConfig, previewTrial, saveTrialConfig } from '@/service/trials';
import type { TrialConfig, TrialGeneration, TrialPreview } from '@/service/trials';

const { canWrite } = useOperations();
const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value));
const saved = ref<TrialConfig | null>(null);
const draft = reactive({ enabled: false, config: null as TrialGeneration | null, reason: '' });
const loading = ref(false);
const saving = ref(false);
const previewing = ref(false);
const conflict = ref(false);
const error = ref('');
const floor = ref<number | null>(1);
const preview = ref<TrialPreview[]>([]);
const previewSignature = ref('');
const signature = computed(() => JSON.stringify({ config: draft.config, floor: floor.value }));
const stalePreview = computed(() => preview.value.length > 0 && previewSignature.value !== signature.value);
const guard = useDraftGuard({ active: () => Boolean(saved.value), value: () => draft,
  busy: () => loading.value || saving.value, writable: () => canWrite.value, label: '试炼配置', scope: false, close: () => reset() });
function reset() {
  if (!saved.value) return;
  draft.enabled = saved.value.enabled;
  draft.config = clone(saved.value.config);
  draft.reason = '';
  error.value = '';
  guard.capture();
}
async function load() {
  if (loading.value || saving.value || !await guard.canLeave()) return;
  loading.value = true; error.value = '';
  try { saved.value = await getTrialConfig(); conflict.value = false; reset(); preview.value = []; }
  catch (e) { error.value = errorMessage(e); }
  finally { loading.value = false; }
}
async function generatePreview() {
  if (!draft.config || previewing.value || saving.value) return;
  if (!Number.isInteger(floor.value) || (floor.value ?? 0) < 1 || (floor.value ?? 0) > draft.config.floors) { error.value = '请选择 1～102 层'; return; }
  previewing.value = true; error.value = '';
  const submitted = signature.value;
  try { const out = await previewTrial(clone(draft.config), floor.value!); preview.value = out.items; previewSignature.value = submitted; }
  catch (e) { error.value = errorMessage(e); }
  finally { previewing.value = false; }
}
async function save() {
  if (!saved.value || !draft.config || saving.value || loading.value || conflict.value || !canWrite.value || !guard.dirty.value) return;
  if (!draft.reason.trim()) { error.value = '请填写修改原因'; return; }
  saving.value = true; error.value = '';
  try {
    saved.value = await saveTrialConfig({ expectedVersion: saved.value.version, enabled: draft.enabled,
      config: clone(draft.config), reason: draft.reason.trim() });
    reset(); window.$message?.success('试炼配置已保存，下一场战斗使用新目录');
  } catch (e) { error.value = errorMessage(e); if (e instanceof ApiError && e.status === 409) conflict.value = true; }
  finally { saving.value = false; }
}
const number = (value: number) => value.toLocaleString('zh-CN');
watch(() => draft.config?.roster.length, (length) => {
  if (length && draft.config) draft.config.maxCount = Math.min(draft.config.maxCount, length);
});
onMounted(load);
</script>

<template>
  <div class="ops-page trial-page">
    <div class="ops-filters">
      <NButton :loading="loading" :disabled="saving" @click="load">刷新</NButton>
      <RouterLink v-slot="{ href, navigate }" to="/shops/point" custom>
        <NButton tag="a" :href="href" @click="navigate">试炼商店</NButton>
      </RouterLink>
    </div>
    <NAlert v-if="error" type="error" :title="error" />
    <NAlert v-if="conflict" type="warning" title="配置已被其他人更新。当前输入已保留，请记录后刷新。" />
    <NCard :bordered="false" class="ops-panel">
      <NSpin :show="loading">
        <template v-if="saved && draft.config">
          <div class="trial-status">
            <NTag :type="saved.enabled ? 'success' : 'default'" :bordered="false">{{ saved.enabled ? '普通试炼已启用' : '普通试炼未启用' }}</NTag>
            <span>当前已发布 {{ saved.publishedFloorCount }} 层 · 配置版本 {{ saved.version }}</span>
            <span v-if="saved.publishedAt" class="ops-muted">{{ new Date(saved.publishedAt).toLocaleString('zh-CN') }}</span>
          </div>
          <NForm label-placement="top" :disabled="saving || !canWrite" @submit.prevent="save">
            <div class="trial-section">
              <h3>生成规则</h3>
              <div class="trial-settings">
                <NFormItem label="启用普通试炼" feedback="关闭后暂停普通试炼新开战，精英试炼不受影响。"><NSwitch v-model:value="draft.enabled" aria-label="启用普通试炼" /></NFormItem>
                <NFormItem label="总层数"><NInputNumber :value="draft.config.floors" disabled /></NFormItem>
                <NFormItem v-for="(_, index) in draft.config.difficultyFactors" :key="index" :label="`${index + 1} 星属性倍率`">
                  <NInputNumber v-model:value="draft.config.difficultyFactors[index]" :min="0.01" :input-props="{ 'aria-label': `${index + 1}星属性倍率` }" />
                </NFormItem>
              </div>
              <h4>属性曲线</h4>
              <div class="trial-table-wrap"><table class="trial-table"><thead><tr><th>楼层</th><th>生命</th><th>攻击</th><th>物防与法防</th></tr></thead>
                <tbody><tr v-for="(anchor, index) in draft.config.anchors" :key="index">
                  <td><NInputNumber v-model:value="anchor.floor" :min="1" :max="102" :precision="0" :disabled="index === 0 || index === draft.config.anchors.length - 1" :input-props="{ 'aria-label': `锚点${index + 1}楼层` }" /></td>
                  <td><NInputNumber v-model:value="anchor.hp" :min="1" :input-props="{ 'aria-label': `锚点${index + 1}生命` }" /></td>
                  <td><NInputNumber v-model:value="anchor.att" :min="1" :input-props="{ 'aria-label': `锚点${index + 1}攻击` }" /></td>
                  <td><NInputNumber v-model:value="anchor.def" :min="1" :input-props="{ 'aria-label': `锚点${index + 1}防御` }" /></td>
                </tr></tbody></table></div>
            </div>
            <div class="trial-section">
              <h3>敌方阵容</h3>
              <div class="trial-settings">
                <NFormItem label="起始人数"><NInputNumber v-model:value="draft.config.initialCount" :min="1" :max="draft.config.maxCount" :precision="0" /></NFormItem>
                <NFormItem label="每隔多少层增加 1 人"><NInputNumber v-model:value="draft.config.countEveryFloors" :min="1" :max="102" :precision="0" /></NFormItem>
                <NFormItem label="最多人数"><NInputNumber v-model:value="draft.config.maxCount" :min="draft.config.initialCount" :max="draft.config.roster.length" :precision="0" /></NFormItem>
              </div>
              <p class="ops-muted">按下列顺序加入阵容，角色须具备模型与技能资源。</p>
              <div class="trial-roster"><div v-for="(_, index) in draft.config.roster" :key="index" class="trial-roster-row">
                <span>{{ index + 1 }}</span><NInput v-model:value="draft.config.roster[index]" :input-props="{ 'aria-label': `第${index + 1}名敌人资源ID` }" />
                <NInputNumber v-model:value="draft.config.positions[index]" :min="1" :max="15" :precision="0" :input-props="{ 'aria-label': `第${index + 1}名敌人站位` }"><template #prefix>站位</template></NInputNumber>
              </div></div>
            </div>
            <div class="trial-section">
              <h3>三难度预览</h3>
              <div class="trial-preview-tools"><NInputNumber v-model:value="floor" :min="1" :max="102" :precision="0" :disabled="previewing || saving" :input-props="{ 'aria-label': '预览楼层' }"><template #suffix>层</template></NInputNumber><NButton :loading="previewing" :disabled="saving" @click="generatePreview">生成预览</NButton><span v-if="stalePreview" class="ops-muted">参数已修改，请重新预览</span></div>
              <NEmpty v-if="!preview.length" description="暂无预览" />
              <div v-for="item in preview" :key="item.star" class="trial-preview">
                <h4>第 {{ item.floor }} 层 · {{ item.star }} 星 <span>推荐战力 {{ number(item.recommendedPower) }}</span></h4>
                <div class="trial-table-wrap"><table class="trial-table"><thead><tr><th>敌人</th><th>站位</th><th>生命</th><th>攻击</th><th>双防</th></tr></thead><tbody><tr v-for="enemy in item.enemies" :key="enemy.pose"><td>{{ enemy.name }}</td><td>{{ enemy.pose }}</td><td>{{ number(enemy.hp) }}</td><td>{{ number(enemy.att) }}</td><td>{{ number(enemy.def) }}</td></tr></tbody></table></div>
              </div>
            </div>
            <p class="ops-muted trial-save-note">保存后用于下一场新战斗，当前进度、星数、已选加成与进行中的战斗保留。已开始旧首组的轮次完成前三层后进入新目录；按游戏内次数与消耗重置后也可使用新目录。</p>
            <NFormItem v-if="canWrite" label="修改原因"><NInput v-model:value="draft.reason" type="textarea" :maxlength="500" placeholder="填写本次调整原因" :autosize="{ minRows: 2, maxRows: 4 }" /></NFormItem>
            <div v-if="canWrite" class="trial-actions"><NButton :disabled="saving || loading || !guard.dirty.value" @click="reset">还原修改</NButton><NButton type="primary" :loading="saving" :disabled="loading || conflict || !guard.dirty.value" attr-type="submit">保存并重新生成</NButton></div>
            <p v-else class="ops-muted">当前账号可查看配置和生成预览。</p>
          </NForm>
        </template>
        <NEmpty v-else-if="!loading" description="试炼配置读取失败，请刷新重试" />
      </NSpin>
    </NCard>
  </div>
</template>

<style scoped>
.trial-page { max-width: 1200px; }
.trial-status { display: flex; align-items: center; flex-wrap: wrap; gap: 12px; }
.trial-save-note { margin: 20px 0; line-height: 1.7; overflow-wrap: anywhere; }
.trial-section { margin: 28px 0; }
h3 { font-size: 16px; font-weight: 600; margin: 0 0 16px; }
h4 { font-size: 14px; font-weight: 600; margin: 20px 0 12px; }
.trial-settings { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 0 20px; }
.trial-table-wrap { overflow-x: auto; }
.trial-table { width: 100%; min-width: 560px; border-collapse: collapse; text-align: left; }
.trial-table th { font-weight: 500; color: var(--n-title-text-color); }
.trial-table th, .trial-table td { padding: 10px 12px; border-bottom: 1px solid var(--n-border-color); }
.trial-table td { font-variant-numeric: tabular-nums; }
.trial-table .n-input-number { min-width: 120px; }
.trial-roster { display: grid; gap: 10px; max-width: 680px; }
.trial-roster-row { display: grid; grid-template-columns: 20px minmax(0, 1fr) 150px; gap: 12px; align-items: center; }
.trial-preview-tools { display: flex; flex-wrap: wrap; align-items: center; gap: 12px; margin-bottom: 20px; }
.trial-preview-tools > .n-input-number { width: 150px; }
.trial-preview h4 { display: flex; flex-wrap: wrap; justify-content: space-between; gap: 12px; }
.trial-preview h4 span { font-weight: 500; }
.trial-actions { display: flex; justify-content: flex-end; flex-wrap: wrap; gap: 12px; }
@media (max-width: 640px) { .trial-settings { grid-template-columns: 1fr; } .trial-roster-row { grid-template-columns: 20px minmax(0, 1fr); } .trial-roster-row > .n-input-number { grid-column: 2; width: 100%; } .trial-actions > button { flex: 1; } }
</style>
