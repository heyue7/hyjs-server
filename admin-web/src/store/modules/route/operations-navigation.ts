import { useSvgIcon } from '@/hooks/common/icon';

/** Organize authorized pages only. Page names and deep links remain stable. */
export function organizeOperationsMenus(menus: App.Global.Menu[]) {
  const pages = new Map(menus.map(row => [row.key, row]));
  const used = new Set<string>();
  function take(key: string, label?: string) {
    const row = pages.get(key);
    if (!row) return undefined;
    used.add(key);
    return label ? { ...row, label, i18nKey: null } : row;
  }
  function group(key: string, label: string, input: (App.Global.Menu | undefined)[]): App.Global.Menu | undefined {
    const children = input.filter((row): row is App.Global.Menu => Boolean(row));
    if (!children.length) return undefined;
    const first = children[0];
    const icons: Record<string, string> = {
      'global-settings': 'ph:globe-hemisphere-east',
      'growth-settings': 'ph:plant',
      'mechanism-settings': 'ph:sliders-horizontal',
      'server-operations': 'ph:flag',
      'player-management': 'ph:users',
      'system-management': 'ph:gear-six'
    };
    const icon = icons[key];
    return {
      key,
      label,
      routeKey: first.routeKey,
      routePath: first.routePath,
      icon: useSvgIcon().SvgIconVNode({ icon, fontSize: 20 }),
      children
    };
  }
  const global = group('global-settings', '全局设置', [
    take('game-accounts', '用户管理'),
    take('items', '道具与资源'),
    take('activities', '活动设置'),
    group('growth-settings', '成长配置', [take('achievements'), take('titles')]),
    group('mechanism-settings', '游戏机制设置', [
      take('arena-prizes'),
      take('wallet-reasons'),
      take('server-clock', '统一游戏时间')
    ]),
    take('shops', '商店管理'),
    take('announcements', '公告管理')
  ]);
  // Wallet deep links remain valid; its entry lives on each user row.
  take('wallets');
  const realms = group('server-operations', '区服运营', [
    take('servers'),
    take('activity-schedules'),
    take('fields', '野外地图与部署'),
    take('players', '玩家管理'),
    take('feedback')
  ]);
  const system = group('system-management', '系统管理', [take('users', '后台账号与权限'), take('audit')]);
  const result = [take('home'), global, realms, system].filter((row): row is App.Global.Menu => Boolean(row));
  // A future authorized page remains reachable even before its category is assigned.
  return [...result, ...menus.filter(row => !used.has(row.key))];
}

export function findOperationsMenu(menus: App.Global.Menu[], key: string): App.Global.Menu | undefined {
  for (const row of menus) {
    if (row.key === key) return row;
    const found = row.children && findOperationsMenu(row.children, key);
    if (found) return found;
  }
  return undefined;
}
