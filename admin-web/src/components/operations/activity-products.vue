<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import type { ActivityProduct } from '@/service/activity-operations';
import { createRequestId } from '@/utils/request-id';
import RewardList from './reward-list.vue';
import ResourceSelect from './resource-select.vue';
const products = defineModel<ActivityProduct[]>({ required: true });
defineProps<{ disabled?: boolean; vipOptions: { label: string; value: string | number }[] }>();
const selectedId = ref('');
const productOptions = computed(() =>
  products.value.map((product, index) => ({
    value: product.id,
    label: `${index + 1}. ${product.name || '未命名项目'}`
  }))
);
const selectedProducts = computed(() => products.value.filter(product => product.id === selectedId.value));
watch(
  () => products.value.map(product => product.id).join(','),
  () => {
    if (!products.value.some(product => product.id === selectedId.value))
      selectedId.value = products.value[0]?.id || '';
  },
  { immediate: true }
);
const qualification = reactive<Record<string, boolean>>({});
function limited(product: ActivityProduct) {
  return qualification[product.id] ?? (product.minVip > 0 || product.minLevel > 1);
}
function toggleQualification(product: ActivityProduct, value: boolean) {
  qualification[product.id] = value;
  if (!value) {
    product.minVip = 0;
    product.minLevel = 1;
  }
}
function add() {
  const id = createRequestId();
  selectedId.value = id;
  products.value = [
    ...products.value,
    {
      id,
      name: '',
      rewards: [],
      cost: { resourceId: 'diamond', amount: 1 },
      dailyLimit: 1,
      totalLimit: 0,
      stock: -1,
      minLevel: 1,
      minVip: 0,
      discountBps: 10000,
      weight: 1,
      index: products.value.length + 1
    }
  ];
}
function remove(index: number) {
  products.value = products.value.filter((_, i) => i !== index);
}
const hasProducts = computed(() => products.value.length > 0);
</script>

<template>
  <div class="activity-products">
    <div class="ops-section-heading">
      <h3>商品与兑换项目</h3>
      <NButton :disabled="disabled" size="small" @click="add">添加项目</NButton>
    </div>
    <NEmpty v-if="!hasProducts" description="尚未配置项目" />
    <NSelect
      v-if="hasProducts"
      v-model:value="selectedId"
      :options="productOptions"
      filterable
      class="mb-20px"
      aria-label="选择商品项目"
    />
    <section v-for="product in selectedProducts" :key="product.id" class="activity-edit-section">
      <div class="ops-section-heading">
        <strong>项目 {{ products.findIndex(row => row.id === product.id) + 1 }}</strong>
        <NButton
          :disabled="disabled"
          quaternary
          size="small"
          @click="remove(products.findIndex(row => row.id === product.id))"
        >
          移除
        </NButton>
      </div>
      <NFormItem label="名称">
        <NInput v-model:value="product.name" :disabled="disabled" placeholder="在游戏中显示的名称" />
      </NFormItem>
      <NFormItem label="获得资源" required>
        <RewardList v-model:value="product.rewards" :disabled="disabled" />
      </NFormItem>
      <NFormItem label="消耗资源" required>
        <div class="resource-cost-line">
          <ResourceSelect
            v-model:value="product.cost.resourceId"
            :disabled="disabled"
            commerce-cost
            :recommended-ids="['diamond', 'coin']"
            placeholder="搜索可消耗的货币、材料或道具"
          />
          <NInputNumber
            v-model:value="product.cost.amount"
            :disabled="disabled"
            :min="1"
            :precision="0"
            placeholder="消耗数量（至少1）"
          />
        </div>
      </NFormItem>
      <NCollapse class="mb-20px">
        <NCollapseItem title="购买限制、资格与刷新" name="limits">
          <NGrid cols="1 s:2 m:3" responsive="screen" :x-gap="16">
            <NGi>
              <NFormItem label="每日限购">
                <NInputNumber
                  v-model:value="product.dailyLimit"
                  :disabled="disabled"
                  :min="1"
                  :precision="0"
                  class="w-full"
                />
              </NFormItem>
            </NGi>
            <NGi>
              <NFormItem label="累计限购（0 不限）">
                <NInputNumber
                  v-model:value="product.totalLimit"
                  :disabled="disabled"
                  :min="0"
                  :precision="0"
                  class="w-full"
                />
              </NFormItem>
            </NGi>
            <NGi>
              <NFormItem label="库存（-1 不限）">
                <NInputNumber
                  v-model:value="product.stock"
                  :disabled="disabled"
                  :min="-1"
                  :precision="0"
                  class="w-full"
                />
              </NFormItem>
            </NGi>
            <NGi>
              <NFormItem label="售价比例">
                <NInputNumber
                  :value="product.discountBps / 100"
                  :disabled="disabled"
                  :min="0.01"
                  :max="100"
                  :precision="2"
                  class="w-full"
                  @update:value="product.discountBps = Math.round(($event ?? 100) * 100)"
                >
                  <template #suffix>%</template>
                </NInputNumber>
              </NFormItem>
            </NGi>
            <NGi>
              <NFormItem label="刷新权重">
                <NInputNumber
                  v-model:value="product.weight"
                  :disabled="disabled"
                  :min="1"
                  :precision="0"
                  class="w-full"
                />
              </NFormItem>
            </NGi>
          </NGrid>
          <div class="qualification-line">
            <span>限制购买资格</span>
            <NSwitch
              :value="limited(product)"
              :disabled="disabled"
              @update:value="toggleQualification(product, $event)"
            />
          </div>
          <NGrid v-if="limited(product)" cols="1 s:2" responsive="screen" :x-gap="16">
            <NGi>
              <NFormItem label="最低角色等级">
                <NInputNumber
                  v-model:value="product.minLevel"
                  :disabled="disabled"
                  :min="1"
                  :precision="0"
                  class="w-full"
                />
              </NFormItem>
            </NGi>
            <NGi>
              <NFormItem label="最低 VIP 等级">
                <NSelect v-model:value="product.minVip" :disabled="disabled" :options="vipOptions" />
              </NFormItem>
            </NGi>
          </NGrid>
        </NCollapseItem>
      </NCollapse>
    </section>
  </div>
</template>
