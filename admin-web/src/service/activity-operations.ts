import type { ResourceAmount as Reward } from './operations';
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
  defaultOptions?: Record<string, any>;
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
  options: Record<string, any>;
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
export const cumulativeModes = ['recharge-total', 'diamond-spent', 'recruit-count', 'copy-win', 'login-total'];
export function activityRepeatPolicy(form: Pick<ActivityForm, 'mode' | 'repeatPolicy'>): 'once' | 'repeat' {
  return form.repeatPolicy || (form.mode === 'opening-14' ? 'once' : 'repeat');
}
export function activityRepeatLabel(form: Pick<ActivityForm, 'mode' | 'repeatPolicy'>): string {
  return activityRepeatPolicy(form) === 'once' ? '一次性' : '可重复';
}
export function newActivityForm(mode: ActivityMode): ActivityForm {
  if (mode.defaultForm) {
    const form: ActivityForm = JSON.parse(JSON.stringify(mode.defaultForm));
    form.repeatPolicy = activityRepeatPolicy(form);
    return form;
  }
  return {
    mode: mode.mode,
    repeatPolicy: mode.mode === 'opening-14' ? 'once' : 'repeat',
    templateId: mode.templates[0]?.id || '',
    name: '',
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
  };
}
