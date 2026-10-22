<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import type { FieldMapPoint, FieldPoint, FieldPresentation, FieldTemplate, FieldWeight } from '@/service/fields';

const props = defineProps<{
  template?: FieldTemplate;
  presentation: Record<string, FieldPresentation>;
}>();

type Kind = 'fish' | 'mine' | 'monster' | 'boss' | 'worldBoss';
interface DisplayPoint {
  key: string;
  kind: Kind;
  id: string;
  label: string;
  candidates: string;
  x: number;
  z: number;
}

const kinds: { key: Kind; label: string; glyph: string; candidateLabel: string }[] = [
  { key: 'fish', label: '钓鱼', glyph: '钓', candidateLabel: '候选鱼群' },
  { key: 'mine', label: '矿物', glyph: '矿', candidateLabel: '候选矿脉' },
  { key: 'monster', label: '小怪', glyph: '怪', candidateLabel: '候选小怪' },
  { key: 'boss', label: '普通首领出生点', glyph: '首', candidateLabel: '出生点候选首领' },
  { key: 'worldBoss', label: '世界首领候选点', glyph: '世', candidateLabel: '候选出生位置' }
];
const kindMap = Object.fromEntries(kinds.map(kind => [kind.key, kind])) as Record<Kind, (typeof kinds)[number]>;
const visibleKinds = ref<Kind[]>(kinds.map(kind => kind.key));
const activeKey = ref('');
const zoom = ref(1);
const zoomOptions = [1, 1.5, 2];

const cfg = computed(() => props.template?.configuration);
const scene = computed(() => cfg.value?.scene || '');
const entry = computed(() => props.presentation[scene.value]);
const map = computed(() => entry.value?.map);
const mapIcon = computed(() => entry.value?.iconUrl || '');
const mapWidth = computed(() => map.value?.width || 512);
const mapHeight = computed(() => map.value?.height || 512);
const mapReady = computed(() => Boolean(mapIcon.value && map.value && map.value.scale > 0));
const emptyReason = computed(() => {
  if (!entry.value?.iconUrl) return '该地图暂无原图，无法显示分布';
  if (!map.value) return '该地图暂无原小地图参数，无法显示分布';
  return '该地图参数不完整，无法显示分布';
});

function parsePoint(raw: string) {
  const values = raw.split(',');
  if (values.length < 3) return undefined;
  const x = Number(values[0]);
  const z = Number(values[2]);
  return Number.isFinite(x) && Number.isFinite(z) ? { x, z } : undefined;
}

function candidateNames(choices: FieldWeight[] | undefined, fallback: string) {
  const names: string[] = [];
  for (const choice of choices || []) {
    const name = props.presentation[choice.id]?.name || choice.id;
    if (name && !names.includes(name)) names.push(name);
  }
  if (!names.length) return fallback;
  return names.length > 4 ? `${names.slice(0, 4).join('、')} 等` : names.join('、');
}

function gatherPoints(points: FieldPoint[] | undefined, kind: 'fish' | 'mine'): DisplayPoint[] {
  return (points || []).flatMap((point, index) => {
    const position = parsePoint(point.point);
    if (!position) return [];
    return [
      {
        key: `${kind}-${point.id}`,
        kind,
        id: point.id,
        label: kind === 'mine' ? `矿物候选 ${index + 1}` : `钓鱼候选 ${index + 1}`,
        candidates: candidateNames(point.choices, '原资源组'),
        ...position
      }
    ];
  });
}

function monsterPoints(points: FieldPoint[] | undefined): DisplayPoint[] {
  return (points || []).flatMap((point, index) => {
    const position = parsePoint(point.point);
    if (!position) return [];
    return [
      {
        key: `monster-${point.id}`,
        kind: 'monster',
        id: point.id,
        label: `小怪候选 ${index + 1}`,
        candidates: candidateNames(point.choices, '原怪物组'),
        ...position
      }
    ];
  });
}

function bossPoints(points: FieldMapPoint[] | undefined, kind: 'boss' | 'worldBoss'): DisplayPoint[] {
  return (points || []).flatMap((point, index) => {
    const label =
      kind === 'boss' ? point.name || `普通首领出生点 ${index + 1}` : point.name || `世界首领候选点 ${index + 1}`;
    return [
      {
        key: `${kind}-${point.id}`,
        kind,
        id: point.id,
        label,
        candidates: point.name || point.id,
        x: point.x,
        z: point.z
      }
    ];
  });
}

const allPoints = computed<DisplayPoint[]>(() => [
  ...gatherPoints(cfg.value?.fishPoints, 'fish'),
  ...gatherPoints(cfg.value?.minePoints, 'mine'),
  ...monsterPoints(cfg.value?.monsterPoints),
  ...bossPoints(map.value?.bossPoints, 'boss'),
  ...bossPoints(map.value?.worldBossPoints, 'worldBoss')
]);
const visiblePoints = computed(() => allPoints.value.filter(point => visibleKinds.value.includes(point.kind)));
const activePoint = computed(() => allPoints.value.find(point => point.key === activeKey.value));
const kindCounts = computed(() => {
  const counts = {} as Record<Kind, number>;
  for (const kind of kinds) counts[kind.key] = allPoints.value.filter(point => point.kind === kind.key).length;
  return counts;
});

function toggleKind(kind: Kind) {
  if (visibleKinds.value.includes(kind)) {
    visibleKinds.value = visibleKinds.value.filter(item => item !== kind);
    if (activePoint.value?.kind === kind) activeKey.value = '';
    return;
  }
  visibleKinds.value = [...visibleKinds.value, kind];
}

function pointStyle(point: DisplayPoint) {
  const meta = map.value;
  if (!meta || meta.scale <= 0) return {};
  return {
    left: `${50 + (((point.x - meta.offsetX) * meta.scale) / mapWidth.value) * 100}%`,
    top: `${50 - (((point.z - meta.offsetY) * meta.scale) / mapHeight.value) * 100}%`
  };
}

function numberText(value: number) {
  return Number(value.toFixed(2)).toString();
}

function pointAria(point: DisplayPoint) {
  const meta = kindMap[point.kind];
  return `${meta.label}，坐标 X ${numberText(point.x)}，Z ${numberText(point.z)}，${meta.candidateLabel}：${point.candidates}`;
}

watch(
  () => props.template?.id,
  () => {
    activeKey.value = '';
    zoom.value = 1;
  }
);
</script>

<template>
  <div class="field-map">
    <NEmpty v-if="!template" description="请选择一个地图方案" />
    <NEmpty v-else-if="!mapReady" :description="emptyReason" />
    <template v-else>
      <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div class="flex flex-wrap items-center gap-2" role="group" aria-label="地图分类筛选">
          <button
            v-for="kind in kinds"
            :key="kind.key"
            type="button"
            class="legend-item"
            :class="[`kind-${kind.key}`, { 'is-active': visibleKinds.includes(kind.key) }]"
            :aria-pressed="visibleKinds.includes(kind.key)"
            @click="toggleKind(kind.key)"
          >
            <span class="legend-shape" aria-hidden="true">{{ kind.glyph }}</span>
            <span>{{ kind.label }}</span>
            <span class="legend-count">{{ kindCounts[kind.key] }}</span>
          </button>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-xs text-gray-500">显示比例</span>
          <div class="flex items-center gap-1" role="group" aria-label="地图显示比例">
            <NButton
              v-for="option in zoomOptions"
              :key="option"
              size="small"
              :type="zoom === option ? 'primary' : 'default'"
              @click="zoom = option"
            >
              {{ option * 100 }}%
            </NButton>
          </div>
        </div>
      </div>
      <p v-if="!visiblePoints.length" class="mb-2 text-sm text-gray-500">没有符合当前分类的标记。</p>
      <div class="map-scroll" tabindex="0" aria-label="大地图，可滚动查看">
        <div
          class="map-frame"
          :class="{ 'is-zoomed': zoom > 1 }"
          :style="{ width: `${zoom * 100}%`, aspectRatio: `${mapWidth} / ${mapHeight}` }"
        >
          <img class="map-image" :src="mapIcon" :alt="`${entry?.name || '野外地图'} 原小地图`" />
          <button
            v-for="point in visiblePoints"
            :key="point.key"
            type="button"
            class="map-marker"
            :class="[`kind-${point.kind}`, { 'is-active': activeKey === point.key }]"
            :style="pointStyle(point)"
            :aria-label="pointAria(point)"
            :aria-pressed="activeKey === point.key"
            :title="`${kindMap[point.kind].label} · ${point.label}`"
            @click="activeKey = point.key"
            @focus="activeKey = point.key"
          >
            <span class="marker-glyph" aria-hidden="true">{{ kindMap[point.kind].glyph }}</span>
          </button>
        </div>
      </div>
      <p class="mt-2 text-xs text-gray-500">
        候选点按原小地图参数投影，标记位置不代表实时刷新状态；世界首领候选点属于同一全服周期，不会同时出现多只。
      </p>
      <div class="map-detail" aria-live="polite">
        <template v-if="activePoint">
          <div class="flex flex-wrap items-center gap-2">
            <span class="detail-glyph" :class="`kind-${activePoint.kind}`" aria-hidden="true">
              {{ kindMap[activePoint.kind].glyph }}
            </span>
            <strong>{{ kindMap[activePoint.kind].label }}</strong>
            <NTag size="small" type="warning">{{ activePoint.kind === 'boss' ? '出生点 · 非实时' : '候选点 · 非实时' }}</NTag>
            <span class="text-sm text-gray-500">{{ activePoint.label }}</span>
          </div>
          <div class="mt-2 flex flex-wrap gap-x-5 gap-y-1 text-sm">
            <span>坐标 X {{ numberText(activePoint.x) }} · Z {{ numberText(activePoint.z) }}</span>
            <span class="text-gray-500">原点位 {{ activePoint.id }}</span>
            <span>{{ kindMap[activePoint.kind].candidateLabel }}：{{ activePoint.candidates }}</span>
          </div>
          <p v-if="activePoint.kind === 'worldBoss'" class="mt-2 text-sm text-gray-500">
            同一世界首领周期只会在其中一个候选点出现。
          </p>
        </template>
        <p v-else class="text-sm text-gray-500">选择地图标记查看类别、坐标和候选资源。</p>
      </div>
    </template>
  </div>
</template>

<style scoped>
.map-scroll {
  max-height: min(78vh, 760px);
  overflow: auto;
  border: 1px solid var(--n-border-color, #e5e7eb);
  border-radius: 8px;
  background: #f8fafc;
}
.map-frame {
  --marker-size: 26px;
  --marker-size: clamp(18px, 3.6cqw, 38px);
  position: relative;
  width: 100%;
  max-width: min(100%, 68vh);
  margin-inline: auto;
  container-type: inline-size;
}
.map-frame.is-zoomed {
  max-width: none;
}
.map-image {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
  user-select: none;
}
.map-marker {
  position: absolute;
  z-index: 1;
  display: grid;
  width: var(--marker-size);
  height: var(--marker-size);
  padding: 0;
  border: 0;
  border-radius: 0;
  place-items: center;
  transform: translate(-50%, -50%);
  color: var(--marker-color);
  background: transparent;
  cursor: pointer;
}
.map-marker::before {
  position: absolute;
  inset: 0;
  border: 1px solid var(--marker-color);
  border-radius: 4px;
  background: var(--marker-bg);
  box-shadow: 0 1px 3px rgb(15 23 42 / 28%);
  content: '';
}
.map-marker .marker-glyph {
  position: relative;
  z-index: 1;
  font-size: clamp(10px, 1.8cqw, 15px);
  font-weight: 700;
  line-height: 1;
}
.map-marker:hover::before {
  filter: brightness(1.06);
}
.map-marker:focus-visible {
  outline: 3px solid #2563eb;
  outline-offset: 3px;
}
.map-marker.is-active {
  z-index: 2;
}
.map-marker.is-active::before {
  box-shadow:
    0 0 0 3px rgb(37 99 235 / 35%),
    0 2px 6px rgb(15 23 42 / 30%);
}
.map-marker.kind-fish::before {
  border-radius: 50%;
}
.map-marker.kind-mine::before {
  border-radius: 3px;
}
.map-marker.kind-monster::before {
  border-radius: 50% 50% 50% 0;
  transform: rotate(45deg);
}
.map-marker.kind-monster .marker-glyph {
  transform: rotate(-45deg);
}
.map-marker.kind-boss::before {
  border-radius: 3px;
  outline: 2px solid var(--marker-color);
  outline-offset: 1px;
}
.map-marker.kind-worldBoss::before {
  border-width: 2px;
  border-radius: 50%;
  box-shadow:
    0 0 0 2px var(--marker-bg),
    0 0 0 3px var(--marker-color);
}
.map-marker.kind-fish,
.legend-item.kind-fish {
  --marker-color: #1d4ed8;
  --marker-bg: #dbeafe;
}
.map-marker.kind-mine,
.legend-item.kind-mine {
  --marker-color: #b45309;
  --marker-bg: #fef3c7;
}
.map-marker.kind-monster,
.legend-item.kind-monster {
  --marker-color: #15803d;
  --marker-bg: #dcfce7;
}
.map-marker.kind-boss,
.legend-item.kind-boss {
  --marker-color: #b91c1c;
  --marker-bg: #fee2e2;
}
.map-marker.kind-worldBoss,
.legend-item.kind-worldBoss {
  --marker-color: #7e22ce;
  --marker-bg: #f3e8ff;
}
.legend-item {
  display: inline-flex;
  min-height: 34px;
  align-items: center;
  gap: 6px;
  padding: 4px 10px 4px 5px;
  border: 1px solid #d1d5db;
  border-radius: 999px;
  color: #374151;
  font-size: 13px;
  background: #fff;
  cursor: pointer;
  transition:
    border-color 0.15s ease,
    background-color 0.15s ease;
}
.legend-item:hover {
  border-color: var(--marker-color);
}
.legend-item:focus-visible {
  outline: 2px solid #2563eb;
  outline-offset: 2px;
}
.legend-item:not(.is-active) {
  opacity: 0.55;
}
.legend-shape,
.detail-glyph {
  display: grid;
  width: 22px;
  height: 22px;
  border: 1px solid var(--marker-color);
  border-radius: 50%;
  place-items: center;
  color: var(--marker-color);
  font-size: 11px;
  font-weight: 700;
  background: var(--marker-bg);
}
.legend-count {
  color: #6b7280;
  font-variant-numeric: tabular-nums;
}
.kind-mine .legend-shape,
.detail-glyph.kind-mine {
  border-radius: 3px;
}
.kind-monster .legend-shape,
.detail-glyph.kind-monster {
  border-radius: 50% 50% 50% 0;
  transform: rotate(45deg);
}
.kind-monster .legend-shape {
  margin-right: 3px;
}
.map-detail {
  min-height: 64px;
  margin-top: 12px;
  padding: 12px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fff;
}
.detail-glyph {
  width: 24px;
  height: 24px;
  font-size: 11px;
}
.dark .map-scroll {
  border-color: #374151;
  background: #111827;
}
.dark .map-marker.kind-fish,
.dark .legend-item.kind-fish {
  --marker-color: #93c5fd;
  --marker-bg: #1e3a8a;
}
.dark .map-marker.kind-mine,
.dark .legend-item.kind-mine {
  --marker-color: #fcd34d;
  --marker-bg: #78350f;
}
.dark .map-marker.kind-monster,
.dark .legend-item.kind-monster {
  --marker-color: #86efac;
  --marker-bg: #14532d;
}
.dark .map-marker.kind-boss,
.dark .legend-item.kind-boss {
  --marker-color: #fca5a5;
  --marker-bg: #7f1d1d;
}
.dark .map-marker.kind-worldBoss,
.dark .legend-item.kind-worldBoss {
  --marker-color: #d8b4fe;
  --marker-bg: #581c87;
}
.dark .legend-item {
  border-color: #4b5563;
  color: #e5e7eb;
  background: #1f2937;
}
.dark .map-detail {
  border-color: #374151;
  background: #1f2937;
}
</style>
