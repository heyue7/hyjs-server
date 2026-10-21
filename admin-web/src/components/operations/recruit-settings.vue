<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { ApiError, errorMessage } from '@/service/operations';
import { getRecruitSettings, saveRecruitSettings } from '@/service/recruit-settings';
import type { RecruitSettings } from '@/service/recruit-settings';
import { useOperations } from '@/composables/useOperations';
import { useDraftGuard } from '@/composables/useDraftGuard';

interface DraftRow {
  kind: 2 | 3;
  name: string;
  dailyLimit: number | null;
  guaranteeEvery: number | null;
}

const { canWrite } = useOperations();
const snapshot = ref<RecruitSettings>();
const rows = ref<DraftRow[]>([]);
const reason = ref('调整招募规则');
const loading = ref(false);
const saving = ref(false);
const error = ref('');
const conflict = ref(false);
const defaultReason = '调整招募规则';
const maxDailyLimit = 100000;
const maxGuaranteeEvery = 10000;
let alive = true;
let loadGeneration = 0;

function content() {
  return JSON.stringify(
    rows.value.map(row => ({
      kind: row.kind,
      name: row.name,
      dailyLimit: row.dailyLimit,
      guaranteeEvery: row.guaranteeEvery
    }))
  );
}

const draft = useDraftGuard({
  active: () => Boolean(snapshot.value),
  value: content,
  busy: () => saving.value,
  writable: () => canWrite.value,
  label: '招募设置',
  close: () => {},
  scope: false
});

const dirty = draft.dirty;
const disabled = computed(() => !canWrite.value || loading.value || saving.value || conflict.value);

function kindLabel(kind: 2 | 3) {
  return kind === 2 ? '强者招募（橙卡）' : '王者招募';
}

function dailyLimitError(row: DraftRow) {
  const min = row.kind === 2 ? 0 : 1;
  if (row.dailyLimit === null || !Number.isInteger(row.dailyLimit))
    return `需为 ${min}–${maxDailyLimit.toLocaleString()} 的整数`;
  if (row.dailyLimit < min || row.dailyLimit > maxDailyLimit) return `范围 ${min}–${maxDailyLimit.toLocaleString()}`;
  return '';
}

function guaranteeError(row: DraftRow) {
  if (row.guaranteeEvery === null || !Number.isInteger(row.guaranteeEvery))
    return `需为 1–${maxGuaranteeEvery.toLocaleString()} 的整数`;
  if (row.guaranteeEvery < 1 || row.guaranteeEvery > maxGuaranteeEvery)
    return `范围 1–${maxGuaranteeEvery.toLocaleString()}`;
  return '';
}

const rowErrors = computed(() =>
  rows.value.map(row => ({
    daily: dailyLimitError(row),
    guarantee: guaranteeError(row)
  }))
);
const invalid = computed(() => rowErrors.value.some(row => row.daily || row.guarantee));

function dailyHint(row: DraftRow, index: number) {
  return rowErrors.value[index]?.daily || (row.kind === 2 ? '0 表示不限' : '免费招募也占 1 次');
}

function guaranteeHint(index: number) {
  return rowErrors.value[index]?.guarantee || '连续未命中时，最多 N 次触发保底';
}

function apply(data: RecruitSettings) {
  const items = Array.isArray(data.items) ? [...data.items].sort((a, b) => a.kind - b.kind) : [];
  if (items.length !== 2 || items[0].kind !== 2 || items[1].kind !== 3) throw new Error('招募配置不完整，请刷新重试。');
  snapshot.value = data;
  rows.value = items.map(item => ({
    kind: item.kind,
    name: item.name,
    dailyLimit: item.dailyLimit,
    guaranteeEvery: item.guaranteeEvery
  }));
  conflict.value = false;
  draft.capture();
}

async function load() {
  if (loading.value || saving.value) return;
  loadGeneration += 1;
  const generation = loadGeneration;
  loading.value = true;
  error.value = '';
  try {
    const data = await getRecruitSettings();
    if (!alive || generation !== loadGeneration) return;
    apply(data);
  } catch (cause) {
    if (alive && generation === loadGeneration) error.value = errorMessage(cause);
  } finally {
    if (alive && generation === loadGeneration) loading.value = false;
  }
}

async function refresh() {
  if (loading.value || saving.value) return;
  if (!(await draft.canLeave())) return;
  await load();
}

async function save() {
  if (!snapshot.value || disabled.value || !dirty.value || saving.value) return;
  error.value = '';
  if (invalid.value) {
    error.value = '请修正标红的招募规则后再保存。';
    return;
  }
  saving.value = true;
  try {
    const data = await saveRecruitSettings({
      version: snapshot.value.version,
      reason: reason.value.trim() || defaultReason,
      items: rows.value.map(row => ({
        kind: row.kind,
        name: row.name,
        dailyLimit: row.dailyLimit as number,
        guaranteeEvery: row.guaranteeEvery as number
      }))
    });
    if (!alive) return;
    apply(data);
    reason.value = defaultReason;
    window.$message?.success('招募规则已保存');
  } catch (cause) {
    if (!alive) return;
    error.value = errorMessage(cause);
    if (cause instanceof ApiError && cause.status === 409) conflict.value = true;
  } finally {
    if (alive) saving.value = false;
  }
}

onMounted(load);
onBeforeUnmount(() => {
  alive = false;
  loadGeneration += 1;
});

defineExpose({ canLeave: () => draft.canLeave(), dirty, saving });
</script>

<template>
  <div class="ops-page recruit-settings">
    <NAlert v-if="error" type="error" :title="error" />
    <NAlert v-if="conflict" type="warning" title="配置已被其他操作更新。当前输入已保留，请刷新后重新编辑。" />

    <NCard v-if="snapshot" :bordered="false" class="ops-panel">
      <NForm label-placement="top" :disabled="disabled">
        <div class="recruit-rules" role="group" aria-label="招募规则">
          <div v-for="(row, index) in rows" :key="row.kind" class="recruit-rule">
            <div class="recruit-rule-title">
              <strong>{{ kindLabel(row.kind) }}</strong>
              <span class="ops-muted">{{ row.kind === 2 ? '独立计算每日次数' : '三个世界共用每日次数' }}</span>
            </div>
            <NFormItem
              label="每日次数"
              :validation-status="rowErrors[index]?.daily ? 'error' : undefined"
              :feedback="dailyHint(row, index)"
            >
              <NInputNumber
                v-model:value="row.dailyLimit"
                :min="row.kind === 2 ? 0 : 1"
                :max="maxDailyLimit"
                :precision="0"
                :show-button="false"
                :disabled="disabled"
                :input-props="{ 'aria-label': `${kindLabel(row.kind)}每日次数` }"
                class="w-full"
              >
                <template #suffix>次</template>
              </NInputNumber>
            </NFormItem>
            <NFormItem
              label="保底间隔"
              :validation-status="rowErrors[index]?.guarantee ? 'error' : undefined"
              :feedback="guaranteeHint(index)"
            >
              <NInputNumber
                v-model:value="row.guaranteeEvery"
                :min="1"
                :max="maxGuaranteeEvery"
                :precision="0"
                :show-button="false"
                :disabled="disabled"
                :input-props="{ 'aria-label': `${kindLabel(row.kind)}保底间隔` }"
                class="w-full"
              >
                <template #suffix>次</template>
              </NInputNumber>
            </NFormItem>
          </div>
        </div>
      </NForm>

      <p class="recruit-consequence ops-muted">保存后仅影响后续招募，已累计次数与保底进度保留。</p>

      <div class="recruit-footer">
        <label v-if="canWrite" class="recruit-reason">
          <span class="ops-muted">操作原因</span>
          <NInput v-model:value="reason" :disabled="disabled" maxlength="100" />
        </label>
        <div class="recruit-actions">
          <NTag v-if="dirty && canWrite" type="warning" :bordered="false">未保存</NTag>
          <NTag v-if="!canWrite" :bordered="false">只读</NTag>
          <NButton :loading="loading" :disabled="saving" @click="refresh">刷新</NButton>
          <NButton
            v-if="canWrite"
            type="primary"
            :loading="saving"
            :disabled="disabled || !dirty || invalid"
            @click="save"
          >
            保存配置
          </NButton>
        </div>
      </div>
    </NCard>
    <NSpin v-else-if="loading" class="recruit-loading" />
    <NEmpty v-else description="招募配置尚未载入">
      <template #extra>
        <NButton :loading="loading" @click="refresh">重新载入</NButton>
      </template>
    </NEmpty>
  </div>
</template>

<style scoped>
.recruit-settings {
  max-width: 1080px;
}
.recruit-rules {
  display: flex;
  flex-direction: column;
}
.recruit-rule {
  display: grid;
  grid-template-columns: minmax(160px, 0.7fr) minmax(0, 1fr) minmax(0, 1fr);
  gap: 20px;
  align-items: start;
  padding: 16px 0;
  border-bottom: 1px solid rgb(var(--base-text-color) / 0.08);
}
.recruit-rule:first-child {
  padding-top: 0;
}
.recruit-rule:last-child {
  padding-bottom: 4px;
  border-bottom: 0;
}
.recruit-rule-title {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding-top: 2px;
  font-size: 14px;
  line-height: 1.5;
}
.recruit-rule-title strong {
  font-weight: 600;
}
.recruit-rule :deep(.n-form-item) {
  margin-bottom: 0;
}
.recruit-consequence {
  margin: 14px 0 0;
}
.recruit-footer {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 16px;
  margin-top: 12px;
  padding-top: 16px;
  border-top: 1px solid rgb(var(--base-text-color) / 0.08);
}
.recruit-reason {
  display: flex;
  flex: 1 1 260px;
  align-items: center;
  gap: 8px;
  min-width: 0;
  max-width: 380px;
}
.recruit-reason > .n-input {
  flex: 1;
  min-width: 0;
}
.recruit-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-left: auto;
}
.recruit-loading {
  display: block;
  margin: 48px auto;
}
@media (max-width: 760px) {
  .recruit-rule {
    grid-template-columns: 1fr;
    gap: 14px;
  }
  .recruit-rule-title {
    padding-top: 0;
  }
  .recruit-footer {
    align-items: stretch;
  }
  .recruit-reason {
    max-width: none;
  }
  .recruit-actions {
    justify-content: flex-start;
    margin-left: 0;
  }
}
</style>
