<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { api, errorMessage } from '@/service/operations';
import { shopLabels } from '@/service/management-operations';
import type { ShopSnapshot } from '@/service/management-operations';
import { activityFixedLabels } from '@/service/activity-operations';
import type { ActivityGameplayOptions } from '@/service/activity-operations';
import type { ResourceAmount } from '@/service/operations';
import TurntableProbability from './turntable-probability.vue';
import RewardList from './reward-list.vue';
const options = defineModel<ActivityGameplayOptions>({ required: true });
const props = defineProps<{ mode: string; disabled?: boolean; fixedValues?: Record<string, string | number> }>();
const router = useRouter();
const shopOptions = Object.entries(shopLabels).map(([value, label]) => ({ value, label }));
const shopStatus = ref('');
const shopLoading = ref(false);
const rankRewards = computed<ResourceAmount[]>({
  get: () => options.value.rankRewards || [],
  set: value => { options.value.rankRewards = value; }
});
let shopLoad = 0;
watch(() => [props.mode, options.value.shopKey], async ([mode, key]) => {
  const request = ++shopLoad;
  shopStatus.value = '';
  if (mode !== 'shop-window' || !key) return;
  shopLoading.value = true;
  try {
    const shop = await api<ShopSnapshot>(`/shops/${encodeURIComponent(key)}`);
    if (request !== shopLoad) return;
    if (shop.settings.unlimited) shopStatus.value = '请先在商店管理关闭「不限时」。';
    else if (!shop.items.length || (!shop.items.some(item => item.enabled) && !(key === 'pet-market' && shop.settings.enabled)))
      shopStatus.value = '请先在商店管理保存可用商品目录并启用。';
  } catch (error) {
    if (request === shopLoad) shopStatus.value = errorMessage(error);
  } finally {
    if (request === shopLoad) shopLoading.value = false;
  }
}, { immediate: true });
watch(
  () => props.mode,
  mode => {
    if (mode === 'mystic-shop' && !Array.isArray(options.value.discounts)) options.value.discounts = [];
    if (mode === 'stamina-meal' && !Array.isArray(options.value.meals)) options.value.meals = [];
    if (mode === 'continued-spend' && !Array.isArray(options.value.finalRewards)) options.value.finalRewards = [];
    if (mode === 'treasure-box') {
      for (const key of ['exploreRewards', 'finalRewards', 'coinPrizes'])
        if (!Array.isArray(options.value[key])) options.value[key] = [];
    }
    if (mode === 'turntable') {
      for (const key of ['topPrize', 'advancePrize'])
        if (!options.value[key]) options.value[key] = { resourceId: '', amount: 0 };
    }
  },
  { immediate: true }
);
function minuteText(value: number) {
  if (value === 1440) return '00:00';
  return `${String(Math.floor((value || 0) / 60)).padStart(2, '0')}:${String((value || 0) % 60).padStart(2, '0')}`;
}
function parseMinute(value: string | null) {
  if (!value) return 0;
  const [hour, minute] = value.split(':').map(Number);
  return hour * 60 + minute;
}
function addMeal() {
  options.value.meals.push({ startMinute: 0, endMinute: 0, amount: 0 });
}
function addCoinPrize() {
  options.value.coinPrizes.push({ coin: 0, weight: 0 });
}
const turntableNumbers = [
  { key: 'scorePerDraw', label: '每次抽取积分' },
  { key: 'initialDiamondPool', label: '初始大奖钻石池' }
];
</script>

<template>
  <div>
    <NDescriptions
      v-if="fixedValues && Object.keys(fixedValues).length"
      :column="1"
      label-placement="top"
      class="mb-24px"
    >
      <NDescriptionsItem
        v-for="(value, key) in fixedValues"
        :key="key"
        :label="activityFixedLabels[String(key)] || '固定规则'"
      >
        {{ typeof value === 'number' ? value.toLocaleString() : value }}
      </NDescriptionsItem>
    </NDescriptions>
    <template v-if="mode === 'power-race'">
      <NFormItem label="获奖名次上限">
        <NInputNumber :value="options.rankPlaces ?? 0" :min="0" :max="1000" :precision="0" :disabled="disabled" class="w-full" @update:value="options.rankPlaces = $event ?? 0">
          <template #suffix>名</template>
        </NInputNumber>
      </NFormItem>
      <p class="ops-inline-note">0 不发排名奖；设置后，前 N 名每人领取下列相同奖励。</p>
      <NFormItem label="排名奖励">
        <RewardList v-model:value="rankRewards" :disabled="disabled || !options.rankPlaces" />
      </NFormItem>
      <p v-if="!options.rankPlaces && rankRewards.length" class="ops-inline-note">奖励草稿已保留，当前不发放。</p>
    </template>
    <template v-else-if="mode === 'shop-window'">
      <NFormItem label="对应商店" :validation-status="shopStatus ? 'warning' : undefined" :feedback="shopStatus || undefined">
        <NSelect v-model:value="options.shopKey" :options="shopOptions" filterable :disabled="disabled" :loading="shopLoading" placeholder="选择商店" />
      </NFormItem>
      <div class="ops-muted">商品、价格与购买资格取该商店的已保存目录。</div>
      <NButton v-if="options.shopKey" text type="primary" class="mt-12px" @click="router.push(`/shops/${options.shopKey}`)">前往商店管理</NButton>
    </template>
    <template v-else-if="mode === 'stamina-meal'">
      <div class="ops-section-heading">
        <h3>领取时段</h3>
        <NButton v-if="options.meals.length < 2" :disabled="disabled" size="small" @click="addMeal">添加时段</NButton>
      </div>
      <section v-for="(meal, index) in options.meals" :key="index" class="activity-edit-section">
        <div class="ops-section-heading">
          <strong>时段 {{ Number(index) + 1 }}</strong>
          <NButton quaternary size="small" :disabled="disabled" @click="options.meals.splice(index, 1)">移除</NButton>
        </div>
        <div class="ops-date-grid">
          <NFormItem label="开始时间">
            <NTimePicker
              :formatted-value="minuteText(meal.startMinute)"
              format="HH:mm"
              :disabled="disabled"
              @update:formatted-value="meal.startMinute = parseMinute($event)"
            />
          </NFormItem>
          <NFormItem label="结束时间">
            <NTimePicker
              :formatted-value="minuteText(meal.endMinute)"
              format="HH:mm"
              :disabled="disabled || meal.endMinute === 1440"
              @update:formatted-value="meal.endMinute = parseMinute($event)"
            />
            <NCheckbox
              :checked="meal.endMinute === 1440"
              :disabled="disabled"
              class="ml-10px"
              @update:checked="meal.endMinute = $event ? 1440 : 0"
            >
              当天结束
            </NCheckbox>
          </NFormItem>
        </div>
        <NFormItem label="体力数量">
          <NInputNumber v-model:value="meal.amount" :min="0" :precision="0" :disabled="disabled" class="w-full" />
        </NFormItem>
      </section>
      <div class="ops-muted">配置两个互不重叠的日内时段，时间采用北京时间。</div>
    </template>
    <template v-else-if="mode === 'diamond-rebate'">
      <NFormItem label="钻石返还比例">
        <NInputNumber
          :value="(options.rebateBps || 0) / 100"
          :min="0.01"
          :max="100"
          :precision="2"
          :disabled="disabled"
          class="w-full"
          @update:value="options.rebateBps = Math.round(($event || 0) * 100)"
        >
          <template #suffix>%</template>
        </NInputNumber>
      </NFormItem>
      <NFormItem label="返还钻石上限">
        <NInputNumber v-model:value="options.rebateCap" :min="0" :precision="0" :disabled="disabled" class="w-full" />
      </NFormItem>
    </template>
    <template v-else-if="mode === 'continued-spend'">
      <NFormItem label="每日消费钻石门槛" required>
        <NInputNumber v-model:value="options.dailySpend" :min="1" :max="2147483647" :precision="0" :disabled="disabled" class="w-full"><template #suffix>钻</template></NInputNumber>
      </NFormItem>
      <p class="ops-inline-note">每个北京时间自然日实际消费达到门槛计为 1 个达标日。</p>
      <NFormItem label="七档完成终奖" required><RewardList v-model:value="options.finalRewards" :disabled="disabled" /></NFormItem>
    </template>
    <template v-else-if="mode === 'treasure-box'">
      <NDescriptions :column="1" label-placement="left" class="mb-20px">
        <NDescriptionsItem label="每日探索次数">{{ options.dayLimit }}</NDescriptionsItem>
      </NDescriptions>
      <NGrid cols="1 s:2" responsive="screen" :x-gap="16">
        <NGi>
          <NFormItem label="累计探索次数上限">
            <NInputNumber
              v-model:value="options.totalLimit"
              :min="0"
              :precision="0"
              :disabled="disabled"
              class="w-full"
            />
          </NFormItem>
        </NGi>
        <NGi>
          <NFormItem label="探索间隔">
            <NInputNumber
              v-model:value="options.cooldownSeconds"
              :min="0"
              :precision="0"
              :disabled="disabled"
              class="w-full"
            >
              <template #suffix>秒</template>
            </NInputNumber>
          </NFormItem>
        </NGi>
      </NGrid>
      <NFormItem label="单次探索奖励">
        <RewardList v-model:value="options.exploreRewards" :disabled="disabled" />
      </NFormItem>
      <NFormItem label="最终奖励"><RewardList v-model:value="options.finalRewards" :disabled="disabled" /></NFormItem>
      <div class="ops-section-heading">
        <h3>金币奖项</h3>
        <NButton size="small" :disabled="disabled" @click="addCoinPrize">添加金币奖项</NButton>
      </div>
      <div v-for="(prize, index) in options.coinPrizes" :key="index" class="reward-list-line mb-12px">
        <NInputNumber v-model:value="prize.coin" :min="0" :precision="0" :disabled="disabled" placeholder="金币数量" />
        <NInputNumber
          v-model:value="prize.weight"
          :min="0"
          :precision="0"
          :disabled="disabled"
          placeholder="相对权重"
        />
        <NButton quaternary :disabled="disabled" @click="options.coinPrizes.splice(index, 1)">移除</NButton>
      </div>
    </template>
    <template v-else-if="mode === 'turntable'">
      <div class="ops-muted mb-16px">各开启奖位的最高概率，加上未开启递增的钻石大奖固定概率，合计须不超过100%。最迟抽数须不少于开启奖位数。钻石池为空暂停计数，返池后待保底奖位依次兑现。</div>
      <TurntableProbability v-model="options" :prize-id="9" :disabled="disabled" />
      <NGrid cols="1 s:2" responsive="screen" :x-gap="16">
        <NGi v-for="field in turntableNumbers" :key="field.key">
          <NFormItem :label="field.label">
            <NInputNumber
              v-model:value="options[field.key]"
              :min="0"
              :precision="0"
              :disabled="disabled"
              class="w-full"
            />
          </NFormItem>
        </NGi>
        <NGi>
          <NFormItem label="钻石池最终大奖固定概率（未开递增时）">
            <NInputNumber
              :value="(options.jackpotPerMillion || 0) / 10000"
              :min="0"
              :max="100"
              :precision="4"
              :disabled="disabled"
              class="w-full"
              @update:value="options.jackpotPerMillion = Math.round(($event || 0) * 10000)"
            >
              <template #suffix>%</template>
            </NInputNumber>
          </NFormItem>
        </NGi>
        <NGi>
          <NFormItem label="消费计入大奖池比例">
            <NInputNumber
              :value="(options.poolContributionBps || 0) / 100"
              :min="0"
              :max="100"
              :precision="2"
              :disabled="disabled"
              class="w-full"
              @update:value="options.poolContributionBps = Math.round(($event || 0) * 100)"
            >
              <template #suffix>%</template>
            </NInputNumber>
          </NFormItem>
        </NGi>
      </NGrid>
      <p class="ops-muted">库存、钻石大奖池和抽取记录按角色独立计算。</p>
    </template>
    <template v-if="Array.isArray(options.discounts)">
      <div class="ops-section-heading">
        <h3>可用折扣</h3>
        <NButton :disabled="disabled" size="small" @click="options.discounts.push(0)">添加折扣</NButton>
      </div>
      <div v-for="(_, index) in options.discounts" :key="index" class="resource-cost-line mb-12px">
        <NInputNumber
          :value="options.discounts[index] / 100"
          :min="0"
          :max="100"
          :precision="2"
          :disabled="disabled"
          @update:value="options.discounts[index] = Math.round(($event || 0) * 100)"
        >
          <template #suffix>%</template>
        </NInputNumber>
        <NButton quaternary :disabled="disabled" @click="options.discounts.splice(index, 1)">移除</NButton>
      </div>
    </template>
    <p
      v-if="!['shop-window', 'stamina-meal', 'diamond-rebate', 'treasure-box', 'turntable', 'power-race', 'king-recruit', 'continued-spend'].includes(mode) && !Array.isArray(options.discounts) && !Object.keys(fixedValues || {}).length"
      class="ops-inline-note"
    >该模式按奖励与资格中的配置执行，无需额外玩法参数。</p>
  </div>
</template>
