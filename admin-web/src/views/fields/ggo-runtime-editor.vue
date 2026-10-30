<script setup lang="ts">
import { computed } from 'vue';
import type { FieldGGOConfiguration, FieldGGOWorldPoint, FieldResource } from '@/service/fields';
import DropEditor from './drop-editor.vue';
const value = defineModel<FieldGGOConfiguration>('value', { required: true });
defineProps<{ disabled: boolean; resources?: FieldResource[]; bossResources?: FieldResource[] }>();
const runtime = computed(() => value.value.runtime!);
type Field = { key: string; label: string; min?: number; max?: number };
const resourceFields: Field[] = [
  { key: 'mineCount', label: '公共矿位数量', min: 1, max: 50 },
  { key: 'fishCount', label: '公共鱼位数量', min: 1, max: 50 },
  { key: 'capacity', label: '每节点公共可采次数', min: 1, max: 100 },
  { key: 'cycleSeconds', label: '强制刷新周期（秒）', min: 60, max: 86400 },
  { key: 'mineCooldownSeconds', label: '矿耗尽刷新（秒）', min: 1, max: 86400 },
  { key: 'fishCooldownSeconds', label: '鱼耗尽刷新（秒）', min: 1, max: 86400 },
  { key: 'mineGatherMillis', label: '挖矿读条（毫秒）', min: 1, max: 300000 },
  { key: 'fishGatherMillis', label: '钓鱼读条（毫秒）', min: 1, max: 300000 },
  { key: 'fatigue', label: '每次消耗疲劳', min: 1, max: 200 },
  { key: 'materialNumerator', label: '材料倍率分子', min: 1, max: 1000 },
  { key: 'materialDenominator', label: '材料倍率分母（四分之一余数）', min: 4, max: 4 }
];
const mobFields: Field[] = [
  { key: 'level', label: '等级', min: 1, max: 1000 },
  { key: 'count', label: '点位数量', min: 1, max: 50 },
  { key: 'hp', label: '生命', min: 1, max: 2000000000 },
  { key: 'attack', label: '攻击', min: 1, max: 2000000000 },
  { key: 'phyDefend', label: '物理防御', min: 0, max: 2000000000 },
  { key: 'magicDefend', label: '魔法防御', min: 0, max: 2000000000 },
  { key: 'respawnSeconds', label: '死亡刷新（秒）', min: 1, max: 604800 },
  { key: 'chipTriggerBps', label: '芯片触发（万分比）', min: 0, max: 10000 },
  { key: 'rewardNumerator', label: '普通奖励倍率分子', min: 1, max: 1000 },
  { key: 'rewardDenominator', label: '普通奖励倍率分母', min: 1, max: 1000 }
];
const bossFields: Field[] = [
  { key: 'level', label: '等级', min: 1, max: 1000 },
  { key: 'partyRounds', label: '每名玩家己方轮数', min: 1, max: 20 },
  { key: 'sproutMax', label: '有效小芽上限', min: 0, max: 2 },
  { key: 'sproutBonusBps', label: '每名小芽矿鱼加成（万分比）', min: 0, max: 10000 },
  { key: 'hp', label: '生命', min: 1, max: 2000000000 },
  { key: 'attack', label: '攻击', min: 1, max: 2000000000 },
  { key: 'phyDefend', label: '物理防御', min: 0, max: 2000000000 },
  { key: 'magicDefend', label: '魔法防御', min: 0, max: 2000000000 },
  { key: 'participationThresholdBps', label: '蓝芯升档贡献（万分比）', min: 1, max: 10000 },
  { key: 'dailyLimit', label: '跨三图每日奖励次数', min: 1, max: 100 },
  { key: 'participationAmount', label: '参与蓝芯数量', min: 1, max: 100 },
  { key: 'thresholdAmount', label: '贡献达标蓝芯数量', min: 1, max: 100 },
  { key: 'activeSeconds', label: '存活时间（秒）', min: 1, max: 86400 },
  { key: 'warningSeconds', label: '提前提示（秒）', min: 0, max: 86400 },
  { key: 'eveningStartHour', label: '晚间开始（北京时间）', min: 0, max: 23 },
  { key: 'dayIntervalMinutes', label: '白天刷新间隔（分钟）', min: 1, max: 1440 },
  { key: 'eveningIntervalMinutes', label: '晚间刷新间隔（分钟）', min: 1, max: 1440 }
];
const shopFields: Field[] = [
  { key: 'priceNumerator', label: '售价倍率分子', min: 1, max: 1000 },
  { key: 'priceDenominator', label: '售价倍率分母', min: 1, max: 1000 },
  { key: 'roundTo', label: '售价向上取整单位', min: 1, max: 1000 },
  { key: 'refreshCost', label: '每次刷新信用点', min: 0, max: 2000000000 }
];
function read(target: object, key: string): number {
  return Number((target as Record<string, unknown>)[key]);
}
function update(target: object, key: string, next: number | null) {
  if (next !== null && Number.isSafeInteger(next)) (target as Record<string, unknown>)[key] = next;
}
const totalWeight = computed(() => runtime.value.resource.tiers.reduce((n, row) => n + row.weight, 0));
const chipWeight = computed(() => value.value.chips.reduce((n, row) => n + (row.dropWeight ?? 0), 0));
const tailWeight = computed(() => runtime.value.boss.tailRewards.reduce((n, row) => n + row.weight, 0));
const chipOptions = computed(() => value.value.chips.map(c => ({ label: c.name, value: c.itemId })));
const minPositive = Number.MIN_VALUE;
const axes = ['x', 'y', 'z'] as const;
const pointKinds = [
  { label: '小怪', value: 'monster' },
  { label: '矿', value: 'mine' },
  { label: '鱼', value: 'fish' },
  { label: 'Boss', value: 'boss' },
  { label: '接应', value: 'exit' }
];
function lowestMonsterLevel() {
  const levels = runtime.value.monsterTiers.map(tier => tier.level).filter(level => Number.isSafeInteger(level) && level >= 1 && level <= 1000);
  return levels.length ? Math.min(...levels) : 0;
}
function addPoint() {
  let i = 1;
  const ids = new Set(runtime.value.points.map(p => p.id));
  while (ids.has(`new-${i}`)) i += 1;
  runtime.value.points.push({
    id: `new-${i}`,
    kind: 'monster',
    level: lowestMonsterLevel(),
    position: null,
    standPosition: null,
    radius: 2
  });
}
function kindText(kind: FieldGGOWorldPoint['kind']) {
  return pointKinds.find(p => p.value === kind)?.label ?? kind;
}
function updatePointKind(point: FieldGGOWorldPoint, kind: FieldGGOWorldPoint['kind']) {
  if (point.kind === kind) return;
  point.kind = kind;
  if (kind === 'monster') {
    point.level = lowestMonsterLevel();
  } else {
    point.level = 0;
    delete point.patrol;
  }
}
function addPatrol(point: FieldGGOWorldPoint) {
  point.patrol = {
    speed: 3.2,
    waypoints: point.position ? [{ ...point.position }] : []
  };
}
const npcRoles: Record<string, string> = { teleport: '随机传送', extraction: '撤离接应', settlement: '安全区结算' };
</script>

<template>
  <div class="desert-runtime">
    <div class="runtime-status">
      <NTag size="small" :type="runtime.enabled ? 'success' : 'default'">
        {{ runtime.enabled ? '玩法配置启用' : '玩法配置关闭' }}
      </NTag>
      <span>保存草稿后，在原发布流程确认生效。</span>
    </div>
    <NForm label-placement="top" :disabled="disabled">
      <section class="runtime-section">
        <h3>入口与结算</h3>
        <div class="runtime-grid">
          <NFormItem label="沙漠玩法开关"><NSwitch v-model:value="runtime.enabled" /></NFormItem>
          <NFormItem label="最低客户端构建号">
            <NInputNumber
              v-model:value="runtime.minClientBuild"
              :min="0"
              :max="2100000000"
              :precision="0"
              :input-props="{ 'aria-label': '最低客户端构建号' }"
            />
          </NFormItem>
          <NFormItem label="经济版本"><NInput v-model:value="runtime.economyVersion" :maxlength="64" /></NFormItem>
          <NFormItem label="地图标题"><NInput v-model:value="value.title" :maxlength="80" /></NFormItem>
          <NFormItem label="玩法名称"><NInput v-model:value="value.modeName" :maxlength="80" /></NFormItem>
          <NFormItem label="落地保护（秒）">
            <NInputNumber v-model:value="value.rules.protectionSeconds" :min="0" :max="300" :precision="0" />
          </NFormItem>
          <NFormItem label="撤离读条（秒）">
            <NInputNumber v-model:value="value.rules.extractionSeconds" :min="0" :max="300" :precision="0" />
          </NFormItem>
          <NFormItem label="保护最高品质芯片（枚）">
            <NInputNumber v-model:value="runtime.protectedChips" :min="0" :max="100" :precision="0" />
          </NFormItem>
          <NFormItem label="死亡扣除比例（%）">
            <NInputNumber v-model:value="value.rules.deathLossPercent" :min="0" :max="100" :precision="0" />
          </NFormItem>
        </div>
        <p class="context">
          死亡以总数计算，向下取整，扣除数不超过未保护数量；PK损失转给胜者。变更经济版本前须处理未结余额。
        </p>
      </section>

      <section class="runtime-section">
        <h3>公共矿与鱼</h3>
        <div class="runtime-grid">
          <NFormItem v-for="f in resourceFields" :key="f.key" :label="f.label">
            <NInputNumber
              :value="read(runtime.resource, f.key)"
              :min="f.min"
              :max="f.max"
              :precision="0"
              :input-props="{ 'aria-label': f.label }"
              @update:value="update(runtime.resource, f.key, $event)"
            />
          </NFormItem>
        </div>
        <p class="context">
          每条线共享节点与可采次数。强刷按北京时间整点周期重抽补满，耗尽刷新独立计时；材料按倍率发放整数，余数结转。
        </p>
        <div class="table-wrap">
          <table>
            <caption>
              资源阶梯
              <span :class="{ invalid: totalWeight !== 10000 }">权重合计 {{ totalWeight }} / 10000</span>
            </caption>
            <thead>
              <tr>
                <th>技能等级</th>
                <th>出现权重</th>
                <th>芯片触发（万分比）</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="tier in runtime.resource.tiers" :key="tier.level">
                <td>{{ tier.level }} 阶</td>
                <td>
                  <NInputNumber
                    v-model:value="tier.weight"
                    :min="0"
                    :max="10000"
                    :precision="0"
                    :input-props="{ 'aria-label': `${tier.level}阶出现权重` }"
                  />
                </td>
                <td>
                  <NInputNumber
                    v-model:value="tier.chipTriggerBps"
                    :min="0"
                    :max="10000"
                    :precision="0"
                    :input-props="{ 'aria-label': `${tier.level}阶采集芯片触发` }"
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <NCollapse class="mb-4">
        <NCollapseItem
          v-for="tier in runtime.resource.tiers"
          :key="tier.level"
          :name="tier.level"
          :title="`${tier.level}阶原材料与掉落数量`"
        >
          <h4 class="mb-2">矿石基数</h4>
          <DropEditor
            v-if="tier.mineDrops"
            v-model:value="tier.mineDrops"
            :disabled="disabled"
            resource-locked
            :max-amount="10000"
            :resources="resources ?? []"
          />
          <h4 class="mb-2 mt-4">鱼种与独立命中</h4>
          <DropEditor
            v-if="tier.fishDrops"
            v-model:value="tier.fishDrops"
            :disabled="disabled"
            resource-locked
            :max-amount="10000"
            :resources="resources ?? []"
          />
          <p class="context mt-3">
            数量填写倍率应用前的基数，0数量档位表示未命中；同种资源按权重抽一档，再应用材料倍率。
          </p>
        </NCollapseItem>
      </NCollapse>
      <section class="runtime-section">
        <h3>分级公共小怪</h3>
        <NCollapse>
          <NCollapseItem
            v-for="(tier, index) in runtime.monsterTiers"
            :key="index"
            :name="index"
            :title="`${tier.level} 级 · ${tier.count} 只 · 芯片 ${tier.chipTriggerBps / 100}%`"
          >
            <div class="runtime-grid">
              <NFormItem label="怪物模型">
                <NSelect
                  v-model:value="tier.heroId"
                  :options="value.monsters.filter(enemy => ['monster_026', 'monster_032', 'monster_031'].includes(enemy.heroId)).map(enemy => ({ label: enemy.name, value: enemy.heroId }))"
                />
              </NFormItem>
              <NFormItem v-for="f in mobFields" :key="f.key" :label="f.label">
                <NInputNumber
                  :value="read(tier, f.key)"
                  :min="f.min"
                  :max="f.max"
                  :precision="0"
                  :input-props="{ 'aria-label': `${tier.level}级${f.label}` }"
                  @update:value="update(tier, f.key, $event)"
                />
              </NFormItem>
            </div>
          </NCollapseItem>
        </NCollapse>
      </section>

      <section class="runtime-section">
        <h3>每线首领</h3>
        <div class="runtime-grid">
          <NFormItem label="Boss 模型">
            <NSelect
              v-model:value="runtime.boss.heroId"
              :options="[...value.bosses.filter(enemy => enemy.heroId === 'hero_017').map(enemy => ({ label: enemy.name, value: enemy.heroId })), { label: '双头怪', value: 'monster_047' }]"
            />
          </NFormItem>
          <NFormItem v-for="f in bossFields" :key="f.key" :label="f.label">
            <NInputNumber
              :value="read(runtime.boss, f.key)"
              :min="f.min"
              :max="f.max"
              :precision="0"
              :input-props="{ 'aria-label': `Boss ${f.label}` }"
              @update:value="update(runtime.boss, f.key, $event)"
            />
          </NFormItem>
          <NFormItem label="参与芯片">
            <NSelect v-model:value="runtime.boss.participationItemId" :options="chipOptions" />
          </NFormItem>
        </div>
        <NCollapse>
          <NCollapseItem title="参与、排名与尾刀资源" name="boss-resources">
            <h4>参与资源包</h4>
            <DropEditor
              v-model:value="runtime.boss.resourceDrops"
              :disabled="disabled"
              :min-weight="0"
              :resources="bossResources ?? []"
            />
            <div v-for="(band, index) in runtime.boss.rankResourceDrops" :key="index" class="boss-resource-band">
              <div class="runtime-grid">
                <NFormItem label="起始名次">
                  <NInputNumber v-model:value="band.from" :min="1" :max="100" :precision="0" />
                </NFormItem>
                <NFormItem label="结束名次"><NInputNumber v-model:value="band.to" :min="1" :max="100" :precision="0" /></NFormItem>
              </div>
              <DropEditor v-model:value="band.drops" :disabled="disabled" :min-weight="0" :resources="bossResources ?? []" />
            </div>
            <h4>尾刀资源包</h4>
            <DropEditor
              v-model:value="runtime.boss.tailResourceDrops"
              :disabled="disabled"
              :min-weight="0"
              :resources="bossResources ?? []"
            />
            <p class="context">
              小芽只影响参与矿鱼包：普通矿鱼有额外复制机会，黄金鱼提高出现概率；钻石、金币、排名与芯片保持原数量。
            </p>
          </NCollapseItem>
          <NCollapseItem title="超频点数阶梯" name="boss-overclock">
            <div v-for="tier in runtime.boss.overclockRewards" :key="tier.sprouts" class="table-wrap">
              <table>
                <caption>{{ tier.sprouts }} 名有效小芽 · 超频点数</caption>
                <thead>
                  <tr>
                    <th>点数</th>
                    <th>权重</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(q, index) in tier.quantities" :key="index">
                    <td><NInputNumber v-model:value="q.amount" :min="1" :max="2147483647" :precision="0" :disabled="disabled" /></td>
                    <td>
                      <NInputNumber
                        v-model:value="q.weight"
                        :min="0"
                        :max="10000"
                        :precision="0"
                        :disabled="disabled"
                      />
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </NCollapseItem>
          <NCollapseItem title="伤害排名与尾刀芯片" name="boss-rewards">
            <div class="table-wrap">
              <table>
                <caption>伤害排名</caption>
                <thead>
                  <tr>
                    <th>起始名次</th>
                    <th>结束名次</th>
                    <th>芯片</th>
                    <th>数量</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(row, index) in runtime.boss.rankRewards" :key="index">
                    <td><NInputNumber v-model:value="row.from" :min="1" :max="100" :precision="0" /></td>
                    <td><NInputNumber v-model:value="row.to" :min="1" :max="100" :precision="0" /></td>
                    <td><NSelect v-model:value="row.itemId" :options="chipOptions" /></td>
                    <td><NInputNumber v-model:value="row.amount" :min="1" :max="100" :precision="0" /></td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="table-wrap">
              <table>
                <caption>
                  尾刀互斥抽取1枚
                  <span :class="{ invalid: tailWeight !== 10000 }">权重合计 {{ tailWeight }} / 10000</span>
                </caption>
                <thead>
                  <tr>
                    <th>芯片</th>
                    <th>权重</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(row, index) in runtime.boss.tailRewards" :key="index">
                    <td><NSelect v-model:value="row.itemId" :options="chipOptions" /></td>
                    <td><NInputNumber v-model:value="row.weight" :min="0" :max="10000" :precision="0" /></td>
                  </tr>
                </tbody>
              </table>
            </div>
          </NCollapseItem>
        </NCollapse>
      </section>

      <section class="runtime-section">
        <h3>六品质芯片与信用商店</h3>
        <div class="table-wrap">
          <table class="chip-table">
            <caption>
              条件品质池
              <span :class="{ invalid: chipWeight !== 10000 }">权重合计 {{ chipWeight }} / 10000</span>
            </caption>
            <thead>
              <tr>
                <th>芯片</th>
                <th>品质权重</th>
                <th>每次枚数</th>
                <th>每枚信用点</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="chip in value.chips" :key="chip.itemId">
                <th scope="row">{{ chip.name }}</th>
                <td><NInputNumber v-model:value="chip.dropWeight" :min="0" :max="10000" :precision="0" /></td>
                <td><NInputNumber v-model:value="chip.dropAmount" :min="1" :max="1" :precision="0" /></td>
                <td><NInputNumber v-model:value="chip.exchangeValue" :min="1" :precision="0" /></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="runtime-grid">
          <NFormItem v-for="f in shopFields" :key="f.key" :label="f.label">
            <NInputNumber
              :value="read(runtime.shop, f.key)"
              :min="f.min"
              :max="f.max"
              :precision="0"
              @update:value="update(runtime.shop, f.key, $event)"
            />
          </NFormItem>
        </div>
        <p class="context">
          建议售价按原价×分子÷分母，再向上取整。数量、见货权重和账号周限沿用信用商店原编辑器；售价只允许上调。
        </p>
      </section>

      <section class="runtime-section">
        <h3>地图点位与安全区</h3>
        <p class="context">
          填写 Unity 已核验的世界坐标。底图像素不能直接填入；鱼模型与岸上站位分开。空坐标可存草稿，启用前须补齐且交互半径大于 0。
        </p>
        <NCollapse>
          <NCollapseItem title="安全区边界与返回位置" name="safe-coordinates">
            <div v-if="runtime.safeArea" class="runtime-grid">
              <NFormItem
                v-for="(label, key) in { minX: '最小X', maxX: '最大X', minZ: '最小Z', maxZ: '最大Z' }"
                :key="key"
                :label="label"
              >
                <NInputNumber v-model:value="runtime.safeArea[key]" :min="-100000" :max="100000" />
              </NFormItem>
            </div>
            <NButton
              v-else
              :disabled="disabled"
              secondary
              @click="runtime.safeArea = { minX: -11, maxX: 3, minZ: -10, maxZ: 1 }"
            >
              设置安全区边界
            </NButton>
            <div v-if="runtime.safeReturn" class="runtime-grid">
              <NFormItem v-for="axis in axes" :key="axis" :label="`返回位置 ${axis.toUpperCase()}`">
                <NInputNumber v-model:value="runtime.safeReturn[axis]" :min="-100000" :max="100000" />
              </NFormItem>
            </div>
            <NButton v-else :disabled="disabled" secondary @click="runtime.safeReturn = { x: null, y: null, z: null }">
              设置返回位置
            </NButton>
          </NCollapseItem>
          <NCollapseItem :title="`随机入场落点 · ${runtime.entryPoints?.length ?? 0} 处`" name="entry-coordinates">
            <div v-for="(position, index) in runtime.entryPoints" :key="index" class="point-entry">
              <div class="runtime-grid">
                <NFormItem v-for="axis in axes" :key="axis" :label="`落点${index + 1} ${axis.toUpperCase()}`">
                  <NInputNumber v-model:value="position[axis]" :min="-100000" :max="100000" />
                </NFormItem>
              </div>
              <NButton :disabled="disabled" size="small" secondary @click="runtime.entryPoints.splice(index, 1)">
                移除此落点
              </NButton>
            </div>
            <NButton :disabled="disabled" secondary @click="runtime.entryPoints.push({ x: null, y: null, z: null })">
              添加入场落点
            </NButton>
          </NCollapseItem>
          <NCollapseItem :title="`公共对象点位 · ${runtime.points?.length ?? 0} 处`" name="world-coordinates">
            <NCollapse>
              <NCollapseItem
                v-for="(point, index) in runtime.points"
                :key="index"
                :name="index"
                :title="`${point.id} · ${kindText(point.kind)}${point.kind === 'monster' ? ` · ${point.level}级` : ''}${point.position ? '' : ' · 坐标待填'}`"
              >
                <div class="runtime-grid">
                  <NFormItem label="点位编号"><NInput v-model:value="point.id" /></NFormItem>
                  <NFormItem label="类型"><NSelect :value="point.kind" :options="pointKinds" @update:value="updatePointKind(point, $event)" /></NFormItem>
                  <NFormItem v-if="point.kind === 'monster'" label="小怪等级">
                    <NInputNumber v-model:value="point.level" :min="1" :max="1000" :precision="0" />
                  </NFormItem>
                  <NFormItem label="交互半径（米）">
                    <NInputNumber v-model:value="point.radius" :min="0" :max="100" />
                  </NFormItem>
                </div>
                <p v-if="point.kind === 'monster' && point.patrol" class="context">切换为非小怪会清除该点位的巡逻路径。</p>
                <div v-if="point.position" class="runtime-grid">
                  <NFormItem v-for="axis in axes" :key="axis" :label="`模型 ${axis.toUpperCase()}`">
                    <NInputNumber v-model:value="point.position[axis]" :min="-100000" :max="100000" />
                  </NFormItem>
                </div>
                <NButton v-else :disabled="disabled" secondary @click="point.position = { x: null, y: null, z: null }">
                  设置模型位置
                </NButton>
                <div v-if="point.standPosition" class="runtime-grid">
                  <NFormItem v-for="axis in axes" :key="axis" :label="`交互站位 ${axis.toUpperCase()}`">
                    <NInputNumber v-model:value="point.standPosition[axis]" :min="-100000" :max="100000" />
                  </NFormItem>
                </div>
                <NButton
                  v-else
                  :disabled="disabled"
                  secondary
                  @click="point.standPosition = { x: null, y: null, z: null }"
                >
                  {{ point.kind === 'fish' ? '设置岸上钓鱼站位' : '设置独立交互站位' }}
                </NButton>
                <NButton
                  :disabled="disabled"
                  secondary
                  size="small"
                  class="ml-3"
                  @click="runtime.points.splice(index, 1)"
                >
                  移除此点位
                </NButton>
                <div v-if="point.kind === 'monster'" class="point-entry">
                  <template v-if="point.patrol">
                    <h4>公共巡逻路径</h4>
                    <p class="context">仅最低等级小怪可巡逻，启用须至少四只。首个路点须与模型位置一致；路径须经过 Unity 导航核验。</p>
                    <NFormItem label="巡逻速度（米/秒）">
                      <NInputNumber
                        v-model:value="point.patrol.speed"
                        :min="minPositive"
                        :max="50"
                        :input-props="{ 'aria-label': `${point.id}巡逻速度` }"
                      />
                    </NFormItem>
                    <div v-for="(waypoint, waypointIndex) in point.patrol.waypoints" :key="waypointIndex" class="point-entry">
                      <div class="runtime-grid">
                        <NFormItem v-for="axis in axes" :key="axis" :label="`路点${waypointIndex + 1} ${axis.toUpperCase()}`">
                          <NInputNumber
                            v-model:value="waypoint[axis]"
                            :min="-100000"
                            :max="100000"
                            :input-props="{ 'aria-label': `${point.id}路点${waypointIndex + 1} ${axis.toUpperCase()}` }"
                          />
                        </NFormItem>
                      </div>
                      <NButton :disabled="disabled" secondary size="small" @click="point.patrol.waypoints.splice(waypointIndex, 1)">移除此路点</NButton>
                    </div>
                    <NSpace class="mt-3">
                      <NButton :disabled="disabled || point.patrol.waypoints.length >= 32" secondary size="small" @click="point.patrol.waypoints.push({ x: null, y: null, z: null })">添加路点</NButton>
                      <NButton :disabled="disabled" secondary size="small" @click="delete point.patrol">取消巡逻</NButton>
                    </NSpace>
                  </template>
                  <NButton v-else :disabled="disabled" secondary size="small" @click="addPatrol(point)">设置巡逻路径</NButton>
                </div>
              </NCollapseItem>
            </NCollapse>
            <NButton :disabled="disabled" secondary class="mt-4" @click="addPoint">添加公共点位</NButton>
          </NCollapseItem>
        </NCollapse>
      </section>

      <section class="runtime-section">
        <h3>交易与 NPC</h3>
        <div class="runtime-grid">
          <NFormItem label="安全区芯片交易"><NSwitch v-model:value="value.rules.safeTradeEnabled" /></NFormItem>
          <NFormItem label="允许赠送">
            <NSwitch :value="value.rules.allowGift === true" @update:value="value.rules.allowGift = $event" />
          </NFormItem>
        </div>
        <NCollapse>
          <NCollapseItem
            v-for="npc in value.npcs"
            :key="npc.id"
            :name="npc.id"
            :title="`${npc.name} · ${npcRoles[npc.role] ?? npc.role}`"
          >
            <div class="runtime-grid">
              <NFormItem label="名称"><NInput v-model:value="npc.name" :maxlength="80" /></NFormItem>
              <NFormItem label="地点名称"><NInput v-model:value="npc.pointLabel" :maxlength="120" /></NFormItem>
            </div>
            <div v-if="npc.position" class="runtime-grid">
              <NFormItem v-for="axis in ['x', 'y', 'z'] as const" :key="axis" :label="`世界坐标 ${axis.toUpperCase()}`">
                <NInputNumber
                  v-model:value="npc.position[axis]"
                  :min="-100000"
                  :max="100000"
                  :input-props="{ 'aria-label': `${npc.name}坐标${axis}` }"
                />
              </NFormItem>
            </div>
            <NButton
              v-else
              :disabled="disabled"
              secondary
              size="small"
              class="mb-4"
              @click="npc.position = { x: null, y: null, z: null }"
            >
              设置世界坐标
            </NButton>
            <NFormItem label="台词">
              <NInput
                v-model:value="npc.dialogue"
                type="textarea"
                :maxlength="1000"
                :autosize="{ minRows: 2, maxRows: 4 }"
              />
            </NFormItem>
          </NCollapseItem>
          <NCollapseItem title="撤离与离开提示" name="copy">
            <NFormItem
              v-for="(_, key) in value.extraction"
              :key="key"
              :label="
                (
                  {
                    arrival: '抵达接应点',
                    confirmation: '确认撤离',
                    confirmButton: '撤离按钮',
                    continueButton: '继续按钮',
                    countdown: '撤离倒计时',
                    cancelled: '撤离中断'
                  } as Record<string, string>
                )[key]
              "
            >
              <NInput v-model:value="value.extraction[key]" :maxlength="500" />
            </NFormItem>
            <NFormItem label="结算并离开确认">
              <NInput v-model:value="value.rules.leaveConfirmation" type="textarea" :maxlength="500" />
            </NFormItem>
            <div class="runtime-grid">
              <NFormItem label="确认按钮">
                <NInput v-model:value="value.rules.leaveConfirmButton" :maxlength="80" />
              </NFormItem>
              <NFormItem label="取消按钮">
                <NInput v-model:value="value.rules.leaveCancelButton" :maxlength="80" />
              </NFormItem>
            </div>
          </NCollapseItem>
        </NCollapse>
      </section>
    </NForm>
  </div>
</template>

<style scoped>
.desert-runtime {
  min-width: 0;
}
.runtime-status {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  align-items: center;
}
.runtime-status > span,
.context {
  color: var(--n-text-color-3, #6b7280);
  font-size: 12px;
  line-height: 1.6;
}
.runtime-section {
  margin-top: 24px;
  padding-top: 20px;
  border-top: 1px solid var(--n-border-color, #e5e7eb);
}
.runtime-section h3 {
  margin: 0 0 16px;
  font-size: 15px;
  font-weight: 600;
}
.runtime-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 0 20px;
}
.runtime-grid :deep(.n-input-number) {
  width: 100%;
}
.context {
  margin: 0 0 14px;
}
.table-wrap {
  overflow-x: auto;
  margin: 12px 0 20px;
}
table {
  width: 100%;
  border-collapse: collapse;
  min-width: 480px;
  font-size: 13px;
}
caption {
  text-align: left;
  padding: 0 0 10px;
  font-weight: 600;
}
caption span {
  margin-left: 12px;
  color: #6b7280;
  font-weight: 400;
}
.invalid {
  color: #d03050 !important;
}
th {
  text-align: left;
  font-weight: 500;
  white-space: nowrap;
}
th,
td {
  padding: 10px 12px;
  border-bottom: 1px solid var(--n-border-color, #e5e7eb);
  vertical-align: middle;
}
th:first-child,
td:first-child {
  padding-left: 0;
}
td :deep(.n-input-number) {
  min-width: 112px;
}
td :deep(.n-select) {
  min-width: 136px;
}
@media (max-width: 600px) {
  .runtime-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .runtime-section {
    margin-top: 20px;
    padding-top: 16px;
  }
  caption span {
    display: block;
    margin: 4px 0 0;
  }
  .chip-table th:first-child {
    position: sticky;
    left: 0;
    z-index: 1;
    background: var(--n-color, #fff);
  }
}
</style>
