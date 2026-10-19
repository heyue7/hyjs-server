import { api, json } from './operations';
import type { ResourceAmount } from './operations';
export interface CDK {
  id: string; name: string; codeHint: string; enabled: boolean; unlimited: boolean;
  startAt: number; endAt: number; totalLimit: number; used: number;
  rewards: ResourceAmount[]; version: string; createdAt: number | string;
}
export interface CDKDraft {
  name: string; rewards: ResourceAmount[]; count: number; prefix?: string; code?: string;
  totalLimit: number; unlimited: boolean; startAt: number; endAt: number; enabled: boolean; reason: string;
}
export interface CDKClaim { playerId: number; playerName: string; claimedAt: number | string }
const scoped = (path: string, serverId: number) => `${path}${path.includes('?') ? '&' : '?'}serverId=${serverId}`;
export const getCDKs = (serverId: number, q: string, page: number, pageSize: number) =>
  api<{ items: CDK[]; total: number }>(scoped(`/cdk?${new URLSearchParams({ q, page: String(page), pageSize: String(pageSize) })}`, serverId));
export const createCDKs = (serverId: number, draft: CDKDraft) => api<{ items: CDK[]; codes: string[] }>(scoped('/cdk', serverId), json('POST', draft));
export const updateCDK = (serverId: number, id: string, draft: Omit<CDKDraft, 'count' | 'prefix' | 'code'> & { version: string }) =>
  api<CDK>(scoped(`/cdk/${encodeURIComponent(id)}`, serverId), json('PUT', draft));
export const getCDKClaims = (serverId: number, id: string, page: number, pageSize: number) =>
  api<{ items: CDKClaim[]; total: number }>(scoped(`/cdk/${encodeURIComponent(id)}/claims?page=${page}&pageSize=${pageSize}`, serverId));
