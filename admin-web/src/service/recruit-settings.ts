import { api, json } from './operations';

export interface RecruitSetting {
  kind: 2 | 3;
  name: string;
  dailyLimit: number;
  guaranteeEvery: number;
}

export interface RecruitSettings {
  version: string;
  items: RecruitSetting[];
}

export interface RecruitSettingsSaveRequest {
  version: string;
  reason: string;
  items: RecruitSetting[];
}

export const getRecruitSettings = () => api<RecruitSettings>('/recruit-settings');
export const saveRecruitSettings = (request: RecruitSettingsSaveRequest) =>
  api<RecruitSettings>('/recruit-settings', json('PUT', request));
