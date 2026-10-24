import { api, json } from './operations';
import type { ResourceAmount } from './operations';

export interface DailySignRow {
  day: number;
  reward: ResourceAmount[];
  doubleVip: number;
}

export interface DailySignRules {
  month: string;
  configured: boolean;
  daysInMonth: number;
  version: string;
  enabled: boolean;
  source: string;
  rows: DailySignRow[];
}

export interface DailySignSaveRequest {
  month: string;
  version: string;
  enabled: boolean;
  rows: DailySignRow[];
}

export const getDailySignRules = (month?: string) => api<DailySignRules>(`/month-sign-rules${month ? `?${new URLSearchParams({ month })}` : ''}`);
export const saveDailySignRules = (request: DailySignSaveRequest) =>
  api<DailySignRules>('/month-sign-rules', json('PUT', request));
