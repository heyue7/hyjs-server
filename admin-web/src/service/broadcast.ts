import { api, json } from './operations';
import type { ResourceAmount } from './operations';

/** Mode all means every eligible player across pages; players means the explicit cross-page checked set. */
export type AudienceMode = 'all' | 'players';
export interface BroadcastAudience {
  mode: AudienceMode; playerIds: number[];
}
export interface BroadcastBatch {
  id: string; serverId: number; serverName: string; actorName: string;
  audienceMode: AudienceMode; title: string; body: string; rewards: ResourceAmount[]; reason: string;
  recipientCount: number; excludedCount: number; successCount: number;
  createdAt: number; expiresAt: number; sentAt: number; version: string;
}
export interface BroadcastDraft {
  audience: BroadcastAudience; title: string; body: string; rewards: ResourceAmount[]; reason: string;
}
export interface MailBatch {
  id: string; serverId: number; serverName: string; actorName: string;
  audienceMode: AudienceMode; title: string; body: string; reason: string;
  recipientCount: number; excludedCount: number; successCount: number;
  createdAt: number; expiresAt: number; sentAt: number; version: string;
}
export interface MailDraft {
  audience: BroadcastAudience; title: string; body: string; reason: string;
}
export const getBroadcasts = (serverId: number, page: number, pageSize: number) =>
  api<{ items: BroadcastBatch[]; total: number }>(`/broadcast?serverId=${serverId}&page=${page}&pageSize=${pageSize}`);
export const previewBroadcast = (serverId: number, requestId: string, draft: BroadcastDraft) =>
  api<{ batch: BroadcastBatch }>(`/broadcast/preview?serverId=${serverId}`, json('POST', { ...draft, requestId }));
export const sendBroadcast = (serverId: number, batch: BroadcastBatch, requestId: string) =>
  api<{ batch: BroadcastBatch }>(`/broadcast/${encodeURIComponent(batch.id)}/send?serverId=${serverId}`, json('POST', { requestId, version: batch.version }));
export const getMails = (serverId: number, page: number, pageSize: number) =>
  api<{ items: MailBatch[]; total: number }>(`/mail?serverId=${serverId}&page=${page}&pageSize=${pageSize}`);
export const previewMail = (serverId: number, requestId: string, draft: MailDraft) =>
  api<{ batch: MailBatch }>(`/mail/preview?serverId=${serverId}`, json('POST', { ...draft, requestId }));
export const sendMail = (serverId: number, batch: MailBatch, requestId: string) =>
  api<{ batch: MailBatch }>(`/mail/${encodeURIComponent(batch.id)}/send?serverId=${serverId}`, json('POST', { requestId, version: batch.version }));
