<script setup lang="ts">
import WalletReasons from '@/components/operations/wallet-reasons.vue';
</script>

<template>
  <div class="ops-page">
    <div class="ops-toolbar">
      <div>
        <div class="ops-page-label">钱包入账规则</div>
      </div>
    </div>
    <NCard :bordered="false" class="ops-panel"><WalletReasons /></NCard>
  </div>
</template>
