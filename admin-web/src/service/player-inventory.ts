export interface InventoryAttribute {
  key: string;
  label: string;
  value: number;
  min: number;
  max: number;
}
export interface PlayerInventoryRow {
  key: string;
  category: string;
  resourceId: string;
  name: string;
  quality: number;
  qualityLabel: string;
  iconUrl: string;
  amount: number;
  guid?: string;
  level?: number;
  attributes: InventoryAttribute[];
  editable: boolean;
  removable: boolean;
  blockedReason?: string;
}
export interface PlayerInventoryResponse {
  items: PlayerInventoryRow[];
  total: number;
  version: string;
  categories: { value: string; label: string; count: number }[];
}
export const inventoryCategories = [
  { value: 'warrior', label: '人物' },
  { value: 'equipment', label: '装备' },
  { value: 'accessory', label: '饰品' },
  { value: 'item', label: '道具及碎片' },
  { value: 'gem', label: '宝石及矿石' },
  { value: 'cooking', label: '料理及食材' },
  { value: 'pet', label: '精灵' }
];
export const inventoryQualities = Array.from({ length: 7 }, (_, value) => ({
  value,
  label: ['无品质', '普通', '优秀', '精良', '史诗', '传说', '神话'][value]
}));
