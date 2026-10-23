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

/** NPC 职责固定：随机传送入口、撤离接应或安全区结算。 */
export type FieldGGONPCRole = 'teleport' | 'extraction' | 'settlement';
export interface FieldGGONPC {
  id: string;
  name: string;
  heroId: string;
  role: FieldGGONPCRole;
  pointLabel: string;
  dialogue: string;
  /** 坐标沿用旧配置；本版后台不提供坐标编辑。 */
  position: { x: number | null; y: number | null; z: number | null } | null;
  settlesChips: boolean;
}
export interface FieldGGOEnemy {
  id: string;
  name: string;
  heroId: string;
  respawnMs: number;
}
/** schemaVersion 2 的 GGO 规则。固定字符串与布尔值只读，可空数值 null 表示待定。 */
export interface FieldGGORules {
  entryMethod: 'npc_random';
  protectionSeconds: number;
  terminalName: string;
  unlimitedCapacity: true;
  searchEnabled: false;
  initialSource: 'monsters';
  pvpTransfer: 'winner_terminal';
  deathReturn: 'safe_area';
  deathLossPercent: number | null;
  pveLossMode: 'pending' | 'destroy';
  extractionReturn: 'safe_area';
  extractionSeconds: number | null;
  settlementMode: 'safe_npc_or_confirm_leave';
  retainAfterDeath: true;
  safeTradeEnabled: boolean;
  tradeAssets: 'pending' | 'chips_only';
  allowGift: boolean | null;
  creditCap: 'none';
  shopLimitScope: 'account_weekly';
  weeklyLimit: number | null;
  /** 1 为周一，7 为周日。 */
  weeklyResetDay: number | null;
  weeklyResetHour: number | null;
  weeklyResetMinute: number | null;
  weeklyResetTimezone: 'Asia/Shanghai';
  weeklyServerScope: 'pending' | 'current_server' | 'all_servers';
  leaveConfirmation: string;
  leaveConfirmButton: string;
  leaveCancelButton: string;
}
export interface FieldGGOConfiguration {
  schemaVersion: 2;
  title: string;
  modeName: string;
  currencyId: string;
  shopKey: string;
  carryIcon: string;
  npcs: FieldGGONPC[];
  monsters: FieldGGOEnemy[];
  bosses: FieldGGOEnemy[];
  /** 六品质芯片；掉落权重、数量与兑换值可为 null（待定）。 */
  chips: {
    itemId: string;
    name: string;
    exchangeValue: number | null;
    dropWeight: number | null;
    dropAmount: number | null;
  }[];
  rules: FieldGGORules;
  extraction: {
    arrival: string;
    confirmation: string;
    confirmButton: string;
    continueButton: string;
    countdown: string;
    cancelled: string;
  };
}
