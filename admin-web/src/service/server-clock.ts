/** 全服游戏日期；独立于玩家、登录与真实账务时间。 */
export interface ServerClock {
  enabled: boolean;
  scope: 'server';
  serverId: number;
  gameTimeMs: number;
  realTimeMs: number;
  offsetDays: number;
  revision: number;
  timezone: string;
  testOnly: boolean;
}
export interface ServerClockChange {
  deltaDays: -1 | 1;
  revision: number;
  requestId: string;
}
