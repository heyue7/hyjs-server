<script setup lang="ts">
import { $t } from '@/locales';

defineOptions({
  name: 'GlobalLogo'
});

interface Props {
  /** Whether to show the title */
  showTitle?: boolean;
}

withDefaults(defineProps<Props>(), {
  showTitle: true
});
</script>

<template>
  <RouterLink to="/" class="ops-brand w-full flex-center nowrap-hidden">
    <SystemLogo class="ops-brand-symbol text-28px" />
    <h2 v-show="showTitle" class="pl-10px text-16px font-semibold">
      {{ $t('system.title') }}
    </h2>
  </RouterLink>
</template>

<style scoped></style>
