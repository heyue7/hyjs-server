export interface GamePlayer {
  accountId: number;
  username: string;
  nickname: string;
  id: number;
  name: string;
  level: number;
  vip: number;
  chapter: number;
  mission: number;
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
