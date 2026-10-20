<script setup lang="ts">
import { computed, watch } from 'vue';

const props = defineProps<{ value: string; disabled?: boolean }>();
const emit = defineEmits<{ 'update:value': [value: string]; valid: [valid: boolean] }>();
interface Choice { type?: string | null; value?: number | null; weight?: number | null }
const attributes = [
  { label: '攻击加成', value: 'peratt' },
  { label: '生命加成', value: 'perhp' },
  { label: '物防加成', value: 'perpdef' },
  { label: '魔防加成', value: 'permdef' }
];
const parsed = computed<{ rows: Choice[]; error: string }>(() => {
  let source: unknown;
  try { source = JSON.parse(props.value); } catch { return { rows: [], error: '祈祷奖池无法读取，原配置已保留。请联系维护人员处理。' }; }
  if (!Array.isArray(source)) return { rows: [], error: '祈祷奖池格式不正确，原配置已保留。' };
  for (let index = 0; index < source.length; index += 1) {
    const row = source[index];
    if (!row || typeof row !== 'object' || Array.isArray(row)) return { rows: [], error: `第 ${index + 1} 项格式不正确，原配置已保留。` };
    if (Object.keys(row).some(key => !['type', 'value', 'weight'].includes(key))) return { rows: [], error: `第 ${index + 1} 项包含当前编辑器不支持的内容，原配置已保留。请联系维护人员处理。` };
    if ((row.type != null && typeof row.type !== 'string') || (row.value != null && typeof row.value !== 'number') || (row.weight != null && typeof row.weight !== 'number')) return { rows: [], error: `第 ${index + 1} 项的数据格式不正确，原配置已保留。` };
  }
  return { rows: source as Choice[], error: '' };
});
const rows = computed(() => parsed.value.rows);
const valueValid = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value) && value > 0 && value <= 100;
const weightValid = (value: unknown): value is number => typeof value === 'number' && Number.isInteger(value) && value >= 1 && value <= 1000000;
const validation = computed(() => {
  if (parsed.value.error) return parsed.value.error;
  if (rows.value.length > 16) return '最多配置 16 项，请移除多余项目。';
  const index = rows.value.findIndex(row => !attributes.some(option => option.value === row.type) || !valueValid(row.value) || !weightValid(row.weight));
  return index < 0 ? '' : `请补全第 ${index + 1} 项：选择属性，加成须大于 0 且不超过 100%，权重须为 1 至 1,000,000 的整数。`;
});
watch(() => !validation.value, valid => emit('valid', valid), { immediate: true });
const totalWeight = computed(() => rows.value.every(row => weightValid(row.weight)) ? rows.value.reduce((sum, row) => sum + (row.weight ?? 0), 0) : 0);
function probability(row: Choice) {
  return totalWeight.value && weightValid(row.weight)
    ? new Intl.NumberFormat('zh-CN', { style: 'percent', maximumFractionDigits: 6 }).format(row.weight / totalWeight.value)
    : '—';
}
function update(index: number, key: keyof Choice, value: string | number | null) {
  if (props.disabled || parsed.value.error) return;
  const next = rows.value.map(row => ({ ...row }));
  next[index] = { ...next[index], [key]: value };
  emit('update:value', JSON.stringify(next));
}
function add() {
  if (props.disabled || parsed.value.error || rows.value.length >= 16) return;
  emit('update:value', JSON.stringify([...rows.value, { type: null, value: null, weight: null }]));
}
function remove(index: number) {
  if (props.disabled || parsed.value.error) return;
  emit('update:value', JSON.stringify(rows.value.filter((_, i) => i !== index)));
}
</script>

<template>
  <div class="prayer-editor">
    <NAlert v-if="parsed.error" type="error" :title="parsed.error" />
    <template v-else>
      <div class="rule-note">加成值按百分比填写，1 代表 1%。概率由各项权重计算。</div>
      <div v-if="!rows.length" class="rule-note">尚未配置祈祷奖励。</div>
      <div v-for="(row, index) in rows" :key="index" class="prayer-row">
        <NFormItem label="属性" :show-feedback="false">
          <NSelect :value="attributes.some(option => option.value === row.type) ? row.type : null" :options="attributes" :disabled="disabled" placeholder="选择属性" :input-props="{ 'aria-label': `祈祷第${index + 1}项属性` }" @update:value="update(index, 'type', typeof $event === 'string' ? $event : null)" />
        </NFormItem>
        <NFormItem label="加成（%）" :show-feedback="false">
          <NInputNumber :value="row.value ?? null" :min="0" :max="100" :disabled="disabled" placeholder="大于 0" :input-props="{ 'aria-label': `祈祷第${index + 1}项加成百分比` }" @update:value="update(index, 'value', $event)" />
        </NFormItem>
        <NFormItem label="权重" :show-feedback="false">
          <NInputNumber :value="row.weight ?? null" :min="1" :max="1000000" :precision="0" :disabled="disabled" placeholder="填写权重" :input-props="{ 'aria-label': `祈祷第${index + 1}项权重` }" @update:value="update(index, 'weight', $event)" />
        </NFormItem>
        <div class="prayer-probability"><span class="rule-note">概率</span><span>{{ probability(row) }}</span></div>
        <NButton quaternary :disabled="disabled" :aria-label="`移除祈祷第${index + 1}项`" @click="remove(index)">移除</NButton>
      </div>
      <NAlert v-if="validation" type="warning" :title="validation" />
      <NButton dashed :disabled="disabled || rows.length >= 16" @click="add">添加祈祷奖励</NButton>
    </template>
  </div>
</template>

<style scoped>
.prayer-editor { width: 100%; display: flex; flex-direction: column; gap: 12px; }
.rule-note { font-size: 13px; color: rgb(var(--base-text-color) / .65); }
.prayer-row { display: grid; grid-template-columns: minmax(130px, 1.2fr) minmax(100px, 1fr) minmax(110px, 1fr) minmax(85px, .7fr) auto; gap: 12px; align-items: end; padding-bottom: 12px; border-bottom: 1px solid rgb(var(--base-text-color) / .09); }
.prayer-row :deep(.n-input-number) { width: 100%; }
.prayer-probability { display: flex; flex-direction: column; gap: 8px; min-height: 58px; justify-content: end; font-variant-numeric: tabular-nums; }
@media (max-width: 760px) { .prayer-row { grid-template-columns: repeat(2, minmax(0, 1fr)); } .prayer-row > :first-child { grid-column: 1 / -1; } }
</style>
