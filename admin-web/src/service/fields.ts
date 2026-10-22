import type { Item } from '@/service/operations';
export interface FieldResource extends Item {
  allowedKinds: string[];
  maxAmount: number;
}
export interface FieldWeight {
  id: string;
  weight: number;
}
export interface FieldDrop {
  resourceId: string;
  quantities: { amount: number; weight: number }[];
}
export interface FieldPoint {
  id: string;
  point: string;
  choices: FieldWeight[];
}
export interface GatherGroup {
  id: string;
  level: number;
  gatherMs: number;
  cooldownMs: number;
  times: number;
  fatigue: number;
  drops: FieldDrop[];
}
/** 原表完整战斗成员。身份、阵位和组号只读，数值随发布快照用于新战斗。 */
export interface CombatMember {
  id: string;
  heroId: string;
  location: number;
  groupIndex: number;
  level: number;
  hp: number;
  attack: number;
  phyDefend: number;
  magicDefend: number;
  hit: number;
  dodge: number;
  cri: number;
  antiCri: number;
  criFactor: number;
  damageAdd: number;
  damageReduce: number;
}
export interface MonsterGroup {
  id: string;
  heroId: string;
  level: number;
  cooldownMs: number;
  members?: CombatMember[];
  drops: FieldDrop[];
}
/** 本地图普通首领组；刷新单位为小时，两类奖励沿用原消费者。 */
export interface BossGroup {
  id: string;
  groupIndex: number;
  heroId: string;
  rebornHours: number;
  members?: CombatMember[];
  drops: FieldDrop[];
  killDrops: FieldDrop[];
}
export interface BossReward {
  type: number;
  level: number;
  weighted: boolean;
  drops: FieldDrop[];
}
export interface FieldConfiguration {
  ggo?: FieldGGOConfiguration;
  scene: string;
  sceneResource: string;
  battleResource: string;
  minLevel: number;
  allowPk: boolean;
  mineCount: number;
  fishCount: number;
  minePoints: FieldPoint[];
  fishPoints: FieldPoint[];
  monsterPoints: FieldPoint[];
  gatherGroups: GatherGroup[];
  monsterGroups: MonsterGroup[];
  /** 缺省时普通首领沿用原启用状态。 */
  ordinaryBossEnabled?: boolean;
  ordinaryBossGroups?: BossGroup[];
  /** 世界首领开关，保留既有接口字段。 */
  bossEnabled: boolean;
  bossRewards: BossReward[];
  /** 世界首领基础成员数值，缺省时继承原表。 */
  worldBossMembers?: CombatMember[];
  /** 新建周期等级；已生成周期使用已冻结的等级和系数。 */
  worldBossLevel?: number;
}
export interface FieldTemplate {
  id: string;
  name: string;
  available: boolean;
  unavailableReason: string;
  version: string;
  configuration: FieldConfiguration;
}
/** 现有全服策略的只读出生配置，读取不会生成周期。 */
export interface FieldWorldBossStatus {
  enabled: boolean;
  level: number;
  positionId: number;
  scene: string;
  sceneName: string;
  bossId: string;
}
export interface FieldDeployment {
  scene: string;
  name: string;
  templateId: string;
  enabled: boolean;
  available: boolean;
  version: string;
  templateChanged: boolean;
}
export interface FieldDeployments {
  serverId: number;
  items: FieldDeployment[];
}

export interface FieldMapPoint {
  id: string;
  name: string;
  x: number;
  z: number;
}
export interface FieldMap {
  width: number;
  height: number;
  scale: number;
  offsetX: number;
  offsetY: number;
  bossPoints?: FieldMapPoint[];
  worldBossPoints?: FieldMapPoint[];
}
export interface FieldPresentation {
  name: string;
  iconUrl: string;
  detail: string;
  map?: FieldMap;
}
export interface FieldChannel {
  scene: string;
  name: string;
  lineCount: number;
  maxPlayers: number;
  version: string;
  lines: { line: number; online: number }[];
}

export interface FieldGGONPC {
  id: string;
  name: string;
  heroId: string;
  role: 'guide' | 'settlement';
  pointLabel: string;
  dialogue: string;
  position: { x: number | null; y: number | null; z: number | null } | null;
  settlesChips: boolean;
}
export interface FieldGGOEnemy {
  id: string;
  name: string;
  heroId: string;
  respawnMs: number;
}
export interface FieldGGOConfiguration {
  title: string;
  modeName: string;
  currencyId: string;
  shopKey: string;
  carryIcon: string;
  sproutIcon: string;
  npcs: FieldGGONPC[];
  monsters: FieldGGOEnemy[];
  bosses: FieldGGOEnemy[];
  chips: { itemId: string; name: string; exchangeValue: number | null }[];
  extraction: {
    arrival: string;
    confirmation: string;
    confirmButton: string;
    continueButton: string;
    countdown: string;
    cancelled: string;
  };
}
