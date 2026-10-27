<script setup lang="ts">
import { ref } from 'vue';
import ActivityTemplateWorkspace from '@/components/operations/activity-template-workspace.vue';
import NativeActivityWorkspace from '@/components/operations/native-activity-workspace.vue';
import PersonalOpeningActivities from '@/components/operations/personal-opening-activities.vue';

const activeTab = ref('native');
const nativeWorkspace = ref<InstanceType<typeof NativeActivityWorkspace> | null>(null);
const personalWorkspace = ref<InstanceType<typeof PersonalOpeningActivities> | null>(null);
const templateWorkspace = ref<InstanceType<typeof ActivityTemplateWorkspace> | null>(null);

async function beforeTabLeave(_name: string | number, oldName: string | number | null) {
  if (oldName === 'native') return nativeWorkspace.value?.beforeLeave() ?? true;
  if (oldName === 'personal') return personalWorkspace.value?.beforeLeave() ?? true;
  if (oldName === 'template') return templateWorkspace.value?.beforeTabLeave() ?? true;
  return true;
}
</script>

<template>
  <NTabs v-model:value="activeTab" type="line" class="activity-settings-tabs" :theme-overrides="{ tabGapMediumLine: '20px' }" :on-before-leave="beforeTabLeave">
    <NTabPane name="native" tab="常驻与精彩活动"><NativeActivityWorkspace ref="nativeWorkspace" /></NTabPane>
    <NTabPane name="personal" tab="个人开服活动" display-directive="show:lazy"><PersonalOpeningActivities ref="personalWorkspace" /></NTabPane>
    <NTabPane name="template" tab="排期模板" display-directive="show:lazy"><ActivityTemplateWorkspace ref="templateWorkspace" /></NTabPane>
  </NTabs>
</template>

<style scoped>
.activity-settings-tabs { width: 100%; max-width: 1600px; margin: 0 auto; }
</style>
