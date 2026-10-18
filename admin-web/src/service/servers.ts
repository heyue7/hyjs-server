import { api, json } from './operations';

export interface GameServer {
  id: number;
  name: string;
  enabled: boolean;
  openAt: number;
  version: string;
  runtimeBound: boolean;
}
export interface ServerDraft {
  name: string;
  enabled: boolean;
  openAt: number;
}
export const getServers = () => api<{ items: GameServer[] }>('/servers');
export const createServer = (draft: Pick<ServerDraft, 'name' | 'openAt'>) =>
  api<GameServer>('/servers', json('POST', draft));
export const updateServer = (id: number, draft: ServerDraft & { version: string }) =>
  api<GameServer>(`/servers/${id}`, json('PUT', draft));
