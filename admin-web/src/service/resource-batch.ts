import type { Item } from './operations';
export interface ResourceBatchLine { item: Item; amount: number | null }
export function resourceAmountMax(item: Item) {
  return ['warrior', 'character', 'character-exp', 'equipment', 'accessory', 'pet'].includes(item.category) ? 100 : 2147483647;
}
export function validResourceBatch(lines: ResourceBatchLine[]) {
  return lines.length > 0 && lines.length <= 20 && new Set(lines.map(line => line.item.id)).size === lines.length &&
    lines.every(line => Number.isSafeInteger(line.amount) && line.amount! >= 1 && line.amount! <= resourceAmountMax(line.item));
}
