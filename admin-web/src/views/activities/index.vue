<script setup lang="ts">
import { ref } from 'vue';
import ActivityTemplateWorkspace from '@/components/operations/activity-template-workspace.vue';
import NativeActivityWorkspace from '@/components/operations/native-activity-workspace.vue';
import DailySign from '@/views/daily-sign/index.vue';

const activeTab = ref('native');
const nativeWorkspace = ref<InstanceType<typeof NativeActivityWorkspace> | null>(null);
const templateWorkspace = ref<InstanceType<typeof ActivityTemplateWorkspace> | null>(null);

async function beforeTabLeave(_name: string | number, oldName: string | number | null) {
  if (oldName === 'native') return nativeWorkspace.value?.beforeLeave() ?? true;
  if (oldName === 'template') return templateWorkspace.value?.beforeTabLeave() ?? true;
  return true;
}
</script>

<template>
  <NTabs v-model:value="activeTab" type="line" class="activity-settings-tabs" :on-before-leave="beforeTabLeave">
    <NTabPane name="native" tab="常驻与精彩活动"><NativeActivityWorkspace ref="nativeWorkspace" /></NTabPane>
    <NTabPane name="template" tab="排期模板" display-directive="show:lazy"><ActivityTemplateWorkspace ref="templateWorkspace" /></NTabPane>
    <NTabPane name="daily-sign" tab="每日签到" display-directive="show:lazy"><DailySign /></NTabPane>
  </NTabs>
</template>

<style scoped>
.activity-settings-tabs { padding: 0 20px; }
</style>
