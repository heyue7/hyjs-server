import type { ActivityAvailability } from './activity-template-types';
import type { ActivityForm } from './activity-operations';

export interface ActivityDefinition {
  id: string;
  name: string;
  mode: string;
  form: ActivityForm;
  defaultDays: number;
  claimDays: number;
  enabled: boolean;
  version: string;
  availability: ActivityAvailability;
  sourceActivityId: string;
}
export interface ActivityOccurrence {
  timing?: 'calendar' | 'character-day';
  startDay?: number;
  durationDays?: number;
  claimDays?: number;
  id: string;
  templateId: string;
  templateVersion: string;
  name: string;
  mode: string;
  startAt: number;
  endAt: number;
  claimEndAt: number;
  lane: number;
  enabled: boolean;
  frozen: boolean;
  version: string;
  consumerId: string;
  form?: ActivityForm;
  draftKey?: string;
}
export interface ActivitySchedule {
  serverId: number;
  managed: boolean;
  gameTimeMs: number;
  version: string;
  items: ActivityOccurrence[];
  warnings: string[];
}
export interface ActivityPatternEntry {
  templateId: string;
  startDay: number;
  durationDays: number;
  claimDays: number;
  lane: number;
}
export interface ActivityPattern {
  id: string;
  name: string;
  cycleDays: number;
  entries: ActivityPatternEntry[];
  version: string;
}
export interface SchedulePreview {
  valid: boolean;
  conflicts: { index: number; otherIndex: number; message: string }[];
  warnings: string[];
}
export const activityDay = 86_400_000;
export function calendarMidnight(value: number): number {
  return Math.floor((value + 8 * 3_600_000) / activityDay) * activityDay - 8 * 3_600_000;
}
export function calendarDate(value: number): string {
  return new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit' }).format(
    value
  );
}
export function expandActivityPattern(
  pattern: ActivityPattern,
  definitions: ActivityDefinition[],
  options: { anchor: number; repeats: number }
): ActivityOccurrence[] {
  const { anchor, repeats } = options;
  if (repeats > 1 && pattern.entries.some(entry => definitions.find(definition => definition.id === entry.templateId)?.mode === 'opening-14')) throw new Error('开服任务按角色独立起算，只需安排一次，无需循环');
  if (pattern.entries.filter(entry => definitions.find(definition => definition.id === entry.templateId)?.mode === 'opening-14').length > 1) throw new Error('同一方案只需安排一次角色开服任务');
  const lookup = new Map(definitions.map(item => [item.id, item]));
  const out: ActivityOccurrence[] = [];
  for (let repeat = 0; repeat < repeats; repeat += 1) {
    for (const [index, entry] of pattern.entries.entries()) {
      const definition = lookup.get(entry.templateId);
      if (!definition || !definition.enabled || !definition.availability.usable) {
        throw new Error('方案中有停用或缺少配置的活动模板，请先修正全局活动设置');
      }
      const startAt = calendarMidnight(anchor) + (repeat * pattern.cycleDays + entry.startDay) * activityDay;
      const endAt = startAt + entry.durationDays * activityDay;
      out.push({
        id: '',
        timing: definition.mode === 'opening-14' ? 'character-day' : 'calendar',
        templateId: entry.templateId,
        templateVersion: definition.version,
        name: definition.name,
        mode: definition.mode,
        startAt,
        endAt,
        claimEndAt: endAt + entry.claimDays * activityDay,
        lane: entry.lane,
        enabled: true,
        frozen: false,
        version: '',
        consumerId: '',
        draftKey: `pattern-${Date.now()}-${repeat}-${index}-${Math.random().toString(36).slice(2)}`
      });
    }
  }
  return out;
}
