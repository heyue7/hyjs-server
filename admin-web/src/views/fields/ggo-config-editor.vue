<script setup lang="ts">
import { computed } from 'vue';
import type { FieldGGOConfiguration, FieldGGONPC } from '@/service/fields';

const value = defineModel<FieldGGOConfiguration>('value', { required: true });
defineProps<{ disabled: boolean }>();

const rules = computed(() => value.value.rules);
const currentRoles = new Set(['teleport', 'extraction', 'settlement']);
const legacyRole = computed(() => value.value.npcs.some(npc => !currentRoles.has(npc.role)));
const schemaVersionText = computed(() => (value.value.schemaVersion === 2 ? '配置准备' : '旧配置待更新'));

type TagType = 'default' | 'error' | 'info' | 'success' | 'warning';
type RoleInfo = { label: string; type: TagType; note: string };
const roleMap: Record<string, RoleInfo> = {
  teleport: { label: '随机传送', type: 'info', note: '通过 NPC 随机传送进入危险区。' },
  extraction: { label: '撤离接应', type: 'success', note: '只负责把玩家送回初始安全区，不处理芯片结算。' },
  settlement: { label: '安全区结算', type: 'warning', note: '在安全区接收临时终端中的芯片并结算信用点。' },
  guide: { label: '职责待迁移', type: 'default', note: '旧版入口向导职责需要后台迁移为随机传送。' }
};
function roleInfo(role: string): RoleInfo {
  return roleMap[role] ?? { label: '职责待确认', type: 'default', note: '当前职责不在已确认范围内。' };
}
function positionText(npc: FieldGGONPC) {
  if (!npc.position) return '未设置坐标';
  return [npc.position.x, npc.position.y, npc.position.z].every(v => v !== null && Number.isFinite(v))
    ? '坐标已保留'
    : '坐标待补齐';
}

type ChipQuality = { label: string; color: string };
const chipQualities: Record<string, ChipQuality> = {
  item_9701: { label: '白', color: '#9aa3ad' },
  item_9702: { label: '绿', color: '#8aa98f' },
  item_9703: { label: '蓝', color: '#8ba4c2' },
  item_9704: { label: '紫', color: '#a294b8' },
  item_9705: { label: '橙', color: '#c3a07d' },
  item_9706: { label: '红', color: '#bd8c8c' }
};
function qualityOf(chip: { itemId: string }, index: number): ChipQuality {
  return chipQualities[chip.itemId] ?? { label: `第 ${index + 1} 品质`, color: '#9ca3af' };
}
function chipMissing(chip: { dropWeight: number | null; dropAmount: number | null; exchangeValue: number | null }) {
  const missing: string[] = [];
  if (chip.dropWeight === null || chip.dropWeight === undefined) missing.push('掉落权重');
  if (chip.dropAmount === null || chip.dropAmount === undefined) missing.push('掉落数量');
  if (chip.exchangeValue === null || chip.exchangeValue === undefined) missing.push('兑换值');
  return missing;
}
const dropReadyCount = computed(
  () =>
    value.value.chips.filter(
      chip =>
        chip.dropWeight !== null &&
        chip.dropWeight !== undefined &&
        chip.dropAmount !== null &&
        chip.dropAmount !== undefined
    ).length
);
const exchangeReadyCount = computed(
  () => value.value.chips.filter(chip => chip.exchangeValue !== null && chip.exchangeValue !== undefined).length
);
const dropPoolIncomplete = computed(() => dropReadyCount.value < value.value.chips.length);
const exchangeIncomplete = computed(() => exchangeReadyCount.value < value.value.chips.length);

type GiftChoice = 'pending' | 'allow' | 'deny';
const giftOptions = [
  { label: '待定', value: 'pending' },
  { label: '允许赠送', value: 'allow' },
  { label: '不允许赠送', value: 'deny' }
];
const giftValue = computed<GiftChoice>({
  get() {
    const current = rules.value?.allowGift;
    if (current === null || current === undefined) return 'pending';
    return current ? 'allow' : 'deny';
  },
  set(next) {
    if (!rules.value) return;
    rules.value.allowGift = next === 'pending' ? null : next === 'allow';
  }
});

const pveLossOptions = [
  { label: '待定', value: 'pending' },
  { label: '直接销毁', value: 'destroy' }
];
const tradeAssetOptions = [
  { label: '待定（交易范围未确认）', value: 'pending' },
  { label: '仅芯片', value: 'chips_only' }
];
const weekdayOptions = [
  { label: '周一', value: 1 },
  { label: '周二', value: 2 },
  { label: '周三', value: 3 },
  { label: '周四', value: 4 },
  { label: '周五', value: 5 },
  { label: '周六', value: 6 },
  { label: '周日', value: 7 }
];
const weeklyServerOptions = [
  { label: '待定（跨服归属未确认）', value: 'pending' },
  { label: '仅当前区服（不跨服）', value: 'current_server' },
  { label: '全部区服共享', value: 'all_servers' }
];

function durationText(ms: number) {
  if (!Number.isFinite(ms) || ms <= 0) return '待定';
  if (ms % 60000 === 0) return `${ms / 60000} 分钟`;
  if (ms % 1000 === 0) return `${ms / 1000} 秒`;
  return `${ms} 毫秒`;
}
</script>

<template>
  <div class="ggo-editor">
    <div class="ggo-head">
      <div class="flex flex-wrap items-center gap-2">
        <NTag size="small" type="info">{{ schemaVersionText }}</NTag>
        <NTag size="small">草案模板</NTag>
        <NTag size="small">未接入客户端</NTag>
      </div>
      <p class="ggo-lede">保存后可继续调整，玩法生效时间待客户端接入。</p>
    </div>

    <NAlert v-if="!rules" type="warning" :bordered="false">
      这份方案还没有 v2 规则对象。后台迁移完成后可编辑规则区；现有名称、NPC、怪物与芯片字段仍按草案保存。
    </NAlert>

    <NAlert type="info" :bordered="false">
      当前准备搜打撤规则；未确定的参数可留空。地图点位和客户端玩法待后续接入。
    </NAlert>

    <section class="ggo-section">
      <header class="ggo-section-head">
        <div>
          <h3>方案信息</h3>
        </div>
      </header>
      <NForm label-placement="top" :disabled="disabled">
        <div class="ggo-field-grid">
          <NFormItem label="地图标题">
            <NInput v-model:value="value.title" :input-props="{ 'aria-label': '地图标题' }" :maxlength="80" />
          </NFormItem>
          <NFormItem label="玩法名称">
            <NInput v-model:value="value.modeName" :input-props="{ 'aria-label': '玩法名称' }" :maxlength="80" />
          </NFormItem>
        </div>
      </NForm>
      <div class="ggo-fixed-grid">
        <div class="ggo-fixed">
          <span>结算货币</span>
          <strong>信用点</strong>
        </div>
        <div class="ggo-fixed">
          <span>商店</span>
          <strong>信用商店</strong>
        </div>
        <div class="ggo-fixed">
          <span>携带标识</span>
          <strong>GGO 芯片标记</strong>
        </div>
      </div>
    </section>

    <template v-if="rules">
      <section class="ggo-section">
        <header class="ggo-section-head">
          <div>
            <h3>入口与临时终端</h3>
            <p>玩家由 NPC 随机传送进入，随身携带临时终端；终端不限容量，也不能搜索物资。</p>
          </div>
        </header>
        <div class="ggo-fixed-grid">
          <div class="ggo-fixed">
            <span>进入方式</span>
            <strong>NPC 随机传送</strong>
          </div>
          <div class="ggo-fixed">
            <span>临时终端</span>
            <strong>{{ rules.terminalName }}</strong>
          </div>
          <div class="ggo-fixed">
            <span>终端容量</span>
            <strong>不限容量</strong>
          </div>
          <div class="ggo-fixed">
            <span>搜索点</span>
            <strong>关闭</strong>
          </div>
          <div class="ggo-fixed">
            <span>初始物资来源</span>
            <strong>小怪</strong>
          </div>
          <div class="ggo-fixed">
            <span>PvP 掉落归属</span>
            <strong>胜者终端</strong>
          </div>
        </div>
        <NForm label-placement="top" :disabled="disabled" class="mt-4">
          <div class="ggo-field-grid">
            <NFormItem label="落地保护（秒）">
              <NInputNumber
                v-model:value="rules.protectionSeconds"
                :input-props="{ 'aria-label': '落地保护（秒）' }"
                :min="0"
                :max="300"
                :precision="0"
              />
              <template #feedback>范围 0–300 秒；当前只记录配置，不改变战斗逻辑。</template>
            </NFormItem>
          </div>
        </NForm>
      </section>

      <section class="ggo-section">
        <header class="ggo-section-head">
          <div>
            <h3>死亡与撤离结算</h3>
            <p>死亡返回和撤离返回都指向安全区；撤离提示只负责送回，不交付芯片结算。</p>
          </div>
        </header>
        <div class="ggo-fixed-grid">
          <div class="ggo-fixed">
            <span>死亡返回</span>
            <strong>安全区</strong>
          </div>
          <div class="ggo-fixed">
            <span>撤离返回</span>
            <strong>初始安全区</strong>
          </div>
          <div class="ggo-fixed">
            <span>结算方式</span>
            <strong>安全区 NPC 或确认离开</strong>
          </div>
          <div class="ggo-fixed">
            <span>扣除死亡损失后</span>
            <strong>剩余芯片留在终端</strong>
          </div>
        </div>
        <NForm label-placement="top" :disabled="disabled" class="mt-4">
          <div class="ggo-field-grid">
            <NFormItem label="死亡损失比例（%）">
              <NInputNumber
                v-model:value="rules.deathLossPercent"
                :input-props="{ 'aria-label': '死亡损失比例（%）' }"
                :min="0"
                :max="100"
                :precision="0"
                clearable
                placeholder="待定"
              />
              <template #feedback>留空表示死亡损失比例待定。</template>
            </NFormItem>
            <NFormItem label="PvE 损失方式">
              <NSelect v-model:value="rules.pveLossMode" :options="pveLossOptions" />
              <template #feedback>“待定”表示尚未确定，不能视为已经采用销毁规则。</template>
            </NFormItem>
            <NFormItem label="撤离读条（秒）">
              <NInputNumber
                v-model:value="rules.extractionSeconds"
                :input-props="{ 'aria-label': '撤离读条（秒）' }"
                :min="0"
                :max="300"
                :precision="0"
                clearable
                placeholder="待定"
              />
              <template #feedback>留空表示撤离时长待定。</template>
            </NFormItem>
          </div>
        </NForm>
        <p class="ggo-footnote">安全区结算由结算员对话单独说明，撤离接应人不接收芯片。</p>
        <NCollapse class="mt-3">
          <NCollapseItem title="撤离提示文案" name="extraction">
            <p class="ggo-footnote mb-3">这些文案只描述返回初始安全区的过程，不承诺结算结果。</p>
            <NForm label-placement="top" :disabled="disabled">
              <div class="ggo-field-grid">
                <NFormItem label="抵达接应点">
                  <NInput v-model:value="value.extraction.arrival" :maxlength="500" />
                </NFormItem>
                <NFormItem label="确认撤离">
                  <NInput v-model:value="value.extraction.confirmation" :maxlength="500" />
                </NFormItem>
                <NFormItem label="确认按钮">
                  <NInput v-model:value="value.extraction.confirmButton" :maxlength="500" />
                </NFormItem>
                <NFormItem label="继续按钮">
                  <NInput v-model:value="value.extraction.continueButton" :maxlength="500" />
                </NFormItem>
                <NFormItem label="撤离倒计时">
                  <NInput v-model:value="value.extraction.countdown" :maxlength="500" />
                </NFormItem>
                <NFormItem label="离开接应范围">
                  <NInput v-model:value="value.extraction.cancelled" :maxlength="500" />
                </NFormItem>
              </div>
            </NForm>
          </NCollapseItem>
        </NCollapse>
      </section>

      <section class="ggo-section">
        <header class="ggo-section-head">
          <div>
            <h3>交易与离开确认</h3>
            <p>安全区交易范围与赠送仍待确定；离开确认会展示预计获得的信用点。</p>
          </div>
        </header>
        <NForm label-placement="top" :disabled="disabled">
          <div class="ggo-field-grid">
            <NFormItem label="安全区交易">
              <div class="flex items-center gap-2">
                <NSwitch v-model:value="rules.safeTradeEnabled" />
                <span class="text-sm text-gray-600 dark:text-gray-300">
                  {{ rules.safeTradeEnabled ? '计划开放' : '计划关闭' }}
                </span>
              </div>
              <template #feedback>配置安全区内的玩家芯片交易。</template>
            </NFormItem>
            <NFormItem label="可交易内容">
              <NSelect v-model:value="rules.tradeAssets" :options="tradeAssetOptions" />
              <template #feedback>“待定”不代表芯片交易已开放。</template>
            </NFormItem>
            <NFormItem label="允许赠送">
              <NSelect v-model:value="giftValue" :options="giftOptions" />
              <template #feedback>是否允许单向赠送。</template>
            </NFormItem>
          </div>
          <NFormItem label="离开确认文案">
            <NInput
              v-model:value="rules.leaveConfirmation"
              :input-props="{ 'aria-label': '离开确认文案' }"
              type="textarea"
              :maxlength="500"
              :autosize="{ minRows: 2, maxRows: 4 }"
            />
            <template #feedback>保留 {信用点} 占位符，由结算时填充实际数值。</template>
          </NFormItem>
          <div class="ggo-field-grid">
            <NFormItem label="确认按钮">
              <NInput
                v-model:value="rules.leaveConfirmButton"
                :input-props="{ 'aria-label': '结算并离开按钮文案' }"
                :maxlength="80"
              />
            </NFormItem>
            <NFormItem label="取消按钮">
              <NInput
                v-model:value="rules.leaveCancelButton"
                :input-props="{ 'aria-label': '离开取消按钮文案' }"
                :maxlength="80"
              />
            </NFormItem>
          </div>
        </NForm>
      </section>

      <section class="ggo-section">
        <header class="ggo-section-head">
          <div>
            <h3>小怪与 Boss</h3>
            <p>首版使用小怪；Boss 保留候选，是否开放后续确定。</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <NTag size="small">{{ value.monsters.length }} 种小怪</NTag>
            <NTag size="small">{{ value.bosses.length }} 个 Boss 候选</NTag>
          </div>
        </header>
        <div class="ggo-subhead">
          <h4>小怪</h4>
          <span>统一掉落池见下方“六品质芯片”</span>
        </div>
        <NCollapse>
          <NCollapseItem v-for="enemy in value.monsters" :key="enemy.id" :name="`monster:${enemy.id}`">
            <template #header>
              <div class="ggo-enemy-head">
                <strong>{{ enemy.name }}</strong>
                <span>{{ durationText(enemy.respawnMs) }}复活</span>
              </div>
            </template>
            <NForm label-placement="top" :disabled="disabled">
              <div class="ggo-field-grid">
                <NFormItem label="显示名称">
                  <NInput v-model:value="enemy.name" :maxlength="80" />
                </NFormItem>
                <NFormItem label="复活间隔（秒）">
                  <NInputNumber
                    :value="enemy.respawnMs / 1000"
                    :min="1"
                    :max="604800"
                    :precision="3"
                    @update:value="enemy.respawnMs = Math.round(($event ?? 0) * 1000)"
                  />
                  <template #feedback>30 秒为测试初值，可调；旧默认 60 秒会由后台迁移为 30 秒，自定义值保留。</template>
                </NFormItem>
              </div>
            </NForm>
            <p class="ggo-raw">模型 {{ enemy.heroId }}</p>
          </NCollapseItem>
        </NCollapse>

        <div class="ggo-subhead">
          <h4>Boss 候选</h4>
          <span>沿用原 3 个候选，默认 60 分钟</span>
        </div>
        <NCollapse>
          <NCollapseItem v-for="enemy in value.bosses" :key="enemy.id" :name="`boss:${enemy.id}`">
            <template #header>
              <div class="ggo-enemy-head">
                <strong>{{ enemy.name }}</strong>
                <span>{{ durationText(enemy.respawnMs) }}复活</span>
                <NTag size="small" :bordered="false">候选</NTag>
              </div>
            </template>
            <NForm label-placement="top" :disabled="disabled">
              <div class="ggo-field-grid">
                <NFormItem label="显示名称">
                  <NInput v-model:value="enemy.name" :maxlength="80" />
                </NFormItem>
                <NFormItem label="复活间隔（秒）">
                  <NInputNumber
                    :value="enemy.respawnMs / 1000"
                    :min="1"
                    :max="604800"
                    :precision="3"
                    @update:value="enemy.respawnMs = Math.round(($event ?? 0) * 1000)"
                  />
                  <template #feedback>保留自定义复活时间；默认 3600 秒仅为候选初值。</template>
                </NFormItem>
              </div>
            </NForm>
            <p class="ggo-raw">模型 {{ enemy.heroId }}</p>
          </NCollapseItem>
        </NCollapse>
        <p class="ggo-footnote mt-3">六品质统一小怪掉落池在下方配置。</p>
      </section>

      <section class="ggo-section">
        <header class="ggo-section-head">
          <div>
            <h3>六品质芯片</h3>
            <p>设置小怪掉落权重、单次数量及每枚兑换的信用点。</p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <NTag size="small" :type="dropPoolIncomplete ? 'warning' : 'success'">
              掉落 {{ dropReadyCount }}/{{ value.chips.length }}
            </NTag>
            <NTag size="small" :type="exchangeIncomplete ? 'warning' : 'success'">
              兑换 {{ exchangeReadyCount }}/{{ value.chips.length }}
            </NTag>
          </div>
        </header>
        <NAlert v-if="dropPoolIncomplete || exchangeIncomplete" type="warning" :bordered="false" class="mb-3">
          未确定的参数可留空。权重为 0 表示不参与掉落，兑换值为 0 表示不产生信用点。
        </NAlert>
        <div class="ggo-chip-grid">
          <article
            v-for="(chip, index) in value.chips"
            :key="chip.itemId"
            class="ggo-chip border border-gray-200 rounded dark:border-gray-700"
            :style="{ borderLeftColor: qualityOf(chip, index).color }"
          >
            <div class="ggo-chip-head">
              <span class="ggo-quality-dot" :style="{ backgroundColor: qualityOf(chip, index).color }" />
              <div class="ggo-chip-title">
                <strong>{{ qualityOf(chip, index).label }} · {{ chip.name }}</strong>
                <span>{{ chip.itemId }}</span>
              </div>
              <NTag v-if="chipMissing(chip).length" size="small" type="warning">
                缺 {{ chipMissing(chip).length }} 项
              </NTag>
              <NTag v-else size="small" type="success">已齐</NTag>
            </div>
            <div class="ggo-chip-fields">
              <label>
                <span>掉落权重</span>
                <NInputNumber
                  v-model:value="chip.dropWeight"
                  :input-props="{ 'aria-label': chip.name + '掉落权重' }"
                  :min="0"
                  :max="1000000"
                  :precision="0"
                  clearable
                  placeholder="待定"
                  :disabled="disabled"
                />
              </label>
              <label>
                <span>掉落数量</span>
                <NInputNumber
                  v-model:value="chip.dropAmount"
                  :input-props="{ 'aria-label': chip.name + '掉落数量' }"
                  :min="1"
                  :max="10000"
                  :precision="0"
                  clearable
                  placeholder="待定"
                  :disabled="disabled"
                />
              </label>
              <label>
                <span>兑换值</span>
                <NInputNumber
                  v-model:value="chip.exchangeValue"
                  :input-props="{ 'aria-label': chip.name + '兑换信用点' }"
                  :min="0"
                  :max="2147483647"
                  :precision="0"
                  clearable
                  placeholder="待定"
                  :disabled="disabled"
                />
              </label>
            </div>
            <p v-if="chipMissing(chip).length" class="ggo-chip-missing">待定：{{ chipMissing(chip).join('、') }}</p>
          </article>
        </div>
      </section>

      <section class="ggo-section">
        <header class="ggo-section-head">
          <div>
            <h3>经济与周限购</h3>
            <p>账号周限购参数只保存为配置，暂不计算每周购买次数。</p>
          </div>
        </header>
        <NAlert type="warning" :bordered="false" class="mb-3">
          周限购尚未接入玩法或商店周额度执行。信用商店现有超频点数测试商品保持 1 信用点 / 每日 1
          次，仅用于样式测试，不代表正式周限购。
        </NAlert>
        <div class="ggo-fixed-grid">
          <div class="ggo-fixed">
            <span>信用点获取上限</span>
            <strong>日、周获取均不限</strong>
          </div>
          <div class="ggo-fixed">
            <span>限购范围</span>
            <strong>账号每周</strong>
          </div>
          <div class="ggo-fixed">
            <span>重置时区</span>
            <strong>北京时间（{{ rules.weeklyResetTimezone }}）</strong>
          </div>
        </div>
        <NForm label-placement="top" :disabled="disabled" class="mt-4">
          <div class="ggo-field-grid">
            <NFormItem label="每周限购次数">
              <NInputNumber
                v-model:value="rules.weeklyLimit"
                :input-props="{ 'aria-label': '每周限购次数' }"
                :min="1"
                :max="10000"
                :precision="0"
                clearable
                placeholder="待定"
              />
              <template #feedback>留空表示正式次数待定。</template>
            </NFormItem>
            <NFormItem label="每周重置日">
              <NSelect v-model:value="rules.weeklyResetDay" :options="weekdayOptions" clearable placeholder="待定" />
              <template #feedback>按北京时间选择重置日。</template>
            </NFormItem>
            <NFormItem label="重置时间">
              <div class="ggo-time-grid">
                <NInputNumber
                  v-model:value="rules.weeklyResetHour"
                  :input-props="{ 'aria-label': '周重置小时' }"
                  :min="0"
                  :max="23"
                  :precision="0"
                  clearable
                  placeholder="时"
                />
                <NInputNumber
                  v-model:value="rules.weeklyResetMinute"
                  :input-props="{ 'aria-label': '周重置分钟' }"
                  :min="0"
                  :max="59"
                  :precision="0"
                  clearable
                  placeholder="分"
                />
              </div>
              <template #feedback>24 小时制；时、分都可单独留空。</template>
            </NFormItem>
            <NFormItem label="跨服归属">
              <NSelect v-model:value="rules.weeklyServerScope" :options="weeklyServerOptions" />
              <template #feedback>“待定”表示跨服归属尚未确认。</template>
            </NFormItem>
          </div>
        </NForm>
      </section>

      <section class="ggo-section">
        <header class="ggo-section-head">
          <div>
            <h3>NPC 与职责</h3>
            <p>候选 NPC 的职责固定；本轮不提供坐标编辑，旧坐标原样保留。</p>
          </div>
          <NTag size="small">{{ value.npcs.length }} 名候选</NTag>
        </header>
        <NAlert v-if="legacyRole" type="warning" :bordered="false" class="mb-3">
          仍有旧版职责未迁移。后台会修正已知默认职责，保留人工改名、话术与坐标。
        </NAlert>
        <p class="ggo-footnote mb-3">模型 ID 只作配置候选，未部署到场景，也不代表客户端已可用。</p>
        <NCollapse>
          <NCollapseItem v-for="npc in value.npcs" :key="npc.id" :name="npc.id">
            <template #header>
              <div class="ggo-npc-head">
                <strong>{{ npc.name }}</strong>
                <NTag size="small" :type="roleInfo(npc.role).type">{{ roleInfo(npc.role).label }}</NTag>
                <span>{{ npc.pointLabel }}</span>
                <span class="ggo-muted">{{ positionText(npc) }}</span>
                <NTag size="small" :type="npc.settlesChips ? 'success' : 'default'" :bordered="false">
                  {{ npc.settlesChips ? '结算芯片' : '不结算芯片' }}
                </NTag>
              </div>
            </template>
            <p class="ggo-role-note">{{ roleInfo(npc.role).note }}</p>
            <NForm label-placement="top" :disabled="disabled">
              <div class="ggo-field-grid">
                <NFormItem label="显示名称">
                  <NInput v-model:value="npc.name" :maxlength="80" />
                </NFormItem>
                <NFormItem label="拟定地点">
                  <NInput v-model:value="npc.pointLabel" :maxlength="120" />
                </NFormItem>
              </div>
              <NFormItem label="台词">
                <NInput
                  v-model:value="npc.dialogue"
                  type="textarea"
                  :maxlength="1000"
                  :autosize="{ minRows: 2, maxRows: 5 }"
                />
                <template #feedback>
                  {{ roleInfo(npc.role).note }}
                </template>
              </NFormItem>
            </NForm>
            <p class="ggo-raw">模型 {{ npc.heroId }}</p>
          </NCollapseItem>
        </NCollapse>
      </section>
    </template>
  </div>
</template>

<style scoped>
.ggo-editor {
  display: grid;
  gap: 20px;
  min-width: 0;
}
.ggo-head {
  display: grid;
  gap: 6px;
}
.ggo-lede {
  margin: 0;
  color: #6b7280;
  font-size: 13px;
  line-height: 1.6;
}
.ggo-section {
  min-width: 0;
  padding-top: 20px;
  border-top: 1px solid #e5e7eb;
}
.ggo-section-head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px 16px;
}
.ggo-section-head h3 {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  line-height: 1.4;
}
.ggo-section-head p {
  margin: 2px 0 0;
  color: #6b7280;
  font-size: 13px;
  line-height: 1.55;
}
.ggo-field-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 12px 16px;
  align-items: start;
}
.ggo-fixed-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: 6px 18px;
}
.ggo-fixed {
  display: grid;
  gap: 1px;
  padding: 5px 0;
}
.ggo-fixed span {
  color: #6b7280;
  font-size: 12px;
}
.ggo-fixed strong {
  color: #374151;
  font-size: 13px;
  font-weight: 500;
}
.ggo-footnote {
  margin: 0;
  color: #6b7280;
  font-size: 12px;
  line-height: 1.6;
}
.ggo-subhead {
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px 10px;
  margin: 16px 0 8px;
}
.ggo-subhead h4 {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
}
.ggo-subhead span {
  color: #6b7280;
  font-size: 12px;
}
.ggo-enemy-head,
.ggo-npc-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px 10px;
  min-width: 0;
}
.ggo-enemy-head > span,
.ggo-npc-head > span {
  color: #6b7280;
  font-size: 12px;
}
.ggo-muted {
  color: #9ca3af;
}
.ggo-role-note {
  margin: 0 0 12px;
  color: #4b5563;
  font-size: 13px;
  line-height: 1.6;
}
.ggo-raw {
  margin: 8px 0 0;
  color: #9ca3af;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  word-break: break-all;
}
.ggo-chip-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 10px;
}
.ggo-chip {
  display: grid;
  gap: 9px;
  padding: 11px 13px;
  border-left-width: 3px;
  min-width: 0;
}
.ggo-chip-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 7px;
}
.ggo-quality-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex: none;
}
.ggo-chip-title {
  display: grid;
  gap: 1px;
  min-width: 0;
  margin-right: auto;
}
.ggo-chip-title strong {
  font-size: 13px;
  font-weight: 600;
}
.ggo-chip-title span {
  color: #9ca3af;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 10px;
}
.ggo-chip-fields {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(92px, 1fr));
  gap: 8px;
}
.ggo-chip-fields label {
  display: grid;
  gap: 3px;
  min-width: 0;
}
.ggo-chip-fields label > span {
  color: #6b7280;
  font-size: 12px;
}
.ggo-chip-missing {
  margin: 0;
  color: #a16207;
  font-size: 11px;
  line-height: 1.5;
}
.ggo-time-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  width: 100%;
  max-width: 220px;
}
@media (max-width: 600px) {
  .ggo-field-grid,
  .ggo-fixed-grid,
  .ggo-chip-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .ggo-chip-fields {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
  .ggo-chip-fields label:last-child {
    grid-column: 1 / -1;
  }
}
:global(.dark) .ggo-section {
  border-top-color: #374151;
}
:global(.dark) .ggo-lede,
:global(.dark) .ggo-section-head p,
:global(.dark) .ggo-footnote,
:global(.dark) .ggo-subhead span,
:global(.dark) .ggo-enemy-head > span,
:global(.dark) .ggo-npc-head > span,
:global(.dark) .ggo-chip-fields label > span,
:global(.dark) .ggo-fixed span {
  color: #9ca3af;
}
:global(.dark) .ggo-fixed strong,
:global(.dark) .ggo-role-note {
  color: #d1d5db;
}
:global(.dark) .ggo-chip-missing {
  color: #d6b56d;
}
</style>
