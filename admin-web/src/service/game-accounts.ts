import { api, json } from './operations';

export interface AccountPlayer {
  serverId: number;
  playerId: number;
  name: string;
  level: number;
}
export interface GameAccount {
  id: number;
  username: string;
  nickname: string;
  protected: boolean;
  enabled: boolean;
  createdAt: number | string;
  version: string;
  players: AccountPlayer[];
}
export const getGameAccounts = (q: string, page = 1, pageSize = 25) =>
  api<{ items: GameAccount[]; total: number }>(
    `/game-accounts?${new URLSearchParams({ q, page: String(page), pageSize: String(pageSize) })}`
  );
export const createGameAccount = (draft: {
  username: string;
  password: string;
  confirmPassword: string;
  nickname: string;
  reason: string;
}) => api<GameAccount>('/game-accounts', json('POST', draft));
export const updateGameAccount = (id: number, draft: {
  nickname: string;
  enabled: boolean;
  version: string;
  reason: string;
}) => api<GameAccount>(`/game-accounts/${id}`, json('PUT', draft));
export const resetGameAccountPassword = (id: number, draft: {
  password: string;
  confirmPassword: string;
  version: string;
  reason: string;
}) => api<GameAccount>(`/game-accounts/${id}/password`, json('PUT', draft));
