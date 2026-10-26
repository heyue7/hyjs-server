export interface GuildScheduleForm {
  day: string;
  enrollmentStart: string;
  enrollmentEnd: string;
  firstStartTime: string;
  secondStartTime: string;
  durationMinutes: number;
  enabled: boolean;
  version: string;
}
export interface GuildScheduleRow extends GuildScheduleForm {
  registrationDay: string;
  firstEnd: string;
  secondEnd: string;
  publishedAt: number;
  rowVersion: string;
  frozen: boolean;
  editable: boolean;
  legacy: boolean;
  publicationNotes?: string[];
}
export interface GuildSchedules {
  items: GuildScheduleRow[];
  customCalendarEnabled: boolean;
  policyVersion: string;
  preparationMinutes: number;
}
export interface GuildScheduleSave extends GuildScheduleForm { rowVersion: string; reason: string }
