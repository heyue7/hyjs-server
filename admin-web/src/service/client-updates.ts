import { api, json } from './operations';

export type UpdateMode = 'notice' | 'next_login' | 'maintenance';
export type UpdateAction = 'publish' | 'remind' | 'withdraw' | 'start_maintenance' | 'end_maintenance';
export interface ClientRelease {
  id: number; buildCode: number; versionName: string; title: string; content: string;
  mode: UpdateMode; minLoginBuild: number; minRunBuild: number;
  maintenanceStartAt: number; maintenanceEndAt: number;
  state: 'draft' | 'published' | 'withdrawn'; publishedAt: number; updatedAt: number; publishedBy: string;
}
export interface ClientPolicy {
  releaseId: number; latestBuild: number; minLoginBuild: number; minRunBuild: number;
  revision: number; noticeBatch: number; noticeEnabled: boolean; maintenance: boolean;
  maintenanceStartAt: number; maintenanceEndAt: number; versionName: string; title: string; content: string; mode: UpdateMode;
}
export interface ClientUpdateOverview {
  version: string; policy: ClientPolicy; items: ClientRelease[];
  stats: { online: number; older: number; unknown: number; shown: number; acknowledged: number; pending: number; observedAt: number };
  canEdit: boolean; canPublish: boolean;
}
export interface ClientUpdateDraft {
  id: number; version: string; buildCode: number; versionName: string; title: string; content: string;
  mode: UpdateMode; minLoginBuild: number; minRunBuild: number; maintenanceStartAt: number; maintenanceEndAt: number; reason: string;
}
export interface ClientUpdateCommand { id: number; version: string; requestId: string; action: UpdateAction; reason: string }
export const getClientUpdates = () => api<ClientUpdateOverview>('/client-updates');
export const saveClientUpdate = (draft: ClientUpdateDraft) => api<ClientUpdateOverview>('/client-updates/draft', json('POST', draft));
export const actClientUpdate = (command: ClientUpdateCommand) => api<ClientUpdateOverview>('/client-updates/action', json('POST', command));
export const getClientUpdateHistory = (page = 1, pageSize = 20) => api<{ items: ClientRelease[]; total: number }>(`/client-updates/history?page=${page}&pageSize=${pageSize}`);
