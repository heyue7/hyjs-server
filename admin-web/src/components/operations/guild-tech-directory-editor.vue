<script setup lang="ts">
import { computed, watch } from 'vue';

const props = defineProps<{ value: string; disabled?: boolean }>();
const emit = defineEmits<{ 'update:value': [value: string]; valid: [valid: boolean] }>();
type Building = 'heroStatue' | 'guardTree';
interface Directory { initialLevel?: number; heroStatue?: string[]; guardTree?: string[] }
const attributes = [
  { key: 'peratt', label: '攻击加成' },
  { key: 'perhp', label: '生命加成' },
  { key: 'perpdef', label: '物防加成' },
  { key: 'permdef', label: '魔防加成' },
  { key: 'cri', label: '暴击' },
  { key: 'hit', label: '命中' },
  { key: 'anticri', label: '抗暴' },
  { key: 'dodge', label: '闪避' }
];
const buildings = [{ value: 'heroStatue', label: '英雄雕像' }, { value: 'guardTree', label: '守护树' }];
const parsed = computed<{ directory: Directory; empty: boolean; error: string }>(() => {
  let source: unknown;
  try { source = JSON.parse(props.value); } catch { return { directory: {}, empty: false, error: '科技分配无法读取，原配置已保留。请联系维护人员处理。' }; }
  if (!source || typeof source !== 'object' || Array.isArray(source)) return { directory: {}, empty: false, error: '科技分配格式不正确，原配置已保留。' };
  const row = source as Record<string, unknown>;
  if (Object.keys(row).some(key => !['initialLevel', 'heroStatue', 'guardTree'].includes(key))) return { directory: {}, empty: false, error: '科技分配包含当前编辑器不支持的内容，原配置已保留。请联系维护人员处理。' };
  if ('initialLevel' in row && row.initialLevel !== 0) return { directory: {}, empty: false, error: '初始科技等级须为 0，原配置已保留。请联系维护人员处理。' };
  for (const building of ['heroStatue', 'guardTree']) {
    if (building in row && (!Array.isArray(row[building]) || (row[building] as unknown[]).some(item => typeof item !== 'string' || !attributes.some(attribute => attribute.key === item)))) return { directory: {}, empty: false, error: '科技分配包含无法识别的属性，原配置已保留。请联系维护人员处理。' };
  }
  return { directory: row as Directory, empty: Object.keys(row).length === 0, error: '' };
});
function assignments(key: string) {
  return [...(parsed.value.directory.heroStatue ?? []), ...(parsed.value.directory.guardTree ?? [])].filter(item => item === key).length;
}
function selected(key: string): Building | null {
  if (assignments(key) !== 1) return null;
  return parsed.value.directory.heroStatue?.includes(key) ? 'heroStatue' : 'guardTree';
}
const assignedCount = computed(() => attributes.filter(attribute => assignments(attribute.key) === 1).length);
const validation = computed(() => {
  if (parsed.value.error) return parsed.value.error;
  if (parsed.value.empty) return '';
  const repeated = attributes.find(attribute => assignments(attribute.key) > 1);
  if (repeated) return `${repeated.label}重复分配，请重新选择所属建筑。`;
  if (assignedCount.value !== 8) return `已分配 ${assignedCount.value} / 8 项，请为每项属性选择一座建筑。`;
  if (!parsed.value.directory.heroStatue?.length || !parsed.value.directory.guardTree?.length) return '英雄雕像和守护树都须至少分配一项属性。';
  if (parsed.value.directory.initialLevel !== 0) return '请重新选择一项建筑，确认初始等级为 0。';
  return '';
});
watch(() => !validation.value, valid => emit('valid', valid), { immediate: true });
function update(key: string, building: unknown) {
  if (building !== null && building !== 'heroStatue' && building !== 'guardTree') return;
  if (props.disabled || parsed.value.error) return;
  // A deliberate row edit records only its choice; other unassigned rows remain unassigned.
  const next: Directory = { ...parsed.value.directory, initialLevel: 0 };
  if (next.heroStatue) next.heroStatue = next.heroStatue.filter(item => item !== key);
  if (next.guardTree) next.guardTree = next.guardTree.filter(item => item !== key);
  if (building) next[building] = [...(next[building] ?? []), key];
  emit('update:value', JSON.stringify(next));
}
</script>

<template>
  <div class="tech-directory-editor">
    <NAlert v-if="parsed.error" type="error" :title="parsed.error" />
    <template v-else>
      <div class="tech-level"><span>初始科技等级</span><NInputNumber :value="0" disabled :show-button="false" :input-props="{ 'aria-label': '初始科技等级，固定为0', readonly: true }" /></div>
      <div class="rule-note">每项属性分配给一座建筑，两座建筑都须有属性。</div>
      <div v-if="parsed.empty" class="rule-note">尚未配置科技分配，请逐项选择所属建筑。</div>
      <div v-for="attribute in attributes" :key="attribute.key" class="tech-attribute-row">
        <span>{{ attribute.label }}</span>
        <NSelect :value="selected(attribute.key)" :options="buildings" :disabled="disabled" clearable placeholder="选择所属建筑" :input-props="{ 'aria-label': `${attribute.label}所属建筑` }" @update:value="update(attribute.key, $event)" />
      </div>
      <NAlert v-if="validation" type="warning" :title="validation" />
    </template>
  </div>
</template>

<style scoped>
.tech-directory-editor { width: 100%; display: flex; flex-direction: column; gap: 12px; }
.tech-level, .tech-attribute-row { display: grid; grid-template-columns: minmax(100px, 1fr) minmax(170px, 2fr); align-items: center; gap: 16px; }
.tech-level :deep(.n-input-number) { width: 100%; }
.tech-attribute-row { padding-bottom: 12px; border-bottom: 1px solid rgb(var(--base-text-color) / .09); }
.rule-note { font-size: 13px; color: rgb(var(--base-text-color) / .65); }
@media (max-width: 520px) { .tech-level, .tech-attribute-row { grid-template-columns: minmax(0, 1fr); gap: 8px; } }
</style>
