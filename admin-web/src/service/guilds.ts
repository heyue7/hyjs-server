import { api, json } from './operations';

export interface GuildConfigField {
  key: string;
  label: string;
  valueType: 'int' | 'double' | 'string';
  value: string;
  editable: boolean;
  min: number;
  max: number;
}
export interface GuildConfigGroup {
  key: string;
  label: string;
  fields: GuildConfigField[];
}
export interface GuildTableColumn {
  key: string;
  label: string;
  valueType: 'int' | 'double' | 'string';
  editable: boolean;
}
export interface GuildTable {
  columns: GuildTableColumn[];
  rows: Record<string, string>[];
}
export interface GuildGatePolicy {
  managementEnabled: boolean;
  economyEnabled: boolean;
  chatEnabled: boolean;
  source: string;
}
export interface GuildPrayerPolicy {
  enabled: boolean;
  version: string;
  source: string;
  poolJson: string;
  poolHash: string;
  publishedAt: number;
}
export interface GuildTechPolicy {
  enabled: boolean;
  version: string;
  source: string;
  directoryJson: string;
  directoryHash: string;
  publishedAt: number;
}
export interface GuildDonateBoxPolicy {
  tiers: { threshold: number; reward: string }[];
  enabled: boolean;
  version: string;
  source: string;
  publishedAt: number;
}
export interface GuildDonationBadgePolicy {
  enabled: boolean;
  version: string;
  source: string;
  coinCost: number;
  badgeAmount: number;
  dailyLimit: number;
  publishedAt: number;
}
export interface GuildUnionFightEnrollmentPolicy {
  enrollmentEnabled: boolean;
  matchEnabled: boolean;
  source: string;
}
export interface GuildUnionFightChatPolicy {
  textEnabled: boolean;
  talkEnabled: boolean;
  talkCooldownMillis: number;
  source: string;
}
export interface GuildUnionFightItemPolicy {
  enabled: boolean;
  source: string;
}
export interface GuildUnionFightFieldPolicy {
  guardHpMode?: 'uniform' | 'template';
  enabled: boolean;
  preparationMillis: number;
  leftRevivePointId: number;
  rightRevivePointId: number;
  initialGuardHp: number;
  initialGateHp: number;
  moveTolerance: number;
  source: string;
}
export interface GuildUnionFightPlayerBattlePolicy {
  id: number;
  enabled: boolean;
  version: string;
  source: string;
  publishedAt: number;
  startsAt: number;
  endsAt: number;
  reviveRule: string;
}
export interface GuildUnionFightPolicyState {
  warEnabled: boolean;
  enrollment: GuildUnionFightEnrollmentPolicy;
  chat: GuildUnionFightChatPolicy;
  item: GuildUnionFightItemPolicy;
  field: GuildUnionFightFieldPolicy;
  playerBattle: GuildUnionFightPlayerBattlePolicy[];
}
export interface GuildPolicyState {
  guild: GuildGatePolicy;
  prayer: GuildPrayerPolicy;
  tech: GuildTechPolicy;
  donateBox: GuildDonateBoxPolicy;
  donationBadge: GuildDonationBadgePolicy;
  unionFight: GuildUnionFightPolicyState;
}
export interface GuildCapability {
  key: string;
  label: string;
  implemented: boolean;
  editable: boolean;
  detail: string;
}
export interface GuildSettings {
  version: string;
  updatedAt: number;
  updatedBy: string;
  groups: GuildConfigGroup[];
  unionLevels: GuildTable;
  unionTech: GuildTable;
  policies: GuildPolicyState;
  shops: { label: string; path: string; note: string }[];
  capabilities: GuildCapability[];
}
export interface GuildSettingsSave {
  version: string;
  reason: string;
  fields: { key: string; value: string }[];
  unionLevels: Record<string, string>[];
  unionTech: Record<string, string>[];
  policies?: Partial<GuildPolicyState> & {
    unionFight?: Partial<GuildUnionFightPolicyState>;
  };
}
// Independent master switch write; excludes unrelated unsaved settings.
export interface GuildWarOpenSave {
  version: string;
  reason: string;
  policies: { unionFight: { warEnabled: boolean } };
}
export interface GuildSummary {
  id: string;
  name: string;
  level: number;
  memberCount: number;
  chairmanName: string;
  coin: number;
  badge: number;
  exp: number;
  donateScore: number;
  activeScore: number;
  createdAt: number;
  version: string;
}
export interface GuildMember {
  playerId: number;
  playerName: string;
  level: number;
  fightPower: number;
  role: 0 | 1 | 2;
  contribution: number;
  spendableContribution: number;
  todayCoin: number;
  todayBadge: number;
  joinedAt: number;
  quitUntil: number;
  lastLoginAt: number;
  donateDay: string;
  version: string;
}
export interface GuildDetail {
  id: string;
  name: string;
  level: number;
  notice: string;
  declaration: string;
  flag: number;
  coin: number;
  badge: number;
  exp: number;
  donateScore: number;
  activeScore: number;
  heroStatueOpen: number;
  guardTreeOpen: number;
  unionFightApply: boolean;
  createdAt: number;
  version: string;
  members: GuildMember[];
}
export interface GuildList {
  items: GuildSummary[];
  total: number;
  page: number;
  pageSize: number;
}
export interface GuildUpdate {
  version: string;
  reason: string;
  name: string;
  level: number;
  notice: string;
  declaration: string;
  flag: number;
  coin: number;
  badge: number;
  exp: number;
  donateScore: number;
  activeScore: number;
  heroStatueOpen: number;
  guardTreeOpen: number;
}
export interface GuildMemberUpdate {
  version: string;
  reason: string;
  contribution: number;
  spendableContribution?: number;
  expectedSpendableContribution?: number;
  todayCoin: number;
  todayBadge: number;
  role: number;
}

const withServer = (path: string, serverId: number) => `${path}${path.includes('?') ? '&' : '?'}serverId=${serverId}`;

export const getGuildSettings = () => api<GuildSettings>('/guild-settings');
export const saveGuildSettings = (body: GuildSettingsSave | GuildWarOpenSave) => api<GuildSettings>('/guild-settings', json('PUT', body));
export const getGuilds = (serverId: number, q: string, page: number, pageSize: number) =>
  api<GuildList>(withServer(`/guilds?${new URLSearchParams({ q, page: String(page), pageSize: String(pageSize) })}`, serverId));
export const getGuild = (serverId: number, id: string) =>
  api<GuildDetail>(withServer(`/guilds/${encodeURIComponent(id)}`, serverId));
export const updateGuild = (serverId: number, id: string, body: GuildUpdate) =>
  api<GuildDetail>(withServer(`/guilds/${encodeURIComponent(id)}`, serverId), json('PUT', body));
export const updateGuildMember = (serverId: number, id: string, playerId: number, body: GuildMemberUpdate) =>
  api<GuildDetail>(withServer(`/guilds/${encodeURIComponent(id)}/members/${playerId}`, serverId), json('PUT', body));
export const addGuildMember = (serverId: number, id: string, body: { version: string; reason: string; playerId: number }) =>
  api<GuildDetail>(withServer(`/guilds/${encodeURIComponent(id)}/members`, serverId), json('POST', body));
export const removeGuildMember = (serverId: number, id: string, playerId: number, body: { version: string; reason: string }) =>
  api<GuildDetail>(withServer(`/guilds/${encodeURIComponent(id)}/members/${playerId}`, serverId), json('DELETE', body));
export const dissolveGuild = (serverId: number, id: string, body: { version: string; confirmName: string; reason: string }) =>
  api<{ id: string; dissolved: boolean }>(withServer(`/guilds/${encodeURIComponent(id)}`, serverId), json('DELETE', body));
