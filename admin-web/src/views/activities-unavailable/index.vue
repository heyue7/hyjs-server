<script setup lang="ts">
import { onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';

const route = useRoute();
const router = useRouter();
// 旧书签沿用活动配置页的内部分类，避免另开全局菜单与重复工作区。
onMounted(() => router.replace({ path: '/activities', query: { ...route.query, availability: 'unavailable' } }));
</script>

<template><div class="p-6"><NSpin size="small" /></div></template>
