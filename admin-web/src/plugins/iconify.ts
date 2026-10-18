import { addCollection } from '@iconify/vue/offline';
import operationsIcons from './operations-icons.json';
// Bundle the actual navigation icons locally, so the offline game admin has no Iconify CDN dependency.
export function setupIconifyOffline() {
  for (const collection of operationsIcons) {
    const icons: Record<string, { body: string }> = {};
    for (const [name, icon] of Object.entries(collection.icons)) if (icon) icons[name] = icon;
    addCollection({ prefix: collection.prefix, width: collection.width, height: collection.height, icons });
  }
}
