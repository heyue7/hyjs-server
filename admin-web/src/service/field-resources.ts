import { api, json } from './operations';

export interface FieldResources { bossDailyRewards: number; version: string }
export const getFieldResources = () => api<FieldResources>('/field-resources');
export const saveFieldResources = (request: FieldResources & { reason: string }) =>
  api<FieldResources>('/field-resources', json('PUT', request));
