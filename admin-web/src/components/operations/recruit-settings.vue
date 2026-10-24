<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import type { DialogReactive } from 'naive-ui';
import { errorMessage } from '@/service/operations';
import { getRecruitSettings } from '@/service/recruit-settings';
import type { RecruitSetting, RecruitSettings, RecruitSettingsSaveRequest } from '@/service/recruit-settings';
import { useOperations } from '@/composables/useOperations';
import ContextHelp from './context-help.vue';

const props = withDefaults(defineProps<{ kind: 2 | 3; disabled?: boolean }>(), { disabled: false });

interface DraftRow {
  kind: 2 | 3;
  name: string;
  dailyLimit: number | null;
  guaranteeEvery: number | null;
  luckExchangeCost: number | null;
}

const { canWrite } = useOperations();
const snapshot = ref<RecruitSettings>();
const row = ref<DraftRow>();
const baseline = ref('');
const loading = ref(false);
const error = ref('');
const defaultReason = '调整招募规则';
const maxDailyLimit = 100000;
const maxGuaranteeEvery = 20;
let alive = true;
let loadGeneration = 0;

const kindName = computed(() => (props.kind === 2 ? '橙卡招募' : '王者招募'));
const helpText = computed(() =>
  props.kind === 2
    ? [
        '橙卡招募常驻开放，每日次数与王者招募分开计算。',
        '每日次数填 0 表示不限；保底间隔 N 表示连续未出高品质时最多 N 次保底；最后20%的抽数逐步提高概率。',
        '保存后对之后的所有抽取生效，已累计次数与保底进度保留。',
        '保底获得橙卡。保底间隔可设10或20抽，每日次数须为10的倍数。'
      ].join('\n')
    : [
        '所有王者招募活动通用，三个世界共用每日次数与保底进度，幸运值由原版及三个世界共用，活动结束后保留，下次开启继续累计。达到兑换门槛后继续累计，兑换扣除所需值并保留余额。',
        '免费招募也占 1 次；保底间隔 N 表示连续未出高品质时最多 N 次保底；最后20%的抽数逐步提高概率。',
        '保存后对之后的所有抽取生效，已累计次数与保底进度保留。',
        '保底可能获得橙卡或红卡，不保证红卡。保底间隔可设10或20抽，每日次数须为10的倍数。'
      ].join('\n')
);

function rowKey(value?: DraftRow) {
  if (!value) return '';
  return JSON.stringify({
    kind: value.kind,
    dailyLimit: value.dailyLimit,
    guaranteeEvery: value.guaranteeEvery,
    luckExchangeCost: value.luckExchangeCost
  });
}

const dirty = computed(() => Boolean(snapshot.value && row.value) && rowKey(row.value) !== baseline.value);
const disabled = computed(() => props.disabled || !canWrite.value || loading.value || Boolean(error.value));

function dailyLimitError(value: DraftRow) {
  const min = value.kind === 2 ? 0 : 10;
  if (value.dailyLimit === null || !Number.isInteger(value.dailyLimit))
    return `需为 ${min}–${maxDailyLimit.toLocaleString()} 的整数`;
  if (value.dailyLimit < min || value.dailyLimit > maxDailyLimit) return `范围 ${min}–${maxDailyLimit.toLocaleString()}`;
  if (value.dailyLimit % 10 !== 0) return '请填写10的倍数';
  return '';
}

function guaranteeError(value: DraftRow) {
  if (value.guaranteeEvery === null || !Number.isInteger(value.guaranteeEvery))
    return `需为 10–${maxGuaranteeEvery.toLocaleString()} 的整数`;
  if (value.guaranteeEvery < 10 || value.guaranteeEvery > maxGuaranteeEvery)
    return `范围 10–${maxGuaranteeEvery.toLocaleString()}`;
  if (value.guaranteeEvery % 10 !== 0) return '请填写10的倍数';
  return '';
}

const dailyError = computed(() => (row.value ? dailyLimitError(row.value) : ''));
const guaranteeErr = computed(() => (row.value ? guaranteeError(row.value) : ''));
const luckError = computed(() => props.kind === 3 && row.value &&
  (row.value.luckExchangeCost === null || !Number.isInteger(row.value.luckExchangeCost) ||
    row.value.luckExchangeCost < 1 || row.value.luckExchangeCost > 2147483647)
  ? '请填写1–2,147,483,647的整数' : '');
const invalid = computed(
  () =>
    !snapshot.value ||
    !row.value ||
    Boolean(error.value) ||
    Boolean(dailyError.value) ||
    Boolean(guaranteeErr.value) ||
    Boolean(luckError.value)
);

function apply(data: RecruitSettings) {
  const items = Array.isArray(data.items) ? [...data.items].sort((a, b) => a.kind - b.kind) : [];
  if (items.length !== 2 || items[0].kind !== 2 || items[1].kind !== 3) throw new Error('招募配置不完整，请刷新重试。');
  const current = items.find(item => item.kind === props.kind);
  if (!current) throw new Error('招募配置不完整，请刷新重试。');
  snapshot.value = data;
  row.value = {
    kind: current.kind,
    name: current.name,
    dailyLimit: current.dailyLimit,
    guaranteeEvery: current.guaranteeEvery,
    luckExchangeCost: current.luckExchangeCost
  };
  baseline.value = rowKey(row.value);
  error.value = '';
}

async function load() {
  if (loading.value) return;
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
    title: '放弃未保存的招募规则修改？',
    content: '返回后，本次招募规则修改不会保存。',
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

async function refresh() {
  if (loading.value) return;
  if (!(await confirmDiscard())) return;
  await load();
}

function request(): RecruitSettingsSaveRequest | undefined {
  if (!dirty.value || invalid.value || !snapshot.value || !row.value) return undefined;
  const current = row.value;
  const items: RecruitSetting[] = snapshot.value.items.map(item =>
    item.kind === props.kind
      ? {
          kind: item.kind,
          name: item.name,
          dailyLimit: current.dailyLimit as number,
          guaranteeEvery: current.guaranteeEvery as number,
          luckExchangeCost: current.luckExchangeCost as number
        }
      : { ...item }
  );
  return { version: snapshot.value.version, reason: defaultReason, items };
}

async function accept() {
  loadGeneration += 1;
  const generation = loadGeneration;
  loading.value = true;
  try {
    const data = await getRecruitSettings();
    if (!alive || generation !== loadGeneration) return;
    apply(data);
  } catch (cause) {
    if (alive && generation === loadGeneration)
      error.value = `招募规则最新版本载入失败：${errorMessage(cause)}。请重新载入后再编辑或保存。`;
  } finally {
    if (alive && generation === loadGeneration) loading.value = false;
  }
}

onMounted(load);
onBeforeUnmount(() => {
  alive = false;
  loadGeneration += 1;
  pendingDiscard?.dispose();
});

defineExpose({ dirty, loading, invalid, request, accept });
</script>

<template>
  <div class="recruit-rule-fields">
    <div class="recruit-rule-heading">
      <span class="recruit-rule-label">招募规则</span>
      <ContextHelp icon-only title="招募规则说明" :text="helpText" />
      <NButton text size="tiny" :loading="loading" class="recruit-rule-refresh" @click="refresh">刷新</NButton>
    </div>
    <div v-if="loading && !snapshot" class="recruit-rule-status">
      <NSpin size="small" />
      <span class="ops-muted">正在载入招募规则…</span>
    </div>
    <template v-else>
      <div v-if="error" class="recruit-rule-status recruit-rule-status-error">
        <NAlert type="error" :show-icon="false" class="recruit-rule-alert">{{ error }}</NAlert>
        <NButton size="tiny" :loading="loading" :disabled="loading" @click="refresh">重新载入</NButton>
      </div>
      <template v-if="snapshot && row">
        <NGrid cols="1 m:2" responsive="screen" :x-gap="20">
          <NGi>
            <NFormItem
              label="每日次数"
              :validation-status="dailyError ? 'error' : undefined"
              :feedback="dailyError || undefined"
            >
              <NInputNumber
                v-model:value="row.dailyLimit"
                :min="kind === 2 ? 0 : 10"
                :step="10"
                :max="maxDailyLimit"
                :precision="0"
                :show-button="false"
                :disabled="disabled"
                :input-props="{ 'aria-label': `${kindName}每日次数` }"
                class="w-full"
              >
                <template #suffix>次</template>
              </NInputNumber>
            </NFormItem>
          </NGi>
          <NGi>
            <NFormItem
              label="保底间隔"
              :validation-status="guaranteeErr ? 'error' : undefined"
              :feedback="guaranteeErr || undefined"
            >
              <NInputNumber
                v-model:value="row.guaranteeEvery"
                :min="10"
                :step="10"
                :max="maxGuaranteeEvery"
                :precision="0"
                :show-button="false"
                :disabled="disabled"
                :input-props="{ 'aria-label': `${kindName}保底间隔` }"
                class="w-full"
              >
                <template #suffix>次</template>
              </NInputNumber>
            </NFormItem>
          </NGi>
          <NGi v-if="kind === 3">
            <NFormItem label="红卡兑换所需幸运值" :validation-status="luckError ? 'error' : undefined" :feedback="luckError || undefined">
              <NInputNumber v-model:value="row.luckExchangeCost" :min="1" :max="2147483647" :precision="0" :show-button="false" :disabled="disabled" :input-props="{ 'aria-label': '红卡兑换所需幸运值' }" class="w-full" />
            </NFormItem>
          </NGi>
        </NGrid>
      </template>
      <div v-else-if="!loading && !error" class="recruit-rule-status">
        <span class="ops-muted">招募规则尚未载入</span>
        <NButton size="tiny" :loading="loading" @click="refresh">重新载入</NButton>
      </div>
    </template>
  </div>
</template>

<style scoped>
.recruit-rule-fields {
  min-width: 0;
}
.recruit-rule-heading {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 10px;
}
.recruit-rule-label {
  font-size: 13px;
  font-weight: 500;
  color: rgb(var(--base-text-color) / 0.55);
}
.recruit-rule-refresh {
  margin-left: auto;
  font-weight: 400;
  color: rgb(var(--base-text-color) / 0.55);
}
.recruit-rule-fields :deep(.n-form-item) {
  margin-bottom: 0;
}
.recruit-rule-status {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  min-height: 34px;
}
.recruit-rule-status-error {
  margin-bottom: 14px;
}
.recruit-rule-alert {
  flex: 1 1 260px;
  min-width: 0;
}
</style>
