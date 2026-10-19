<script setup lang="ts">
import { onMounted } from 'vue';
import { useRoute, useRouter } from 'vue-router';
const route = useRoute();
const router = useRouter();
// Keep existing wallet links while the user's wallet is managed from their own row.
onMounted(() => {
  const query = { ...route.query };
  if (query.playerId) query.wallet = query.playerId;
  delete query.playerId;
  router.replace({ name: 'players', query });
});
</script>
<template><NSpin class="p-6" /></template>
