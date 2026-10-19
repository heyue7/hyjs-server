import { api, json } from './operations';
import type { ResourceAmount } from './operations';

export interface BroadcastBatch {
  id: string; serverId: number; serverName: string; actorName: string;
  title: string; body: string; rewards: ResourceAmount[]; reason: string;
  recipientCount: number; excludedCount: number; successCount: number;
  createdAt: number; expiresAt: number; sentAt: number; version: string;
}
export interface BroadcastDraft {
  title: string; body: string; rewards: ResourceAmount[]; reason: string;
}
export const getBroadcasts = (serverId: number, page: number, pageSize: number) =>
  api<{ items: BroadcastBatch[]; total: number }>(`/broadcast?serverId=${serverId}&page=${page}&pageSize=${pageSize}`);
export const previewBroadcast = (serverId: number, requestId: string, draft: BroadcastDraft) =>
  api<{ batch: BroadcastBatch }>(`/broadcast/preview?serverId=${serverId}`, json('POST', { ...draft, requestId }));
export const sendBroadcast = (serverId: number, batch: BroadcastBatch, requestId: string) =>
  api<{ batch: BroadcastBatch }>(`/broadcast/${encodeURIComponent(batch.id)}/send?serverId=${serverId}`, json('POST', { requestId, version: batch.version }));
