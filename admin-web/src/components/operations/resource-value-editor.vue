<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import ResourceSelect from './resource-select.vue';

const props = withDefaults(defineProps<{
  value: string;
  disabled?: boolean;
  single?: boolean;
  label?: string;
}>(), { label: '奖励' });
const emit = defineEmits<{
  'update:value': [value: string];
  valid: [value: boolean];
}>();
interface Row { key: number; resourceId: string; amount: number | null }
const rows = ref<Row[]>([]);
const parseError = ref('');
const maxAmount = 2147483647;
const limit = computed(() => props.single ? 1 : 100);
let nextKey = 0;
let pendingEcho: string | undefined;
const resourcePattern = /^[^\s,;]+$/u;
const rowError = computed(() => {
  if (rows.value.length > limit.value) return `最多配置 ${limit.value} 项${props.label}，请移除多余项。`;
  if (props.single && !rows.value.length) return `请选择${props.label}资源并填写数量。`;
  const index = rows.value.findIndex(row => !resourcePattern.test(row.resourceId) ||
    row.amount === null || !Number.isSafeInteger(row.amount) || row.amount < 1 || row.amount > maxAmount);
  return index < 0 ? '' : `第 ${index + 1} 项请选择资源，数量须为 1 至 ${maxAmount} 的整数。`;
});
const error = computed(() => parseError.value || rowError.value);
watch(error, value => emit('valid', !value), { immediate: true, flush: 'sync' });

function parse(value: string) {
  parseError.value = '';
  if (value === '') { rows.value = []; return; }
  const parts = value.split(';');
  if (parts.length > limit.value) {
    parseError.value = `原配置超过 ${limit.value} 项${props.label}，原值已保留；重新配置会替换原内容。`;
    rows.value = [];
    return;
  }
  const parsed: Row[] = [];
  for (let index = 0; index < parts.length; index += 1) {
    const pair = parts[index].split(',');
    const amount = pair.length === 2 && /^[0-9]+$/.test(pair[1]) ? Number(pair[1]) : NaN;
    if (pair.length !== 2 || !resourcePattern.test(pair[0]) ||
      !Number.isSafeInteger(amount) || amount < 1 || amount > maxAmount) {
      parseError.value = `第 ${index + 1} 项原配置格式无法识别。原值已保留；重新配置会替换原内容。`;
      rows.value = [];
      return;
    }
    parsed.push({ key: nextKey++, resourceId: pair[0], amount });
  }
  rows.value = parsed;
}
watch(() => props.value, value => {
  // An unfinished user row (for example a newly added resource) belongs to this editor.
  // The parent's v-model echo must not parse it as an invalid imported configuration.
  if (value === pendingEcho) { pendingEcho = undefined; return; }
  pendingEcho = undefined;
  parse(value);
}, { immediate: true });
function publish() {
  emit('valid', !error.value);
  const value = rows.value.map(row => `${row.resourceId},${row.amount ?? ''}`).join(';');
  pendingEcho = value;
  emit('update:value', value);
  nextTick(() => { if (pendingEcho === value) pendingEcho = undefined; });
}
function update(index: number, key: 'resourceId' | 'amount', value: string | number | null) {
  if (props.disabled) return;
  const row = rows.value[index];
  if (key === 'resourceId') row.resourceId = String(value ?? '');
  else row.amount = typeof value === 'number' ? value : null;
  publish();
}
function add() {
  if (props.disabled || parseError.value || rows.value.length >= limit.value) return;
  rows.value.push({ key: nextKey++, resourceId: '', amount: 1 });
  publish();
}
function remove(index: number) {
  if (props.disabled) return;
  rows.value.splice(index, 1);
  publish();
}
function reset() {
  if (props.disabled) return;
  parseError.value = '';
  rows.value = [{ key: nextKey++, resourceId: '', amount: 1 }];
  publish();
}
</script>

<template>
  <div class="resource-value-editor" :class="{ 'resource-value-single': single }" :aria-label="`${label}配置`">
    <p v-if="error" class="resource-value-error" role="alert">{{ error }}</p>
    <template v-if="!parseError">
      <div v-for="(row, index) in rows" :key="row.key" class="resource-value-row">
        <div class="resource-value-resource">
          <ResourceSelect :value="row.resourceId" :disabled="disabled" :aria-label="`第 ${index + 1} 项${label}资源`" @update:value="update(index, 'resourceId', $event)" />
        </div>
        <NInputNumber :value="row.amount" :disabled="disabled" :min="1" :max="maxAmount" :precision="0" :input-props="{ 'aria-label': `第 ${index + 1} 项${label}数量` }" placeholder="数量" @update:value="update(index, 'amount', $event)" />
        <NButton v-if="!single" quaternary :disabled="disabled" :aria-label="`移除第 ${index + 1} 项${label}`" @click="remove(index)">移除</NButton>
      </div>
      <NButton v-if="rows.length < limit" dashed size="small" :disabled="disabled" @click="add">添加{{ label }}</NButton>
    </template>
    <NPopconfirm v-else :disabled="disabled" positive-text="重新配置" negative-text="保留原值" @positive-click="reset">
      <template #trigger><NButton size="small" :disabled="disabled">重新配置{{ label }}</NButton></template>
      重新配置将替换无法识别的原内容，是否继续？
    </NPopconfirm>
  </div>
</template>

<style scoped>
.resource-value-editor { display: grid; gap: 10px; min-width: 0; width: 100%; }
.resource-value-row { display: grid; grid-template-columns: minmax(0, 1fr) 150px auto; gap: 10px; align-items: start; }
.resource-value-resource { min-width: 0; }
.resource-value-single .resource-value-row { grid-template-columns: minmax(0, 1fr) 96px; }
.resource-value-error { margin: 0; color: var(--n-feedback-text-color-error, #d03050); font-size: 13px; line-height: 1.6; }
.resource-value-editor > .n-button { justify-self: start; }
@media (max-width: 640px) {
  .resource-value-row { grid-template-columns: minmax(0, 1fr) auto; }
  .resource-value-resource { grid-column: 1 / -1; }
}
</style>
