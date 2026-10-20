<script setup lang="ts">
import ActivityTemplateWorkspace from '@/components/operations/activity-template-workspace.vue';
import NativeActivityWorkspace from '@/components/operations/native-activity-workspace.vue';
import DailySign from '@/views/daily-sign/index.vue';
</script>

<template>
  <NTabs type="line" class="activity-settings-tabs">
    <NTabPane name="templates" tab="活动模板"><ActivityTemplateWorkspace /></NTabPane>
    <NTabPane name="native" tab="固定活动配置" display-directive="show:lazy"><NativeActivityWorkspace /></NTabPane>
    <NTabPane name="daily-sign" tab="每日签到" display-directive="show:lazy"><DailySign /></NTabPane>
  </NTabs>
</template>

<style scoped>
.activity-settings-tabs { padding: 0 20px; }
</style>
