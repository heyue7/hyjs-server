import type { CustomRoute, ElegantConstRoute, ElegantRoute } from '@elegant-router/types';
import { generatedRoutes } from '../elegant/routes';
import { layouts, views } from '../elegant/imports';
import { transformElegantRoutesToVueRoutes } from '../elegant/transform';

/**
 * custom routes
 *
 * @link https://github.com/soybeanjs/elegant-router?tab=readme-ov-file#custom-route
 */
const customRoutes: CustomRoute[] = [];

/** create routes when the auth route mode is static */
export function createStaticRoutes() {
  const constantRoutes: ElegantRoute[] = [];

  const authRoutes: ElegantRoute[] = [];

  [...customRoutes, ...generatedRoutes]
    .map((item) => {
      const order: Record<string, number> = {
        home: 1,
        players: 2,
        items: 3,
        activities: 4,
        'activities-unavailable': 5,
        shops: 6,
        achievements: 7,
        titles: 8,
        feedback: 9,
        wallets: 10,
        'server-clock': 11,
        announcements: 12,
        audit: 13,
        users: 14
      };
      if (item.meta && order[item.name]) item.meta.order = order[item.name];
      return item;
    })
    .filter((item) => item.name !== 'iframe-page')
    .forEach((item) => {
      if (item.meta?.constant) {
        constantRoutes.push(item);
      } else {
        authRoutes.push(item);
      }
    });

  return {
    constantRoutes,
    authRoutes
  };
}

/**
 * Get auth vue routes
 *
 * @param routes Elegant routes
 */
export function getAuthVueRoutes(routes: ElegantConstRoute[]) {
  return transformElegantRoutesToVueRoutes(routes, layouts, views);
}
