<script setup lang="ts">
import { computed } from 'vue';
import type { ActivityGameplayOptions, TurntablePityRule } from '@/service/activity-operations';
const options = defineModel<ActivityGameplayOptions>({ required: true });
const props = defineProps<{ prizeId: number; disabled?: boolean }>();
const rule = computed(() => options.value.prizePityRules?.find(row => row.prizeId === props.prizeId));
function preset(): TurntablePityRule { return { prizeId: props.prizeId, enabled: false, hardPity: 100, softStart: 90, basePPM: 500, softMaxPPM: 50000 }; }
function setRule(value: TurntablePityRule) {
  options.value.prizePityRules = [...(options.value.prizePityRules || []).filter(row => row.prizeId !== props.prizeId), value].sort((a,b) => a.prizeId-b.prizeId);
}
function toggle(value: boolean) { setRule({ ...(rule.value || preset()), enabled: value }); }
function change(key: keyof TurntablePityRule, value: number | null) { setRule({ ...(rule.value || preset()), [key]: value ?? 0 }); }
function probability(r: TurntablePityRule, n: number) {
  if (n >= r.hardPity) return 1;
  if (n < r.softStart) return r.basePPM / 1000000;
  return (r.basePPM + Math.floor((r.softMaxPPM-r.basePPM) * (n-r.softStart)**2 / (r.hardPity-1-r.softStart)**2)) / 1000000;
}
const preview = computed(() => {
  const r=rule.value;
  if (!r || r.hardPity<3 || r.hardPity>10000 || r.softStart<1 || r.softStart>r.hardPity-2 || r.basePPM<=0 || r.basePPM>r.softMaxPPM || r.softMaxPPM>=1000000) return null;
  let survival=1, expected=0;
  for(let n=1;n<=r.hardPity;n++) { expected+=survival; if(n<r.hardPity) survival*=1-probability(r,n); }
  return { expected: expected.toFixed(2), hard: (survival*100).toFixed(2), samples: [...new Set([1,r.softStart,r.hardPity-1,r.hardPity])].map(n=>`${n}抽 ${(probability(r,n)*100).toFixed(4)}%`).join(' · ') };
});
</script>
<template>
  <section class="pity-settings">
    <div class="qualification-line"><span>{{ prizeId === 9 ? '钻石池最终大奖递增与保底' : '本奖位概率递增与保底' }}</span><NSwitch :value="rule?.enabled || false" :disabled="disabled" @update:value="toggle" /></div>
    <template v-if="rule?.enabled">
      <NButton text size="small" :disabled="disabled" class="mb-12px" @click="setRule({ ...preset(), enabled: true })">应用后段集中预设</NButton>
      <NGrid cols="1 s:2" responsive="screen" :x-gap="16">
        <NGi><NFormItem label="前段概率（%）"><NInputNumber :value="rule.basePPM / 10000" :min="0.0001" :max="99.9999" :precision="4" :disabled="disabled" class="w-full" @update:value="change('basePPM', Math.round(($event || 0)*10000))" /></NFormItem></NGi>
        <NGi><NFormItem label="开始提升抽数"><NInputNumber :value="rule.softStart" :min="1" :max="rule.hardPity-2" :precision="0" :disabled="disabled" class="w-full" @update:value="change('softStart', $event)" /></NFormItem></NGi>
        <NGi><NFormItem label="保底前最高概率（%）"><NInputNumber :value="rule.softMaxPPM / 10000" :min="0.0001" :max="99.9999" :precision="4" :disabled="disabled" class="w-full" @update:value="change('softMaxPPM', Math.round(($event || 0)*10000))" /></NFormItem></NGi>
        <NGi><NFormItem label="最迟抽数"><NInputNumber :value="rule.hardPity" :min="3" :max="10000" :precision="0" :disabled="disabled" class="w-full" @update:value="change('hardPity', $event)" /></NFormItem></NGi>
      </NGrid>
      <div v-if="preview" class="ops-muted">单独此规则的理论值：平均 {{ preview.expected }} 抽，到保底 {{ preview.hard }}%。<br />{{ preview.samples }}<br />多个奖位临近保底时会提前依次安排，实际比例随其他奖位变化。</div>
      <NAlert v-else type="warning" :show-icon="false">最迟抽数须为3至10000，提升起点至少早于保底2抽；前段概率须大于0且不超过最高概率。</NAlert>
    </template>
  </section>
</template>
<style scoped>
.pity-settings { margin-top: 12px; padding-top: 12px; border-top: 1px solid rgb(var(--base-text-color) / .1); }
</style>
