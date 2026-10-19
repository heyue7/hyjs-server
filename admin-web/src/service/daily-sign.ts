import { api, json } from './operations';
import type { ResourceAmount } from './operations';

export interface DailySignRow {
  day: number;
  reward: ResourceAmount[];
  doubleVip: number;
}

export interface DailySignRules {
  version: string;
  enabled: boolean;
  source: string;
  rows: DailySignRow[];
}

export interface DailySignSaveRequest {
  version: string;
  enabled: boolean;
  rows: DailySignRow[];
}

export const getDailySignRules = () => api<DailySignRules>('/month-sign-rules');
export const saveDailySignRules = (request: DailySignSaveRequest) =>
  api<DailySignRules>('/month-sign-rules', json('PUT', request));
