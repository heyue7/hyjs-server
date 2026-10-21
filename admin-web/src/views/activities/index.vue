<script setup lang="ts">
import { provide, ref } from 'vue';
import ActivityTemplateWorkspace from '@/components/operations/activity-template-workspace.vue';
import NativeActivityWorkspace from '@/components/operations/native-activity-workspace.vue';
import RecruitSettings from '@/components/operations/recruit-settings.vue';
import DailySign from '@/views/daily-sign/index.vue';

const activeTab = ref('native');
const nativeWorkspace = ref<InstanceType<typeof NativeActivityWorkspace> | null>(null);
const templateWorkspace = ref<InstanceType<typeof ActivityTemplateWorkspace> | null>(null);
const recruitWorkspace = ref<InstanceType<typeof RecruitSettings> | null>(null);

async function beforeTabLeave(_name: string | number, oldName: string | number | null) {
  if (oldName === 'native') return nativeWorkspace.value?.beforeLeave() ?? true;
  if (oldName === 'template') return templateWorkspace.value?.beforeTabLeave() ?? true;
  if (oldName === 'recruit') return recruitWorkspace.value?.canLeave() ?? true;
  return true;
}

async function openRecruitSettings() {
  if (activeTab.value === 'recruit') return;
  if (await beforeTabLeave('recruit', activeTab.value)) activeTab.value = 'recruit';
}

provide('hyjs:open-recruit-settings', openRecruitSettings);
</script>

<template>
  <NTabs v-model:value="activeTab" type="line" class="activity-settings-tabs" :theme-overrides="{ tabGapMediumLine: '20px' }" :on-before-leave="beforeTabLeave">
    <NTabPane name="native" tab="常驻与精彩活动"><NativeActivityWorkspace ref="nativeWorkspace" /></NTabPane>
    <NTabPane name="template" tab="排期模板" display-directive="show:lazy"><ActivityTemplateWorkspace ref="templateWorkspace" /></NTabPane>
    <NTabPane name="daily-sign" tab="每日签到" display-directive="show:lazy"><DailySign /></NTabPane>
    <NTabPane name="recruit" tab="招募设置"><RecruitSettings ref="recruitWorkspace" /></NTabPane>
  </NTabs>
</template>

<style scoped>
.activity-settings-tabs { padding: 0 20px; }
</style>
