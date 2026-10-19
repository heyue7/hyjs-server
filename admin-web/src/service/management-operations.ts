import type { Item, ResourceAmount } from './operations';

export interface ShopDefinition {
  id: string;
  label: string;
  notes: string;
  fields: string[];
  settingsFields?: string[];
  canCreate?: boolean;
}
export interface ShopProduct {
  id: string;
  name: string;
  rewards: ResourceAmount[];
  cost: ResourceAmount;
  dailyLimit: number;
  totalLimit: number;
  stock: number;
  minLevel: number;
  minVip: number;
  priority: number;
  weight: number;
  discountBps: number;
  contributionCost: number;
  guildLevel: number;
  day: string;
  buyCondition: number;
  refreshMillis: number;
  enabled: boolean;
  source: string;
  maxRank: number;
}
export interface ShopSnapshot {
  id: string;
  items: ShopProduct[];
  version: string;
  resources?: Item[];
  settings: {
    enabled: boolean;
    slots: number;
    startAt: number;
    endAt: number;
    claimEndAt: number;
    source: string;
    periodId?: string;
    frozen?: boolean;
    newPeriod?: boolean;
  };
}
export interface AchievementKind {
  id: string;
  label: string;
  type: number | string;
  supported: boolean;
  notes: string;
  fixedTarget?: number;
}
export interface Achievement {
  id: string;
  name: string;
  kind: string;
  type: number | string;
  target: number;
  rewards: ResourceAmount[];
  source: string;
  enabled: boolean;
  version: string;
  supported?: boolean;
  notes?: string;
}
export interface TitleEffect {
  key: string;
  value: number;
}
export interface GameTitle {
  iconUrl: string;
  requirement: string;
  id: string;
  name: string;
  type: number;
  param: string;
  effects: TitleEffect[];
  effectTime: number;
  show: boolean;
  enabled: boolean;
  version: string;
  notes: string;
}
export interface FeedbackMessage {
  id: number | string;
  sender: string;
  content: string;
  createdAt: number;
}
export interface FeedbackTicket {
  id: number | string;
  playerId: number;
  playerName: string;
  subject: string;
  category: string;
  state: string;
  createdAt: number;
  updatedAt: number;
  version: string;
  messages?: FeedbackMessage[];
}
export const shopLabels: Record<string, string> = {
  ordinary: '普通商店',
  memory: '记忆圣坛',
  popularity: '声望商店商品',
  'popularity-rank': '声望商店奖励',
  point: '攻略点商店',
  score: '积分商店',
  guild: '公会商店',
  'pet-market': '精灵商店',
  'union-fight': '公会战商店',
  vip: '会员礼包'
};
export function newShopProduct(): ShopProduct {
  return {
    id: '',
    name: '',
    rewards: [],
    cost: { resourceId: '', amount: 1 },
    dailyLimit: 1,
    totalLimit: 0,
    stock: -1,
    minLevel: 1,
    minVip: 0,
    priority: 0,
    weight: 1,
    discountBps: 10000,
    contributionCost: 0,
    guildLevel: 1,
    day: '',
    buyCondition: 0,
    refreshMillis: 0,
    enabled: false,
    source: '用户运营配置',
    maxRank: 0
  };
}
