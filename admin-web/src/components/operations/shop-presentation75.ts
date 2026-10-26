import type { Item } from '@/service/operations';
import type { ShopProduct } from '@/service/management-operations';

export const trialShopMenus75 = [
  { value: 1, label: '商品' }, { value: 2, label: '紫装' },
  { value: 3, label: '橙装' }, { value: 4, label: '红装' }
];
export const shopCount75 = (value: number) => Number(value || 0).toLocaleString('zh-CN');

// Equipment fragments inherit the original target category and quality in the catalogue.
// Selector boxes use the configured native thousand-wide priority when their target is not exposed.
export function trialShopMenu75(row: ShopProduct, catalogue: Record<string, Item>): number {
  const reward = row.rewards[0];
  const item = catalogue[reward?.resourceId];
  if (item) {
    const equipment = ['equipment', 'accessory'].includes(item.category) ||
      item.applicableTypes?.some(type => ['fragment.target:equipment', 'fragment.target:accessory'].includes(type));
    if (equipment) return ({ purple: 2, orange: 3, red: 4 } as Record<string, number>)[item.qualityColor || ''] || 1;
    if (item.category !== 'item') return 1;
  }
  const group = Math.floor(row.priority / 1000);
  return group >= 1 && group <= 4 ? group : 0;
}
