import { api, json } from './operations';
import type { GuildScheduleRow, GuildSchedules, GuildScheduleSave } from './guild-schedules.types';
export const getGuildSchedules = () => api<GuildSchedules>('/guild-schedules');
export const saveGuildSchedule = (body: GuildScheduleSave, create: boolean) => api<GuildScheduleRow>(`/guild-schedules${create ? '' : `/${encodeURIComponent(body.day)}`}`, json(create ? 'POST' : 'PUT', body));
export const saveGuildCalendar = (enabled: boolean, rowVersion: string, reason: string) => api<GuildSchedules>('/guild-schedules/calendar', json('PUT', { enabled, rowVersion, reason }));
export function emptyGuildSchedule(): GuildScheduleRow {
  return { day: '', enrollmentStart: '', enrollmentEnd: '', firstStartTime: '20:00', secondStartTime: '21:00', durationMinutes: 30, enabled: false, version: '', registrationDay: '', firstEnd: '', secondEnd: '', publishedAt: 0, rowVersion: '', frozen: false, editable: true, legacy: false };
}
