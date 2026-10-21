export interface GamePlayer {
  accountId: number;
  username: string;
  nickname: string;
  id: number;
  name: string;
  level: number;
  vip: number;
  chapter: string;
  mission: string;
  scene: string;
  createdAt?: number;
  version: string;
}
export interface InventoryItem {
  resourceId: string;
  amount: number;
  guid?: string;
  level?: number;
}
export interface GamePlayerDetail extends GamePlayer {
  exp: number;
  balances: Record<string, number>;
  counts: Record<string, number>;
  inventory: InventoryItem[];
}

export interface GamePlayerCreate {
  accountId: number | null;
  name: string;
  heroId: string;
  reason: string;
}

export const balanceLabels: Record<string, string> = {
  coin: '金币',
  diamond: '钻石',
  energy: '精力',
  stamina: '体力',
  light: '滞留之光',
  point: '攻略点数',
  popularity: '声望',
  score: '悬赏积分',
  coinpet: '精灵币',
  contribution: '公会贡献'
};
export interface PlayerEditOptions {
  levelMax: number;
  vipMax: number;
  chapters: {
    value: string;
    label: string;
    missions: { value: string; label: string }[];
  }[];
  scenes: { value: string; label: string }[];
  balanceLimits: Record<string, number>;
}
