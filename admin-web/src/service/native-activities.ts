import { ApiError, api, json } from './operations';
import type { ResourceAmount } from './operations';
import type { ActivityForm, ActivityPrize } from './activity-operations';
import type { RecruitSettingsSaveRequest } from './recruit-settings';

export interface NativeMilestone { key: string; goal: number; rewards: ResourceAmount[] }
export interface NativeVIPTier { vip: number; dailyRewards: ResourceAmount[]; weekCost: ResourceAmount; weekRewards: ResourceAmount[] }
export interface NativeActivityChild { id: string; domain: string; form: ActivityForm }
export interface NativeActivityForm {
  startAt: number; endAt: number; claimEndAt: number;
  minVip: number; diamondCost: number; threshold: number;
  cents: number; charge: number; dailyDiamond: number;
  weekdayMask: number; startMinute: number; endMinute: number; currencyFactor: number; dropFactor: number;
  rewards: ResourceAmount[]; milestones: NativeMilestone[];
  /** 精灵扭蛋使用固定精灵名单，只有每项的 weight 可以编辑。 */
  prizes: ActivityPrize[];
  vipTiers: NativeVIPTier[]; children: NativeActivityChild[];
}
export interface NativeActivity {
  id: string; kind: string; name: string; alias: string; version: string;
  enabled: boolean; permanent: boolean; frozen: boolean; note: string; form: NativeActivityForm;
}
export const getNativeActivities = () => api<{items: NativeActivity[]}>('/activity-native-settings');
export const saveNativeActivity = (row: NativeActivity, recruitSettings?: RecruitSettingsSaveRequest) => api<NativeActivity>(
  `/activity-native-settings/${encodeURIComponent(row.id)}`,
  json('PUT', {
    version: row.version,
    enabled: row.enabled,
    alias: row.alias,
    form: row.form,
    ...(recruitSettings ? { recruitSettings } : {})
  })
);
export async function removeNativeActivity(row: NativeActivity) {
  try {
    return await api<unknown>(
      `/activity-native-settings/${encodeURIComponent(row.id)}`,
      json('DELETE', { version: row.version })
    );
  } catch (error) {
    // 后端返回 204 空响应时同样是成功，普通成功 data 由上面的 api 处理。
    if (error instanceof ApiError && error.status === 204) return;
    throw error;
  }
}
