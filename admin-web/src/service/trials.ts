import { api, json } from './operations';

export interface TrialAnchor { floor: number; hp: number; att: number; def: number }
export interface TrialGeneration {
  floors: number;
  anchors: TrialAnchor[];
  difficultyFactors: number[];
  roster: string[];
  positions: number[];
  initialCount: number;
  countEveryFloors: number;
  maxCount: number;
}
export interface TrialConfig {
  enabled: boolean;
  source: string;
  version: number;
  publishedAt: number;
  publishedFloorCount: number;
  config: TrialGeneration;
}
export interface TrialEnemy { id: string; name: string; pose: number; hp: number; att: number; def: number }
export interface TrialPreview { floor: number; star: number; recommendedPower: number; enemies: TrialEnemy[] }
export const getTrialConfig = () => api<TrialConfig>('/trials/config');
export const previewTrial = (config: TrialGeneration, floor: number) =>
  api<{ items: TrialPreview[] }>('/trials/preview', json('POST', { config, floor }));
export const saveTrialConfig = (request: { expectedVersion: number; enabled: boolean; config: TrialGeneration; reason: string }) =>
  api<TrialConfig>('/trials/config', json('PUT', request));
