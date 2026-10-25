import { api, json } from './operations';
import type { ResourceAmount } from './operations';

export interface DailyTaskRow {
  id: string;
  name: string;
  type: number;
  total: number;
  score: number;
  minLevel: number;
  enabled: boolean;
  reward: ResourceAmount;
}
export interface DailyTaskRules { version: string; items: DailyTaskRow[] }
export interface DailyTaskSaveRequest {
  version: string;
  reason: string;
  items: { id: string; reward: ResourceAmount }[];
}
export const getDailyTasks = () => api<DailyTaskRules>('/daily-tasks');
export const saveDailyTasks = (request: DailyTaskSaveRequest) =>
  api<DailyTaskRules>('/daily-tasks', json('PUT', request));
