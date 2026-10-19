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
export interface MonsterGroup {
  id: string;
  heroId: string;
  level: number;
  cooldownMs: number;
  drops: FieldDrop[];
}
export interface BossReward {
  type: number;
  level: number;
  weighted: boolean;
  drops: FieldDrop[];
}
export interface FieldConfiguration {
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
  bossEnabled: boolean;
  bossRewards: BossReward[];
}
export interface FieldTemplate {
  id: string;
  name: string;
  available: boolean;
  unavailableReason: string;
  version: string;
  configuration: FieldConfiguration;
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

export interface FieldPresentation {
  name: string;
  iconUrl: string;
  detail: string;
}
export interface FieldChannel {
  scene: string;
  name: string;
  lineCount: number;
  maxPlayers: number;
  version: string;
  lines: { line: number; online: number }[];
}
