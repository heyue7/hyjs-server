<script setup lang="ts">
import type { CombatMember, FieldPresentation } from '@/service/fields';

withDefaults(defineProps<{
  members?: CombatMember[];
  presentation: Record<string, FieldPresentation>;
  readonly?: boolean;
  levelEditable?: boolean;
  levelHint?: string;
}>(), { levelEditable: true });

const numberFields = [
  { key: 'hp', label: '生命', min: 1, max: 1e12 },
  { key: 'attack', label: '攻击', min: 1, max: 1e12 },
  { key: 'phyDefend', label: '物防', min: 0, max: 1e12 },
  { key: 'magicDefend', label: '魔防', min: 0, max: 1e12 },
  { key: 'hit', label: '命中', min: 0, max: 10000 },
  { key: 'dodge', label: '闪避', min: 0, max: 10000 },
  { key: 'cri', label: '暴击', min: 0, max: 10000 },
  { key: 'antiCri', label: '抗暴', min: 0, max: 10000 },
  { key: 'criFactor', label: '暴击系数', min: 0, max: 10000 },
  { key: 'damageAdd', label: '伤害加成', min: 0, max: 10000 },
  { key: 'damageReduce', label: '伤害减免', min: 0, max: 10000 }
] as const;
</script>

<template>
  <div v-if="members?.length" class="combat-members">
    <div v-for="member in members" :key="member.id" class="combat-member">
      <div class="member-head">
        <NImage
          v-if="presentation[member.heroId]?.iconUrl"
          :src="presentation[member.heroId].iconUrl"
          width="34"
          height="34"
          object-fit="contain"
        />
        <div class="member-title">
          <strong>{{ presentation[member.heroId]?.name || member.heroId }}</strong>
          <span>阵位 {{ member.location }} · 第 {{ member.groupIndex }} 组</span>
        </div>
        <label class="member-level">
          <span>等级</span>
          <NInputNumber
            v-model:value="member.level"
            size="small"
            :min="1"
            :max="1000"
            :precision="0"
            :disabled="readonly || levelEditable === false"
            class="w-24"
          />
          <span v-if="levelHint" class="level-hint">{{ levelHint }}</span>
        </label>
      </div>
      <div class="member-grid">
        <label v-for="field in numberFields" :key="field.key">
          <span>{{ field.label }}</span>
          <NInputNumber
            v-model:value="member[field.key]"
            size="small"
            :min="field.min"
            :max="field.max"
            :disabled="readonly"
          />
        </label>
      </div>
    </div>
  </div>
</template>

<style scoped>
.combat-members {
  display: grid;
  gap: 10px;
}
.combat-member {
  padding: 10px 12px;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  background: #fafafa;
}
.member-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
.member-title {
  display: grid;
  gap: 1px;
  min-width: 140px;
}
.member-title span {
  color: #6b7280;
  font-size: 12px;
}
.member-level {
  display: flex;
  flex-wrap: wrap;
  max-width: 100%;
  align-items: center;
  gap: 6px;
  margin-left: auto;
  color: #4b5563;
  font-size: 12px;
}
.level-hint {
  max-width: 210px;
  color: #6b7280;
  line-height: 1.4;
}
.member-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(122px, 1fr));
  gap: 8px 10px;
}
.member-grid label {
  display: grid;
  gap: 3px;
  color: #4b5563;
  font-size: 12px;
}
:global(.dark) .combat-member {
  border-color: #374151;
  background: #1f2937;
}
:global(.dark) .member-title span,
:global(.dark) .member-grid label,
:global(.dark) .member-level,
:global(.dark) .level-hint {
  color: #9ca3af;
}
@media (max-width: 600px) { .member-level { margin-left: 0; } .member-grid { grid-template-columns: minmax(0, 1fr); } }
</style>
