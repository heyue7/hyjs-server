import { api, json } from './operations';
import type { ResourceAmount } from './operations';

export interface LuxurySignRules {
  version: string;
  enabled: boolean;
  rewards: ResourceAmount[];
  rechargeCents: number;
  startAt: number;
  endAt: number;
}
export type LuxurySignSaveRequest = Pick<LuxurySignRules, 'version' | 'enabled' | 'rewards'>;
export const getLuxurySignRules = () => api<LuxurySignRules>('/luxury-sign-rules');
export const saveLuxurySignRules = (request: LuxurySignSaveRequest) => api<LuxurySignRules>('/luxury-sign-rules', json('PUT', request));
