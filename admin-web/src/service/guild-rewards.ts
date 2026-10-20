import { api, json } from './operations';
import type { ResourceAmount } from './operations';
export type GuildRewardKind = 'settlement' | 'weekly' | 'monthly';
export interface GuildRewardPolicy {
  kind: GuildRewardKind;
  group: string;
  weekStart: string;
  month: string;
  enabled: boolean;
  version: string;
  source: string;
  participationRewards: ResourceAmount[];
  victoryRewards: ResourceAmount[];
  transportMvpRewards: ResourceAmount[];
  taskRewards: ResourceAmount[];
  personalMissionsJson: string;
  factoryMissionPrestige: number;
  winPrestige: number;
  losePrestige: number;
  ranks: { rank: number; rewards: ResourceAmount[] }[];
  memberTitle: boolean;
  chairmanTitle: boolean;
}
export interface GuildRewardRow extends GuildRewardPolicy {
  rowVersion: string;
  publishedAt: number;
  frozen: boolean;
  editable: boolean;
}
export interface GuildRewardSave extends GuildRewardPolicy { rowVersion: string; reason: string }
export const getGuildRewards = () => api<{ items: GuildRewardRow[] }>('/guild-rewards');
export const saveGuildReward = (body: GuildRewardSave, create: boolean) => {
  const id = body.kind === 'settlement' ? body.group : body.kind === 'weekly' ? body.weekStart : body.month;
  const path = `/guild-rewards/${body.kind}${create ? '' : `/${encodeURIComponent(id)}`}`;
  return api<GuildRewardRow>(path, json(create ? 'POST' : 'PUT', body));
};
export function emptyGuildReward(kind: GuildRewardKind): GuildRewardRow {
  return { kind, group: kind === 'monthly' ? '' : 'A', weekStart: '', month: '', enabled: false, version: '', source: '', participationRewards: [], victoryRewards: [], transportMvpRewards: [], taskRewards: [], personalMissionsJson: '', factoryMissionPrestige: 0, winPrestige: 0, losePrestige: 0, ranks: [], memberTitle: false, chairmanTitle: false, rowVersion: '', publishedAt: 0, frozen: false, editable: true };
}
