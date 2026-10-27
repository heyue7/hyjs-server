<script setup lang="ts">
import type { ResourceBatchLine } from '@/service/resource-batch';
import { resourceAmountMax } from '@/service/resource-batch';
import ResourceImage from './resource-image.vue';
defineProps<{ lines: ResourceBatchLine[]; disabled?: boolean }>();
const emit = defineEmits<{ amount: [id: string, value: number | null]; remove: [id: string] }>();
</script>
<template>
  <div class="batch-lines">
    <div class="batch-heading"><strong>已选 {{ lines.length }} / 20 项</strong></div>
    <NEmpty v-if="!lines.length" description="请选择资源" size="small" />
    <div v-for="line in lines" :key="line.item.id" class="batch-line">
      <ResourceImage :item="line.item" :size="40" />
      <div class="batch-name"><strong>{{ line.item.name }}</strong><div class="ops-muted ops-mono">{{ line.item.id }}</div></div>
      <NInputNumber :value="line.amount" :min="1" :max="resourceAmountMax(line.item)" :precision="0" :disabled="disabled" :input-props="{ 'aria-label': `${line.item.name}数量` }" @update:value="emit('amount', line.item.id, $event)" />
      <NButton quaternary size="small" :disabled="disabled" :aria-label="`移除${line.item.name}`" @click="emit('remove', line.item.id)">移除</NButton>
    </div>
  </div>
</template>
<style scoped>
.batch-lines{margin:16px 0;container-type:inline-size}.batch-heading{margin-bottom:8px}.batch-line{display:grid;grid-template-columns:40px minmax(0,1fr) 150px auto;gap:10px;align-items:center;padding:10px 0;border-bottom:1px solid rgb(var(--base-text-color)/.08)}.batch-name{min-width:0;overflow-wrap:anywhere}.batch-line :deep(.resource-thumb),.batch-line :deep(.resource-thumb-placeholder){flex:none;width:40px!important;max-width:40px;background:transparent}
@media(max-width:480px){.batch-line{grid-template-columns:40px minmax(0,1fr) auto}.batch-line :deep(.n-input-number){grid-column:2;grid-row:2;width:100%}.batch-line :deep(.n-button){grid-column:3;grid-row:1 / 3}}
@container(max-width:420px){.batch-line{grid-template-columns:40px minmax(0,1fr) auto}.batch-line :deep(.n-input-number){grid-column:2;grid-row:2;width:100%}.batch-line :deep(.n-button){grid-column:3;grid-row:1 / 3}}
</style>
