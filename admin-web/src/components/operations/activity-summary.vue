<script setup lang="ts">
import { computed } from 'vue';
import type { ActivityForm, ActivityMode } from '@/service/activity-operations';
import { formatTime } from '@/service/operations';
import RewardSummary from './reward-summary.vue';
const props = defineProps<{ form: ActivityForm; mode?: ActivityMode }>();
const options = computed(() => props.form.options || {});
const rows = computed(() => props.form.products || []);
const milestones = computed(() => props.form.milestones || []);
const prizes = computed(() => props.form.prizes || []);
const totalWeight = computed(() => prizes.value.reduce((sum, row) => sum + row.weight, 0));
const numberFields = [
  ['scorePerDraw', '每次抽取积分'], ['advancePlaces', '进阶奖名次范围'], ['initialDiamondPool', '初始大奖钻石池'],
  ['rebateCap', '返还钻石上限'], ['dayLimit', '每日探索次数'], ['totalLimit', '累计探索次数上限'], ['cooldownSeconds', '探索间隔（秒）']
];
const fixedLabels: Record<string, string> = { singleDiamond: '单抽钻石', tenDiamond: '十连钻石', coinPerDraw: '每次购买金币', dayLimit: '每日次数' };
function minuteText(value: number) { return value === 1440 ? '24:00' : `${String(Math.floor(value / 60)).padStart(2, '0')}:${String(value % 60).padStart(2, '0')}`; }
</script>

<template>
  <div class="activity-summary">
    <section class="activity-summary-section">
      <h3>活动信息</h3>
      <NDescriptions :column="2" label-placement="top">
        <NDescriptionsItem label="模式">{{ mode?.label || '暂未支持此模式' }}</NDescriptionsItem>
        <NDescriptionsItem label="游戏模板">{{ mode?.templates.find(template => template.id === form.templateId)?.name || '无可用模板' }}</NDescriptionsItem>
        <NDescriptionsItem label="开始">{{ formatTime(form.startAt) }}</NDescriptionsItem>
        <NDescriptionsItem label="结束">{{ formatTime(form.endAt) }}</NDescriptionsItem>
        <NDescriptionsItem label="领奖截止">{{ formatTime(form.claimEndAt) }}</NDescriptionsItem>
        <NDescriptionsItem label="运营备注">{{ form.source || '—' }}</NDescriptionsItem>
      </NDescriptions>
    </section>
    <section v-if="rows.length" class="activity-summary-section">
      <h3>商品与兑换项目 <span class="ops-muted">{{ rows.length }} 项</span></h3>
      <article v-for="(product, index) in rows" :key="product.id" class="activity-summary-entry">
        <h4>{{ product.name || `项目 ${index + 1}` }}</h4>
        <dl class="activity-summary-pairs">
          <dt>获得</dt><dd><RewardSummary :rewards="product.rewards" /></dd>
          <dt>消耗</dt><dd><RewardSummary :rewards="product.cost?.resourceId ? [product.cost] : []" empty-text="免费领取" /></dd>
          <dt>购买限制</dt><dd>每日 {{ product.dailyLimit }} 次 · 累计 {{ product.totalLimit || '不限' }} · 库存 {{ product.stock < 0 ? '不限' : product.stock }}<template v-if="product.discountBps !== 10000"> · 售价比例 {{ product.discountBps / 100 }}%</template></dd>
          <dt>购买资格</dt><dd>{{ product.minLevel > 1 ? `角色 ${product.minLevel} 级` : '不限等级' }} · {{ product.minVip > 0 ? `VIP${product.minVip}` : '不限VIP' }}</dd>
          <dt>排列与刷新</dt><dd>顺序 {{ product.index }} · 刷新权重 {{ product.weight }}</dd>
        </dl>
      </article>
    </section>
    <section v-if="milestones.length" class="activity-summary-section">
      <h3>{{ ['login-rewards', 'limited-sign'].includes(form.mode) ? '每日奖励' : '达成奖励' }}</h3>
      <article v-for="(row, index) in milestones" :key="index" class="activity-summary-entry">
        <h4>{{ ['login-rewards', 'limited-sign'].includes(form.mode) ? `第 ${row.goal} 天` : `目标 ${row.goal.toLocaleString()}` }}<NTag v-if="row.options" size="small" :bordered="false">单项选一</NTag></h4>
        <RewardSummary :rewards="row.options || row.rewards" />
      </article>
    </section>
    <section v-if="prizes.length" class="activity-summary-section">
      <h3>转盘奖位</h3>
      <div class="activity-summary-prizes">
        <article v-for="(row, index) in prizes" :key="row.id" class="activity-summary-entry">
          <h4>奖位 {{ index + 1 }}<NTag v-if="row.guaranteed" size="small" :bordered="false">保底候选</NTag></h4>
          <RewardSummary :rewards="row.resourceId ? [{ resourceId: row.resourceId, amount: row.amount }] : []" />
          <p class="ops-muted mt-12px">权重 {{ row.weight }}<template v-if="totalWeight"> · 占比 {{ (row.weight * 100 / totalWeight).toFixed(2) }}%</template> · 库存 {{ row.stock || '不限' }}</p>
        </article>
      </div>
      <p class="ops-muted mt-12px">权重占比不包含保底及超级大奖规则。</p>
    </section>
    <section v-if="Object.keys(options).length || mode?.fixedValues" class="activity-summary-section">
      <h3>规则与额外奖励</h3>
      <NDescriptions :column="2" label-placement="top">
        <NDescriptionsItem v-for="(value, key) in mode?.fixedValues || {}" :key="key" :label="fixedLabels[String(key)] || String(key)">{{ value }}</NDescriptionsItem>
        <template v-for="field in numberFields" :key="field[0]">
          <NDescriptionsItem v-if="options[field[0]] !== undefined" :label="field[1]">{{ Number(options[field[0]]).toLocaleString() }}</NDescriptionsItem>
        </template>
        <NDescriptionsItem v-if="options.rebateBps !== undefined" label="返钻比例">{{ options.rebateBps / 100 }}%</NDescriptionsItem>
        <NDescriptionsItem v-if="options.jackpotPerMillion !== undefined" label="超级大奖概率">{{ options.jackpotPerMillion / 10000 }}%</NDescriptionsItem>
        <NDescriptionsItem v-if="options.poolContributionBps !== undefined" label="消费计入大奖池比例">{{ options.poolContributionBps / 100 }}%</NDescriptionsItem>
        <NDescriptionsItem v-if="options.discounts" label="可用折扣">{{ options.discounts.map((value: number) => `${value / 100}%`).join('、') || '未配置' }}</NDescriptionsItem>
      </NDescriptions>
      <div v-for="(meal, index) in options.meals || []" :key="index" class="activity-summary-entry">{{ minuteText(meal.startMinute) }}–{{ minuteText(meal.endMinute) }} · 体力 {{ meal.amount }}</div>
      <div v-if="options.topPrize?.resourceId" class="activity-summary-entry"><h4>榜首奖励</h4><RewardSummary :rewards="[options.topPrize]" /></div>
      <div v-if="options.advancePrize?.resourceId" class="activity-summary-entry"><h4>进阶排名奖励</h4><RewardSummary :rewards="[options.advancePrize]" /></div>
      <div v-if="options.exploreRewards" class="activity-summary-entry"><h4>单次探索奖励</h4><RewardSummary :rewards="options.exploreRewards" /></div>
      <div v-if="options.finalRewards" class="activity-summary-entry"><h4>最终奖励</h4><RewardSummary :rewards="options.finalRewards" /></div>
      <div v-for="(prize, index) in options.coinPrizes || []" :key="index" class="activity-summary-entry">金币 {{ Number(prize.coin).toLocaleString() }} · 相对权重 {{ prize.weight }}</div>
    </section>
    <NEmpty v-if="!mode && !rows.length && !milestones.length && !prizes.length" description="该活动暂未提供可运营表单，原配置和参与历史保持。" />
  </div>
</template>

<style scoped>
.activity-summary-section + .activity-summary-section { margin-top: 32px; padding-top: 28px; border-top: 1px solid rgb(var(--base-text-color) / .09); }
.activity-summary h3 { margin: 0 0 22px; font-size: 17px; font-weight: 600; }
.activity-summary h4 { display: flex; align-items: center; gap: 10px; margin: 0 0 15px; font-size: 14px; font-weight: 600; }
.activity-summary h3 span { margin-left: 8px; font-size: 13px; font-weight: 400; }
.activity-summary-entry { padding: 18px 0; }
.activity-summary-entry + .activity-summary-entry { border-top: 1px solid rgb(var(--base-text-color) / .07); }
.activity-summary-pairs { display: grid; grid-template-columns: 88px minmax(0, 1fr); gap: 15px; align-items: center; margin: 0; }
.activity-summary-pairs dt { color: rgb(var(--base-text-color) / .55); }
.activity-summary-pairs dd { margin: 0; min-width: 0; }
.activity-summary-prizes { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 28px; }
@media (max-width: 640px) { .activity-summary-prizes { grid-template-columns: 1fr; } .activity-summary-pairs { grid-template-columns: 1fr; gap: 8px; } .activity-summary-pairs dd { margin-bottom: 8px; } }
</style>
