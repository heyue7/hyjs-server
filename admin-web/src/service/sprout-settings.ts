/** 全局小芽规则：按角色创建后的天数判定。 */
export interface SproutSettings {
  enabled: boolean;
  newCharacterEnabled: boolean | null;
  returnEnabled: boolean | null;
  returnOfflineDays: number | null;
  returnDays: number | null;
  days: number | null;
  icon: string;
  version: string;
}
