<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';

const props = withDefaults(defineProps<{
  value: string;
  disabled?: boolean;
  labels?: string[];
  min: number;
  max: number;
  integer?: boolean;
  total?: number;
  maxRows?: number;
  label?: string;
}>(), { maxRows: 64, label: '数值' });
const emit = defineEmits<{
  'update:value': [value: string];
  valid: [value: boolean];
}>();
interface Row { key: number; value: number | null }
const rows = ref<Row[]>([]);
const parseError = ref('');
const fixed = computed(() => props.labels !== undefined);
const limit = computed(() => Math.max(1, props.maxRows));
let nextKey = 0;
let pendingEcho: string | undefined;
function rowLabel(index: number) { return props.labels?.[index] ?? `第 ${index + 1} 次`; }
function valueError(value: number | null) {
  return value === null || !Number.isFinite(value) || value < props.min || value > props.max ||
    (props.integer && !Number.isSafeInteger(value));
}
const rowError = computed(() => {
  if (!rows.value.length) return `请至少配置一项${props.label}。`;
  if (fixed.value && rows.value.length !== props.labels!.length) return `须配置全部 ${props.labels!.length} 组${props.label}。`;
  if (rows.value.length > limit.value) return `最多配置 ${limit.value} 项${props.label}。`;
  const index = rows.value.findIndex(row => valueError(row.value));
  if (index >= 0) return `${rowLabel(index)}须填写 ${props.min} 至 ${props.max} 的${props.integer ? '整数' : '数值'}。`;
  if (props.total !== undefined) {
    const sum = rows.value.reduce((result, row) => result + row.value!, 0);
    // Match the server's 0.001 tolerance, allowing only floating-point roundoff at the boundary.
    const roundoff = Number.EPSILON * Math.max(1, Math.abs(sum), Math.abs(props.total)) * rows.value.length;
    if (Math.abs(sum - props.total) > 0.001 + roundoff) return `${props.label}合计须为 ${props.total}，当前为 ${Number(sum.toFixed(6))}（允许误差 0.001）。`;
  }
  return '';
});
const error = computed(() => parseError.value || rowError.value);
watch(error, value => emit('valid', !value), { immediate: true, flush: 'sync' });
function parse(value: string) {
  parseError.value = '';
  const parts = value.split(',');
  if (value === '' || parts.length > limit.value || (fixed.value && parts.length !== props.labels!.length)) {
    parseError.value = `原配置${value === '' ? '为空' : '项数不符合要求'}，原值已保留；重新配置会替换原内容。`;
    rows.value = [];
    return;
  }
  const parsed: Row[] = [];
  for (let index = 0; index < parts.length; index += 1) {
    const numericPattern = props.integer ? /^[+-]?[0-9]+$/ : /^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)$/;
    const number = numericPattern.test(parts[index]) ? Number(parts[index]) : NaN;
    if (valueError(number)) {
      parseError.value = `${rowLabel(index)}原配置无效，须为 ${props.min} 至 ${props.max} 的${props.integer ? '整数' : '数值'}。原值已保留。`;
      rows.value = [];
      return;
    }
    parsed.push({ key: nextKey++, value: number });
  }
  rows.value = parsed;
}
watch(() => props.value, value => {
  if (value === pendingEcho) { pendingEcho = undefined; return; }
  pendingEcho = undefined;
  parse(value);
}, { immediate: true });
function publish() {
  emit('valid', !error.value);
  const value = rows.value.map(row => row.value === null ? '' : String(row.value)).join(',');
  pendingEcho = value;
  emit('update:value', value);
  nextTick(() => { if (pendingEcho === value) pendingEcho = undefined; });
}
function update(index: number, value: number | null) {
  if (props.disabled) return;
  rows.value[index].value = value;
  publish();
}
function add() {
  if (props.disabled || fixed.value || parseError.value || rows.value.length >= limit.value) return;
  rows.value.push({ key: nextKey++, value: null });
  publish();
}
function remove(index: number) {
  if (props.disabled || fixed.value || rows.value.length <= 1) return;
  rows.value.splice(index, 1);
  publish();
}
function reset() {
  if (props.disabled) return;
  parseError.value = '';
  rows.value = Array.from({ length: fixed.value ? props.labels!.length : 1 }, () => ({ key: nextKey++, value: null }));
  publish();
}
</script>

<template>
  <div class="number-sequence-editor" :aria-label="`${label}配置`">
    <p v-if="error" class="number-sequence-error" role="alert">{{ error }}</p>
    <template v-if="!parseError">
      <div v-for="(row, index) in rows" :key="row.key" class="number-sequence-row">
        <span class="number-sequence-label">{{ rowLabel(index) }}</span>
        <NInputNumber :value="row.value" :disabled="disabled" :min="min" :max="max" :precision="integer ? 0 : undefined" :aria-label="`${rowLabel(index)}${label}`" :placeholder="`填写${label}`" @update:value="update(index, $event)">
          <template v-if="!fixed" #suffix>秒</template>
        </NInputNumber>
        <NButton v-if="!fixed" quaternary :disabled="disabled || rows.length <= 1" :aria-label="`移除${rowLabel(index)}${label}`" @click="remove(index)">移除</NButton>
      </div>
      <NButton v-if="!fixed && rows.length < limit" dashed size="small" :disabled="disabled" @click="add">添加次数</NButton>
    </template>
    <NPopconfirm v-else :disabled="disabled" positive-text="重新配置" negative-text="保留原值" @positive-click="reset">
      <template #trigger><NButton size="small" :disabled="disabled">重新配置{{ label }}</NButton></template>
      重新配置将替换原内容，所有数值需重新填写，是否继续？
    </NPopconfirm>
  </div>
</template>

<style scoped>
.number-sequence-editor { display: grid; gap: 10px; min-width: 0; width: 100%; }
.number-sequence-row { display: grid; grid-template-columns: 70px minmax(120px, 1fr) auto; gap: 10px; align-items: center; }
.number-sequence-label { font-size: 13px; overflow-wrap: anywhere; }
.number-sequence-error { margin: 0; color: var(--n-feedback-text-color-error, #d03050); font-size: 13px; line-height: 1.6; }
.number-sequence-editor > .n-button { justify-self: start; }
@media (max-width: 480px) {
  .number-sequence-row { grid-template-columns: minmax(0, 1fr) auto; }
  .number-sequence-label { grid-column: 1 / -1; }
}
</style>
