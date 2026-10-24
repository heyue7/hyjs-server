import { api, json } from './operations';
import type { Item, ResourceAmount as Reward } from './operations';

export function deleteActivityTemplate(id: string, version: string) {
  return api<{ ok: boolean; archived: boolean }>(`/activity-templates/${encodeURIComponent(id)}`, json('DELETE', { version }));
}
export function deleteActivityInstance(id: string, version: string) {
  return api<{ ok: boolean; archived: boolean }>(`/activity-instances/${encodeURIComponent(id)}`, json('DELETE', { version }));
}

export interface TurntablePityRule {
  prizeId: number; enabled: boolean; hardPity: number; softStart: number; basePPM: number; softMaxPPM: number;
}
export interface ActivityGameplayOptions extends Record<string, any> {
  prizePityRules?: TurntablePityRule[];
  dailySpend?: number;
  rankPlaces?: number;
  rankRewards?: Reward[];
  /** 服务端冻结的原王者池快照，不提供运营编辑入口。 */
  kingPoolSource?: string;
  kingWorld?: number;
}
export interface KingRecruitResource extends Item {
  space: number;
  spaceLabel: string;
}
export interface ActivityTemplate {
  id: string;
  name: string;
  prefab: string;
  preview?: string;
}
export interface ActivityMode {
  mode: string;
  label: string;
  templates: ActivityTemplate[];
  conditions: { key: string; label: string; type: string; options?: { label: string; value: number | string }[] }[];
  supportsProducts: boolean;
  supportsMilestones: boolean;
  supportsPrizes: boolean;
  maxMilestones?: number;
  notes?: string;
  defaultOptions?: ActivityGameplayOptions;
  defaultForm?: ActivityForm;
  requiredFields?: string[];
  fixedValues?: Record<string, number | string>;
}
export interface ActivityProduct {
  id: string;
  name: string;
  rewards: Reward[];
  cost: Reward;
  dailyLimit: number;
  totalLimit: number;
  stock: number;
  minLevel: number;
  minVip: number;
  discountBps: number;
  weight: number;
  index: number;
}
export interface ActivityMilestone {
  goal: number;
  rewards: Reward[];
  options?: Reward[];
}
export interface ActivityPrize {
  id: number;
  resourceId: string;
  amount: number;
  weight: number;
  stock: number;
  guaranteed: boolean;
}
export interface ActivityForm {
  /** 后台别名只用于运营区分，不映射到游戏内活动名称。 */
  alias?: string;
  mode: string;
  repeatPolicy?: 'once' | 'repeat';
  templateId: string;
  name: string;
  source: string;
  startAt: number;
  endAt: number;
  claimEndAt: number;
  products: ActivityProduct[];
  milestones: ActivityMilestone[];
  prizes: ActivityPrize[];
  options: ActivityGameplayOptions;
}
export interface ActivityInstance {
  id: string;
  name: string;
  mode: string;
  templateId: string;
  enabled: boolean;
  version: string;
  frozen: boolean;
  editable: boolean;
  form: ActivityForm;
  availability: { usable: boolean; reason: string; missing: string[] };
}
export const productModes = ['exchange', 'discount-shop', 'half-price', 'limited-shop', 'mystic-shop'];
export const cumulativeModes = ['recharge-total', 'diamond-spent', 'recruit-count', 'copy-win', 'login-total', 'arena-wins', 'plunder-count', 'hero-collection'];
/** Format only the two known backend defaults; custom and unknown notes stay intact. */
export function formatActivityModeNotes(mode?: Pick<ActivityMode, 'mode' | 'notes'>): string {
  const notes = mode?.notes || '';
  const base = '只修改今后未冻结的新一期；空奖品草稿保持关闭。';
  const periodDefault = `${base} 每实例支持最多31档独立目标；累计充值目标单位为分。`;
  const periodModes = [
    'recharge-total', 'single-spend', 'work', 'diamond-spent', 'recruit-count',
    'copy-win', 'arena-wins', 'plunder-count', 'wanted-attempts', 'wanted-kills',
    'hero-collection', 'trial-stars', 'power-race', 'login-total'
  ];
  if (notes === periodDefault && periodModes.includes(mode?.mode || '')) {
    return `每期最多配置 31 档独立目标。${mode?.mode === 'recharge-total' ? '充值目标以人民币分填写。' : ''}未配置奖励的草稿保持关闭。`;
  }
  const baseModes = [
    ...productModes, 'login-rewards', 'limited-sign', 'stamina-meal',
    'diamond-rebate', 'treasure-box'
  ];
  if (notes === base && baseModes.includes(mode?.mode || '')) return '未配置奖励的草稿保持关闭。';
  return notes;
}
const milestoneTargets: Record<string, { label: string; unit: string; note?: string }> = {
  'recharge-total': { label: '累计充值', unit: '分' },
  'diamond-spent': { label: '累计消耗钻石', unit: '钻' },
  'recruit-count': { label: '累计招募', unit: '次' },
  'copy-win': { label: '累计主线胜利', unit: '次' },
  'login-total': { label: '累计登录', unit: '天' },
  'arena-wins': { label: '累计竞技胜利', unit: '次' },
  'plunder-count': { label: '累计夺宝', unit: '次' },
  'hero-collection': { label: '不同英雄数量', unit: '种', note: '相同英雄重复获得不重复计数。' },
  'trial-stars': { label: '试炼星数峰值', unit: '星', note: '按本期试炼星数最高值判断，多轮结果不相加。' },
  'single-spend': { label: '累计消费钻石', unit: '钻', note: '按活动期内已实际消费的钻石累计。' },
  'work': { label: '主线胜利次数', unit: '次' },
  'wanted-attempts': { label: '悬赏挑战次数', unit: '次' },
  'wanted-kills': { label: '悬赏击杀次数', unit: '次' },
  'fortune-cat': { label: '消耗钻石', unit: '钻石' },
  'continued-spend': { label: '累计达标消费日', unit: '天', note: '奖励档目标依次为 1 至 7 天。' },
  'power-race': { label: '战力峰值', unit: '战力', note: '按本期战力最高值判断达成目标与排名。' }
};
export function activityMilestoneTarget(mode?: string): { label: string; unit: string; note?: string } {
  return milestoneTargets[mode || ''] || { label: '达成目标', unit: '' };
}
export function activityMilestoneTitle(mode: string | undefined, goal: number, dayBased = false) {
  const value = Number.isFinite(goal) ? goal.toLocaleString() : '未填写';
  if (dayBased) return `第 ${value} 天`;
  const target = activityMilestoneTarget(mode);
  return `${target.label} ${value}${target.unit ? ` ${target.unit}` : ''}`;
}
export const activityFixedLabels: Record<string, string> = {
  singleDiamond: '单抽钻石', tenDiamond: '十连钻石', singleCost: '单抽消耗', tenCost: '十连消耗',
  coinPerDraw: '每次购买金币', dayLimit: '每日次数', minLevel: '解锁等级', dailyLimit: '每日招募次数',
  freeHours: '免费招募间隔（小时）', rotationHours: '世界轮换间隔（小时）',
  guaranteeEvery: '保底间隔（抽）', luckLimit: '幸运值上限', luckPerDraw: '每抽幸运值',
  luckyGoal: '幸运值上限', luckyPerDraw: '每抽幸运值'
};
export function activityRepeatPolicy(form: Pick<ActivityForm, 'mode' | 'repeatPolicy'>): 'once' | 'repeat' {
  return form.repeatPolicy || (form.mode === 'opening-14' ? 'once' : 'repeat');
}
export function activityRepeatLabel(form: Pick<ActivityForm, 'mode' | 'repeatPolicy'>): string {
  return activityRepeatPolicy(form) === 'once' ? '一次性' : '可重复';
}
/** 游戏内标题沿用客户端固定名称，后台只通过 alias 区分运营批次。 */
export function activityClientName(
  mode?: Pick<ActivityMode, 'mode' | 'label' | 'defaultForm'> | null,
  fallback = ''
): string {
  const current = fallback.trim();
  if (!mode) return current;
  if (mode.mode === 'opening-14') return '开服狂欢';
  // 限时商店沿用各商店自己的名称，后端同样不强制改成模式标签。
  if (mode.mode === 'shop-window') return current || mode.defaultForm?.name?.trim() || mode.label?.trim() || '';
  const defaultName = mode.defaultForm?.name?.trim() || '';
  return defaultName || mode.label?.trim() || current;
}
/** 表单缺少模式信息时仍按 opening-14 的客户端名称展示。 */
export function activityFormName(
  form: Pick<ActivityForm, 'mode' | 'name'>,
  mode?: Pick<ActivityMode, 'mode' | 'label' | 'defaultForm'> | null
): string {
  if (!mode && form.mode === 'opening-14') return '开服狂欢';
  return activityClientName(mode, form.name) || form.name.trim();
}
/** 旧数据未带 alias 时把历史自定义名保留到后台别名，再把表单名称收敛到客户端名。 */
export function normalizeActivityForm(form: ActivityForm, mode?: ActivityMode | null): ActivityForm {
  const current = form.name.trim();
  const alias = (form.alias || '').trim();
  if (mode || form.mode === 'opening-14') {
    const name = activityFormName(form, mode);
    if (!alias && current && current !== name) form.alias = current;
    form.name = name;
  } else {
    form.alias = alias;
    form.name = current;
  }
  return form;
}
/** 复制活动时仅给后台别名追加标记，避免改动游戏内固定名称。 */
export function activityCopyAlias(alias?: string): string {
  const suffix = '（副本）';
  const base = (alias || '').trim();
  return base.slice(0, 80 - suffix.length) + suffix;
}
export function newActivityForm(mode: ActivityMode): ActivityForm {
  if (mode.defaultForm) {
    const form: ActivityForm = JSON.parse(JSON.stringify(mode.defaultForm));
    form.alias ||= '';
    form.repeatPolicy = activityRepeatPolicy(form);
    return normalizeActivityForm(form, mode);
  }
  const name = activityClientName(mode);
  return normalizeActivityForm({
    mode: mode.mode,
    repeatPolicy: mode.mode === 'opening-14' ? 'once' : 'repeat',
    templateId: mode.templates[0]?.id || '',
    name,
    alias: '',
    source: '用户运营配置',
    startAt: Date.now(),
    endAt: 0,
    claimEndAt: 0,
    products: [],
    milestones: [],
    prizes:
      mode.mode === 'turntable'
        ? Array.from({ length: 9 }, (_, id) => ({
            id,
            resourceId: '',
            amount: 1,
            weight: 1,
            stock: 0,
            guaranteed: false
          }))
        : [],
    options: JSON.parse(JSON.stringify(mode.defaultOptions || {}))
  }, mode);
}
