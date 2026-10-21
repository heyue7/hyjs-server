import { api } from './operations';

export interface OnlineServer { serverId: number; name: string; online: number; connected: boolean }
export interface OnlinePlayer { id: number; serverId: number; name: string; level: number; vip: number }
export interface OnlinePlayersResponse { servers: OnlineServer[]; items: OnlinePlayer[]; total: number; observedAt: number }
// 此接口独立选择区服，保留全区服统计，不跟随页眉选择。
export function fetchOnlinePlayers(serverId: number | null, q: string, page: number, pageSize: number) {
  const query = new URLSearchParams({ q, page: String(page), pageSize: String(pageSize) });
  if (serverId) query.set('serverId', String(serverId));
  return api<OnlinePlayersResponse>(`/online-players?${query}`);
}
