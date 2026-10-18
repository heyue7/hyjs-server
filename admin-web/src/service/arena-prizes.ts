export interface ArenaPrizeItem {
  resourceId: string;
  amount: number;
  weight: number;
}

export interface ArenaOperatorPool {
  enabled: boolean;
  startAt: number;
  endAt: number;
  minLevel: number;
  dailyWins: number;
  windowWins: number;
  items: ArenaPrizeItem[];
}

export interface ArenaPrizeSnapshot {
  version: string;
  regularCoin: { min: number; max: number };
  operatorPool: ArenaOperatorPool;
  operatorPoolFrozen: boolean;
  notes: string;
  allowedResourceIds: string[];
}

export interface ArenaPrizeSaveRequest {
  version: string;
  regularCoin: ArenaPrizeSnapshot['regularCoin'];
  operatorPool?: ArenaOperatorPool;
}
