<script setup lang="ts">
import WalletReasons from '@/components/operations/wallet-reasons.vue';
</script>

<template>
  <div class="ops-page">
    <div class="ops-toolbar">
      <div>
        <div class="ops-page-label">钱包入账规则</div>
        <p class="ops-muted mt-8px">全局共享的入账理由、金额范围和权重。个人余额与账单在区服用户管理中查看。</p>
      </div>
    </div>
    <NCard :bordered="false" class="ops-panel"><WalletReasons /></NCard>
  </div>
</template>
