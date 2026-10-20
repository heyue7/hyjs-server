import type { RouteMeta } from 'vue-router';
import ElegantVueRouter from '@elegant-router/vue/vite';
import type { RouteKey } from '@elegant-router/types';

export function setupElegantRouter() {
  return ElegantVueRouter({
    layouts: {
      base: 'src/layouts/base-layout/index.vue',
      blank: 'src/layouts/blank-layout/index.vue'
    },
    routePathTransformer(routeName, routePath) {
      const key = routeName as RouteKey;

      if (key === 'login') {
        const modules: UnionKey.LoginModule[] = ['pwd-login', 'code-login', 'register', 'reset-pwd', 'bind-wechat'];

        const moduleReg = modules.join('|');

        return `/login/:module(${moduleReg})?`;
      }

      return routePath;
    },
    onRouteMetaGen(routeName) {
      const key = routeName as RouteKey;

      const constantRoutes: RouteKey[] = ['login', '403', '404', '500'];

      const meta: Partial<RouteMeta> = {
        title: key,
        i18nKey: `route.${key}` as App.I18n.I18nKey
      };

      if (constantRoutes.includes(key)) {
        meta.constant = true;
      }

      const operations: Record<string, Pick<RouteMeta, 'title' | 'icon' | 'order' | 'hideInMenu' | 'activeMenu'>> = {
        shops_ordinary: {
          title: '普通商店',
          icon: 'ph:treasure-chest',
          order: 1
        },
        shops_memory: {
          title: '记忆圣坛',
          icon: 'ph:treasure-chest',
          order: 2
        },
        shops_popularity: {
          title: '声望商店商品',
          icon: 'ph:treasure-chest',
          order: 3
        },
        'shops_popularity-rank': {
          title: '声望商店奖励',
          icon: 'ph:treasure-chest',
          order: 4
        },
        shops_point: {
          title: '攻略点商店',
          icon: 'ph:treasure-chest',
          order: 5
        },
        shops_score: { title: '积分商店', icon: 'ph:treasure-chest', order: 6 },
        'guild-settings': { title: '公会设置', icon: 'ph:flag', order: 4 },
        guilds: { title: '公会管理', icon: 'ph:users-three', order: 5 },
        shops_guild: { title: '公会商店', icon: 'ph:treasure-chest', order: 7 },
        'shops_pet-market': {
          title: '精灵商店',
          icon: 'ph:treasure-chest',
          order: 8
        },
        'shops_union-fight': {
          title: '公会战商店',
          icon: 'ph:treasure-chest',
          order: 9
        },
        shops_vip: { title: '会员礼包', icon: 'ph:treasure-chest', order: 10 },
        servers: { title: '区服管理', icon: 'ph:flag', order: 2 },
        'activity-schedules': { title: '活动排期', icon: 'ph:calendar-blank', order: 3 },
        fields: { title: '野外地图与部署', icon: 'ph:mountains', order: 4 },
        'wallet-reasons': { title: '钱包入账规则', icon: 'ph:wallet', order: 7 },
        'arena-prizes': { title: '竞技胜利翻牌', icon: 'ph:cards-three', order: 6 },
        shops: { title: '商店管理', icon: 'ph:treasure-chest', order: 6 },
        achievements: {
          title: '成就管理',
          icon: 'ph:treasure-chest',
          order: 7
        },
        titles: { title: '称号管理', icon: 'ph:treasure-chest', order: 8 },
        feedback: { title: '客服反馈', icon: 'ph:megaphone', order: 9 },
        home: { title: '运营概览', icon: 'ph:squares-four', order: 1 },
        items: { title: '道具目录', icon: 'ph:treasure-chest', order: 2 },
        activities: { title: '活动设置', icon: 'ph:calendar-blank', order: 3 },
        announcements: { title: '公告管理', icon: 'ph:megaphone', order: 4 },
        audit: {
          title: '操作审计',
          icon: 'ph:clock-counter-clockwise',
          order: 5
        },
        cdk: { title: '兑换码管理', icon: 'ph:gift', order: 4 },
        'game-accounts': { title: '用户管理', icon: 'ph:users', order: 1 },
        players: { title: '玩家管理', icon: 'ph:users', order: 2 },
        wallets: { title: '蓝绿钱包', icon: 'ph:treasure-chest', order: 5 },
        users: { title: '后台账号', icon: 'ph:users', order: 9 },
        'activities-unavailable': {
          title: '不可用活动',
          icon: 'ph:calendar-blank',
          order: 5,
          hideInMenu: true,
          activeMenu: 'activities'
        },
        'server-clock': {
          title: '服务器时间',
          icon: 'ph:clock-counter-clockwise',
          order: 6
        }
      };
      if (operations[key])
        Object.assign(meta, operations[key], {
          roles: key === 'users' ? ['admin'] : ['viewer', 'operator', 'admin']
        });
      return meta;
    }
  });
}
