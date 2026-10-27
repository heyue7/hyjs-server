<script lang="ts" setup>
import { computed } from 'vue';
import { useRouterPush } from '@/hooks/common/router';
import { $t } from '@/locales';

defineOptions({ name: 'ExceptionBase' });

type ExceptionType = '403' | '404' | '500';

interface Props {
  /**
   * Exception type
   *
   * - 403: no permission
   * - 404: not found
   * - 500: service error
   */
  type: ExceptionType;
}

const props = defineProps<Props>();

const { routerPushByKey } = useRouterPush();

const iconMap: Record<ExceptionType, string> = {
  '403': 'no-permission',
  '404': 'not-found',
  '500': 'service-error'
};

const icon = computed(() => iconMap[props.type]);
const title = computed(() => props.type === '403' ? $t('route.403') : props.type === '404' ? $t('route.404') : $t('route.500'));
</script>

<template>
  <div class="exception-page">
    <div class="exception-illustration text-primary" aria-hidden="true">
      <SvgIcon :local-icon="icon" />
    </div>
    <h1>{{ title }}</h1>
    <NButton type="primary" @click="routerPushByKey('root')">{{ $t('common.backToHome') }}</NButton>
  </div>
</template>

<style scoped>
.exception-page { box-sizing: border-box; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 16px; width: 100%; min-height: 100%; padding: 32px 16px; text-align: center; }
.exception-illustration { display: flex; font-size: clamp(180px, 55vw, 320px); }
.exception-page h1 { margin: 0; font-size: 20px; font-weight: 600; line-height: 1.5; }
</style>
