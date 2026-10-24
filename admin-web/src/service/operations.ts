export interface AdminUser {
  username: string;
  role: 'viewer' | 'operator' | 'admin';
}
export interface Session {
  user: AdminUser;
  csrfToken: string;
}
export interface Item {
  id: string;
  name: string;
  category: string;
  quality?: string | number;
  description?: string;
  icon?: string;
  iconUrl?: string;
  previewUrl?: string;
  qualityLabel?: string;
  qualityColor?: 'white' | 'green' | 'blue' | 'purple' | 'orange' | 'red' | 'gray';
  applicableType?: string;
  applicableTypes?: string[];
  applicableTypeLabel?: string;
  subtype?: string;
  subtypeLabel?: string;
}
export interface FilterOption {
  value: string | number;
  label: string;
  count?: number;
}
export interface ItemFacets {
  categories: FilterOption[];
  qualities: FilterOption[];
  applicableTypes: FilterOption[];
  subtypes?: FilterOption[];
}
export interface ItemsResponse {
  items: Item[];
  total: number;
  categories?: string[];
  facets?: ItemFacets;
}
export function resourceImage(item?: Item | null, preview = false) {
  const value = (preview && item?.previewUrl) || item?.iconUrl || item?.icon || '';
  return /^(\/admin\/|https:\/\/)/.test(value) ? value : '';
}
export interface Activity {
  id: string;
  name: string;
  type: string;
  enabled: boolean;
  configured: boolean;
  source: string;
  version: string;
  config: Record<string, any>;
  editable?: boolean;
}
export interface Announcement {
  id: string | number;
  title: string;
  content: string;
  enabled: boolean;
  startAt: number;
  endAt: number;
  version: string;
}
export interface Audit {
  id: string | number;
  time: number | string;
  actor: string;
  action: string;
  target: string;
  detail: unknown;
}
let csrfToken = '';
export function setCsrfToken(value: string) {
  csrfToken = value || '';
}
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public code?: number | string
  ) {
    super(message);
  }
}
export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const method = (options.method || 'GET').toUpperCase();
  const response = await fetch(`/admin/api${path}`, {
    ...options,
    credentials: 'same-origin',
    headers: {
      Accept: 'application/json',
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      ...(!['GET', 'HEAD'].includes(method) && csrfToken ? { 'X-CSRF-Token': csrfToken } : {}),
      ...options.headers
    }
  });
  let result: { code: number | string; msg?: string; message?: string; data: T };
  try {
    result = await response.json();
  } catch {
    throw new ApiError('服务返回异常，请稍后重试', response.status);
  }
  if (!response.ok || String(result.code) !== '0') {
    if (response.status === 401 && path !== '/auth/login' && path !== '/auth/me')
      window.dispatchEvent(new Event('hyjs-session-expired'));
    throw new ApiError(result.msg || result.message || `请求失败（${response.status}）`, response.status, result.code);
  }
  return result.data;
}
export const json = (method: string, body: unknown): RequestInit => ({ method, body: JSON.stringify(body) });
export function errorMessage(error: unknown) {
  if (error instanceof TypeError && error.message === 'Failed to fetch')
    return '网络连接异常，未能确认操作结果。请检查连接后重试。';
  return error instanceof Error ? error.message : '操作失败，请重试';
}
export function formatTime(value: number | string | undefined) {
  if (!value) return '—';
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? String(value)
    : date.toLocaleString('zh-CN', { timeZone: 'Asia/Shanghai', hour12: false });
}
export function resourceSummary(config: Record<string, any>) {
  if (Array.isArray(config.rewards)) return config.rewards.length ? `${config.rewards.length} 项奖励` : '奖励待配置';
  if (Array.isArray(config.rules?.products))
    return config.rules.products.length ? `${config.rules.products.length} 件商品` : '商品待配置';
  if (typeof config.rewards === 'string') return config.rewards || '奖励待配置';
  return Object.keys(config).length ? '查看配置' : '待配置';
}

const activityNames: Record<string, string> = {
  EveryDaySign: '每日签到',
  OpenServerRace: '开服竞赛',
  AsunaKitchen: '亚丝娜厨房',
  LoginAward: '登录奖励',
  RechargeSurprise: '充值惊喜',
  FirstRecharge: '首充奖励',
  MonthCard: '月卡',
  VIPBenefits: '会员福利',
  BattleRace: '竞技活动',
  RobTrea: '夺宝活动',
  PrizeTask: '悬赏任务',
  PrizeBoss: '悬赏首领',
  CopyBenefits: '副本福利',
  MainCopy: '主线活动',
  CostTotal: '累计消费',
  RechargeTotal: '累计充值',
  DiscountSale: '折扣商店',
  HalfDiscountSale: '半价特惠',
  OpenSeverMoney: '开服基金',
  AincradStars: '艾恩葛朗特之星',
  Congratulation: '庆典活动',
  OpenHappy: '开服狂欢',
  TimeLimitExchange: '限时兑换',
  TimeLimitSail: '限时商店',
  Festival: '节日活动',
  Update: '更新福利',
  fortuneCat: '招财猫',
  continueRechargeGift: '连续充值',
  WarriorRescruitTimes: '累计招募',
  CollectionWarrior: '角色收集',
  WarriorWeightImprove: '强者招募轮换',
  KaiGongActivity: '开工活动',
  RechangeDouble: '首笔充值双倍',
  BigTurnTable: '幸运大转盘',
  CostReturn: '消费返还',
  MysticDiscount: '神秘折扣'
};
export function activityName(row: Activity) {
  return activityNames[row.name] || activityNames[row.config?.kind] || row.name || row.id;
}

export interface ResourceAmount {
  resourceId: string;
  amount: number;
}
