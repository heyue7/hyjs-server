import { api } from './operations';
import type { Item } from './operations';
export interface ItemDetail { item: Item; sections: { title: string; fields: { label: string; value: string }[] }[] }
export const getItemDetail = (id: string) => api<ItemDetail>(`/items/${encodeURIComponent(id)}`);
