<script setup lang="ts">
import { computed } from 'vue';
import { resourceImage } from '@/service/operations';
import type { Item } from '@/service/operations';
const props = withDefaults(defineProps<{ item?: Item | null; size?: number; preview?: boolean; lazy?: boolean }>(), {
  item: null,
  size: 44,
  preview: true,
  lazy: true
});
const src = computed(() => resourceImage(props.item));
</script>

<template>
  <NImage
    v-if="src"
    :src="src"
    :preview-src="resourceImage(item, true)"
    :alt="item?.name || ''"
    :width="size"
    :height="size"
    object-fit="contain"
    :preview-disabled="!preview"
    :lazy="lazy"
    class="resource-thumb"
  >
    <template #error>
      <div class="resource-thumb-placeholder" :style="{ width: `${size}px`, height: `${size}px` }">
        <SvgIcon icon="ph:treasure-chest" />
      </div>
    </template>
  </NImage>
  <div
    v-else
    class="resource-thumb-placeholder"
    :style="{ width: `${size}px`, height: `${size}px` }"
    aria-label="暂无原图"
  >
    <SvgIcon icon="ph:treasure-chest" />
  </div>
</template>
