import { api, json } from './operations';
import type { ActivityForm } from './activity-operations';

export interface PersonalOpeningActivity {
  id: string;
  name: string;
  mode: string;
  startDay: number;
  durationDays: number;
  claimDays: number;
  enabled: boolean;
  version: string;
  form: ActivityForm;
}
export const getPersonalOpeningActivities = (serverId: number) =>
  api<{ items: PersonalOpeningActivity[] }>(`/personal-opening-activities?serverId=${serverId}`);
export const savePersonalOpeningActivity = (serverId: number, row: PersonalOpeningActivity) =>
  api<PersonalOpeningActivity>(`/personal-opening-activities/${encodeURIComponent(row.id)}?serverId=${serverId}`, json('PUT', row));
