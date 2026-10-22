<script setup lang="ts">
import type { FieldGGOConfiguration, FieldGGONPC } from '@/service/fields';
const value = defineModel<FieldGGOConfiguration>('value', { required: true });
defineProps<{ disabled: boolean }>();
const promptLabels: Record<keyof FieldGGOConfiguration['extraction'], string> = {
  arrival: '抵达接应点',
  confirmation: '确认撤离',
  confirmButton: '确认按钮',
  continueButton: '继续按钮',
  countdown: '撤离倒计时',
  cancelled: '离开接应范围'
};
function togglePosition(npc: FieldGGONPC, enabled: boolean) {
  npc.position = enabled ? { x: null, y: null, z: null } : null;
}
</script>

<template>
  <div class="ggo-editor space-y-4">
    <div class="flex flex-wrap items-center gap-2">
      <NTag type="info" size="small">配置准备</NTag>
      <span class="text-sm text-gray-500">尚未放置场景；复活时间为可编辑初值。</span>
    </div>
    <NForm label-placement="top" :disabled="disabled">
      <div class="ggo-grid">
        <NFormItem label="地图标题"><NInput v-model:value="value.title" :maxlength="80" /></NFormItem>
        <NFormItem label="玩法名称"><NInput v-model:value="value.modeName" :maxlength="80" /></NFormItem>
      </div>
    </NForm>
    <NDescriptions :column="1" label-placement="left" size="small">
      <NDescriptionsItem label="结算货币 / 商店">{{ value.currencyId }} / {{ value.shopKey }}</NDescriptionsItem>
      <NDescriptionsItem label="携带 / 小芽标识">{{ value.carryIcon }} / {{ value.sproutIcon }}</NDescriptionsItem>
    </NDescriptions>
    <h3 class="font-semibold">NPC 与接应点</h3>
    <NCollapse>
      <NCollapseItem v-for="npc in value.npcs" :key="npc.id" :name="npc.id">
        <template #header>
          <div class="flex flex-wrap items-center gap-2">
            <strong>{{ npc.name }}</strong>
            <span class="text-sm text-gray-500">{{ npc.pointLabel }}</span>
            <NTag size="small">{{ npc.settlesChips ? '撤离 + 芯片结算' : '入口向导' }}</NTag>
            <span class="text-xs text-gray-500">{{ !npc.position ? '坐标未设置' : [npc.position.x, npc.position.y, npc.position.z].every(v => v !== null && Number.isFinite(v)) ? '已填写坐标' : '坐标待补齐' }}</span>
          </div>
        </template>
        <NForm label-placement="top" :disabled="disabled">
          <div class="ggo-grid">
            <NFormItem label="显示名称"><NInput v-model:value="npc.name" :maxlength="80" /></NFormItem>
            <NFormItem label="拟定地点"><NInput v-model:value="npc.pointLabel" :maxlength="120" /></NFormItem>
          </div>
          <NFormItem label="台词">
            <NInput
              v-model:value="npc.dialogue"
              type="textarea"
              :maxlength="1000"
              :autosize="{ minRows: 2, maxRows: 5 }"
            />
          </NFormItem>
          <div class="mb-3 flex flex-wrap items-center gap-3">
            <span class="text-xs text-gray-500">模型 {{ npc.heroId }}</span>
            <NSwitch
              :aria-label="`${npc.name}填写坐标`"
              :value="Boolean(npc.position)"
              :disabled="disabled"
              @update:value="togglePosition(npc, $event)"
            />
            <span>填写坐标</span>
          </div>
          <div v-if="npc.position" class="ggo-coordinates">
            <NFormItem v-for="axis in ['x', 'y', 'z'] as const" :key="axis" :label="axis.toUpperCase()">
              <NInputNumber v-model:value="npc.position[axis]" placeholder="必填" :disabled="disabled" />
            </NFormItem>
          </div>
        </NForm>
      </NCollapseItem>
    </NCollapse>
    <template
      v-for="group in [
        { title: '小怪', rows: value.monsters },
        { title: 'Boss', rows: value.bosses }
      ]"
      :key="group.title"
    >
      <h3 class="font-semibold">{{ group.title }}</h3>
      <NCollapse>
        <NCollapseItem v-for="enemy in group.rows" :key="enemy.id" :name="enemy.id">
          <template #header>
            <div class="flex flex-wrap items-center gap-2">
              <strong>{{ enemy.name }}</strong>
              <span class="text-sm text-gray-500">{{ enemy.respawnMs / 1000 }} 秒复活</span>
            </div>
          </template>
          <div class="ggo-grid">
            <label>
              显示名称
              <NInput v-model:value="enemy.name" class="mt-1" :maxlength="80" :disabled="disabled" />
            </label>
            <label>
              复活时间（秒）
              <NInputNumber
                :value="enemy.respawnMs / 1000"
                class="mt-1"
                :min="1"
                :max="604800"
                :precision="3"
                :disabled="disabled"
                @update:value="enemy.respawnMs = Math.round(($event ?? 0) * 1000)"
              />
            </label>
          </div>
          <p class="mt-2 text-xs text-gray-500">模型 {{ enemy.heroId }}</p>
        </NCollapseItem>
      </NCollapse>
    </template>
    <h3 class="font-semibold">芯片兑换值</h3>
    <div class="ggo-grid">
      <label v-for="chip in value.chips" :key="chip.itemId">
        {{ chip.name }}
        <NInputNumber
          v-model:value="chip.exchangeValue"
          class="mt-1"
          :min="0"
          :max="2147483647"
          :precision="0"
          placeholder="待定"
          clearable
          :disabled="disabled"
        />
        <span class="text-xs text-gray-500">
          {{ chip.itemId }} · {{ chip.exchangeValue === null ? '待定' : '已配置' }}
        </span>
      </label>
    </div>
    <NCollapse>
      <NCollapseItem title="撤离提示" name="extraction">
        <NForm label-placement="top" :disabled="disabled">
          <NFormItem v-for="(label, key) in promptLabels" :key="key" :label="label">
            <NInput v-model:value="value.extraction[key]" :maxlength="500" />
          </NFormItem>
        </NForm>
      </NCollapseItem>
    </NCollapse>
  </div>
</template>

<style scoped>
.ggo-editor {
  min-width: 0;
}
.ggo-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 16px;
}
.ggo-coordinates {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}
@media (max-width: 600px) {
  .ggo-grid,
  .ggo-coordinates {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
