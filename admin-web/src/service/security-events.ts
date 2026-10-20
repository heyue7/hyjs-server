import { api } from './operations';

export type SecurityEventOutcome = '' | 'success' | 'failure' | 'denied' | 'throttled';

export interface SecurityEvent {
  id: number;
  firstAt: number;
  lastAt: number;
  actor: string;
  sourceIp: string;
  action: string;
  target: string;
  outcome: SecurityEventOutcome;
  reasonCode: string;
  requestId: string;
  occurrences: number;
}

export interface SecurityEventFilters {
  action: string;
  outcome: SecurityEventOutcome;
  actor: string;
}

export const securityEventOutcomeOptions: { label: string; value: SecurityEventOutcome }[] = [
  { label: '全部结果', value: '' },
  { label: '成功', value: 'success' },
  { label: '失败', value: 'failure' },
  { label: '拒绝', value: 'denied' },
  { label: '限流', value: 'throttled' }
];

export function securityEventOutcomeLabel(outcome: SecurityEventOutcome) {
  if (!outcome) return '未知';
  return securityEventOutcomeOptions.find(option => option.value === outcome)?.label || '未知';
}

export function getSecurityEvents(page: number, pageSize: number, filters: SecurityEventFilters) {
  const params = new URLSearchParams({
    page: String(page),
    pageSize: String(pageSize)
  });
  if (filters.action.trim()) params.set('action', filters.action.trim());
  if (filters.outcome) params.set('outcome', filters.outcome);
  if (filters.actor.trim()) params.set('actor', filters.actor.trim());
  return api<{ items: SecurityEvent[]; total: number }>(`/security-events?${params.toString()}`);
}
