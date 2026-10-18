<script setup lang="ts">
import type { Item } from '@/service/operations';
defineProps<{ item?: Item; color?: Item['qualityColor']; label?: string }>();
</script>

<template>
  <span class="item-quality" :data-quality="color || item?.qualityColor || 'gray'">{{ label || item?.qualityLabel || '未标注' }}</span>
</template>

<style scoped>
.item-quality { --quality-ink: rgb(var(--base-text-color) / .65); display: inline-flex; align-items: center; padding: 3px 9px; line-height: 1.4; border-radius: 5px; font-size: 12px; font-weight: 500; color: var(--quality-ink); background: color-mix(in srgb, var(--quality-ink) 9%, transparent); white-space: nowrap; }
.item-quality[data-quality='white'] { --quality-ink: rgb(var(--base-text-color) / .9); }
.item-quality[data-quality='green'] { --quality-ink: #27834d; }
.item-quality[data-quality='blue'] { --quality-ink: #326fc0; }
.item-quality[data-quality='purple'] { --quality-ink: #8b51bd; }
.item-quality[data-quality='orange'] { --quality-ink: #b96b13; }
.item-quality[data-quality='red'] { --quality-ink: #c64551; }
:global(.dark) .item-quality[data-quality='green'] { --quality-ink: #70c894; }
:global(.dark) .item-quality[data-quality='blue'] { --quality-ink: #7caeee; }
:global(.dark) .item-quality[data-quality='purple'] { --quality-ink: #bd94e2; }
:global(.dark) .item-quality[data-quality='orange'] { --quality-ink: #e8ab64; }
:global(.dark) .item-quality[data-quality='red'] { --quality-ink: #ef8791; }
</style>
