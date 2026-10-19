<script setup lang="ts">
import { computed, h, onBeforeUnmount, ref, watch } from 'vue';
import { onBeforeRouteLeave, useRouter } from 'vue-router';
import { NButton, NSpace, NSwitch } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, errorMessage, formatTime, json } from '@/service/operations';
import { newShopProduct, shopLabels } from '@/service/management-operations';
import type { ShopDefinition, ShopProduct, ShopSnapshot } from '@/service/management-operations';
import type { Item, ItemsResponse } from '@/service/operations';
import { useOperations } from '@/composables/useOperations';
import ManagementWorkspace from './management-workspace.vue';
import ContextHelp from './context-help.vue';
import RewardList from './reward-list.vue';
import ResourceSelect from './resource-select.vue';
import ResourceImage from './resource-image.vue';
import ItemQuality from './item-quality.vue';
const props = defineProps<{ shopId: string }>();
const router = useRouter();
const isPopularityShop = computed(() => ['popularity', 'popularity-rank'].includes(props.shopId));
const { canWrite, loading, error, run } = useOperations();
const definition = ref<ShopDefinition>();
const snapshot = ref<ShopSnapshot>();
const catalogue = ref<Record<string, Item>>({});
const catalogueError = ref('');
const q = ref('');
const state = ref<string | null>(null);
const editing = ref(false);
const selected = ref<ShopProduct | null>(null);
const draft = ref<ShopProduct>(newShopProduct());
const original = ref('');
const saving = ref(false);
const toggling = ref('');
const saveError = ref('');
const section = ref('product');
const settingsOpen = ref(false);
const settingsDraft = ref<ShopSnapshot['settings']>({
  unlimited: true,
  enabled: false,
  slots: 0,
  startAt: 0,
  endAt: 0,
  claimEndAt: 0,
  source: ''
});
const settingsError = ref('');
const settingsSaving = ref(false);
const needsPeriod = computed(
  () => !['vip', 'union-fight', 'pet-market'].includes(props.shopId) && !snapshot.value?.settings.periodId
);
const productReadonly = (_row: ShopProduct | null) => !canWrite.value;
const readonly = computed(() => productReadonly(selected.value));
const isLegacyProduct = computed(() => props.shopId === 'ordinary' && Boolean(selected.value?.id.startsWith('legacy:')));
const recommendedCosts = computed(() => {
  const original = ({ ordinary: ['coin', 'diamond'], memory: ['light'], popularity: ['popularity'], 'popularity-rank': ['popularity'], point: ['point'], score: ['score'], vip: ['diamond'], 'pet-market': ['coinpet', 'diamond'], 'union-fight': ['unionFightToken'] } as Record<string, string[]>)[props.shopId] || [];
  return [...new Set([...original, ...(snapshot.value?.items || []).map(row => row.cost?.resourceId).filter(Boolean)])].slice(0, 20);
});
const vipOptions = computed(() =>
  Array.from({ length: props.shopId === 'vip' ? 12 : 13 }, (_, index) => {
    const value = props.shopId === 'vip' ? index + 1 : index;
    return { value, label: value ? `VIP ${value}` : '不限 VIP' };
  })
);
const settingSupports = (key: string) => Boolean(definition.value?.settingsFields?.includes(key));
const sections = [
  { key: 'product', label: '商品与消耗' },
  { key: 'limits', label: '资格与限购' },
  { key: 'publish', label: '状态与备注' }
];
const title = computed(() =>
  isPopularityShop.value ? shopLabels[props.shopId] : definition.value?.label || shopLabels[props.shopId] || '商店管理'
);
const fields = computed(() => definition.value?.fields || []);
const supports = (key: string) => fields.value.includes(key);
const hasPublishFields = computed(() =>
  supports('enabled') ||
  ['day', 'refreshMillis', 'source'].some(supports)
);
const showShopStatus = computed(() => Boolean(snapshot.value && (
  settingSupports('unlimited') || settingSupports('enabled') || snapshot.value.settings.startAt || snapshot.value.settings.endAt
)));
const dirty = computed(() => editing.value && JSON.stringify(draft.value) !== original.value);
const rows = computed(() =>
  (snapshot.value?.items || []).filter(
    row =>
      (state.value === null || row.enabled === (state.value === 'enabled')) &&
      `${row.name} ${row.id} ${row.rewards.map(r => `${r.resourceId} ${catalogue.value[r.resourceId]?.name || ''}`).join(' ')} ${catalogue.value[row.cost?.resourceId]?.name || ''}`
        .toLowerCase()
        .includes(q.value.trim().toLowerCase())
  )
);
let generation = 0;
async function load() {
  const current = ++generation;
  const result = await run(async () => {
    const [directory, data] = await Promise.all([
      api<{ items: ShopDefinition[] }>('/shops'),
      api<ShopSnapshot>(`/shops/${encodeURIComponent(props.shopId)}`)
    ]);
    return { directory, data };
  });
  if (result && current === generation) {
    definition.value = result.directory.items.find(row => row.id === props.shopId);
    snapshot.value = { ...result.data, items: result.data.items || [] };
    catalogueError.value = '';
    const resources = Object.fromEntries((result.data.resources || []).map(item => [item.id, item]));
    const ids = [...new Set(result.data.items.flatMap(row => [...row.rewards.map(reward => reward.resourceId), row.cost?.resourceId]).filter(Boolean))].filter(id => !resources[id]);
    try {
      const batches = [];
      for (let index = 0; index < ids.length; index += 100)
        batches.push(api<ItemsResponse>(`/items?${new URLSearchParams({ ids: ids.slice(index, index + 100).join(','), page: '1', pageSize: '100' })}`));
      const responses = await Promise.all(batches);
      if (current !== generation) return;
      for (const response of responses) for (const item of response.items) resources[item.id] = item;
    } catch {
      if (current !== generation) return;
      catalogueError.value = '部分资源名称暂未载入，请刷新重试。';
    }
    if (current === generation) catalogue.value = resources;
  }
}
function edit(row: ShopProduct | null) {
  selected.value = row;
  draft.value = row
    ? {
        ...newShopProduct(),
        ...JSON.parse(JSON.stringify(row)),
        cost: row.cost ? { ...row.cost } : { resourceId: '', amount: 0 },
        rewards: (row.rewards || []).map(reward => ({ ...reward }))
      }
    : newShopProduct();
  if (!row && ['pet-market', 'union-fight'].includes(props.shopId)) draft.value.stock = 1;
  if (!row && props.shopId === 'union-fight') draft.value.refreshMillis = 60000;
  if (!row && props.shopId === 'vip') draft.value.minVip = 1;
  if (!row && recommendedCosts.value.length) draft.value.cost.resourceId = recommendedCosts.value[0];
  original.value = JSON.stringify(draft.value);
  section.value = 'product';
  saveError.value = '';
  editing.value = true;
}
function beginCreate() {
  if (needsPeriod.value) configure();
  else edit(null);
}
function discard() {
  if (!dirty.value || !canWrite.value) {
    editing.value = false;
    return;
  }
  window.$dialog?.warning({
    title: '放弃未保存的商品修改？',
    content: '返回后当前输入将不会保存。',
    positiveText: '放弃修改',
    negativeText: '继续编辑',
    onPositiveClick: () => {
      editing.value = false;
    }
  });
}
function validate() {
  if (
    !draft.value.rewards.length ||
    draft.value.rewards.some(r => !r.resourceId || !Number.isInteger(r.amount) || r.amount < 1 || r.amount > 10000)
  )
    return '请选择获得资源并填写1–10000的数量';
  if (
    supports('cost') &&
    (!draft.value.cost.resourceId ||
      draft.value.cost.amount < (props.shopId === 'union-fight' ? 0 : 1) ||
      !Number.isInteger(draft.value.cost.amount))
  )
    return '请填写完整的消耗资源和有效金额';
  if (supports('maxRank') && (!Number.isInteger(draft.value.maxRank) || draft.value.maxRank < 1))
    return '请填写可兑换的最末排名（正整数），排名数字越小越靠前';
  return '';
}
async function save() {
  if (saving.value || !snapshot.value || readonly.value) return;
  saveError.value = validate();
  if (saveError.value) return;
  saving.value = true;
  try {
    await api(
      `/shops/${encodeURIComponent(props.shopId)}/products${selected.value ? `/${encodeURIComponent(selected.value.id)}` : ''}`,
      json(selected.value ? 'PUT' : 'POST', {
        ...draft.value,
        version: snapshot.value.version
      })
    );
    original.value = JSON.stringify(draft.value);
    editing.value = false;
    window.$message?.success('商店商品已保存');
    await load();
  } catch (e) {
    saveError.value = errorMessage(e);
  } finally {
    saving.value = false;
  }
}
async function toggleProduct(row: ShopProduct, enabled: boolean) {
  if (!snapshot.value || !canWrite.value || toggling.value || saving.value) return;
  toggling.value = row.id;
  try {
    await api(`/shops/${encodeURIComponent(props.shopId)}/products/${encodeURIComponent(row.id)}`,
      json('PUT', { ...row, enabled, version: snapshot.value.version }));
    await load();
    window.$message?.success(enabled ? '商品已启用' : '商品已停用');
  } catch (e) {
    window.$message?.error(errorMessage(e));
  } finally {
    toggling.value = '';
  }
}
function configure() {
  if (!snapshot.value) return;
  settingsDraft.value = {
    ...snapshot.value.settings,
    source: snapshot.value.settings.source || '用户运营配置',
    newPeriod: false
  };
  settingsError.value = '';
  settingsOpen.value = true;
}
async function saveSettings() {
  if (!snapshot.value || settingsSaving.value || !canWrite.value) return;
  settingsError.value =
    !settingsDraft.value.unlimited &&
    Boolean(settingsDraft.value.startAt || settingsDraft.value.endAt || settingsDraft.value.claimEndAt) &&
    (!settingsDraft.value.startAt ||
      !settingsDraft.value.endAt ||
      !settingsDraft.value.claimEndAt)
      ? '请填写营业开始、结束和领奖截止时间'
      : !settingsDraft.value.unlimited && settingsDraft.value.endAt && settingsDraft.value.endAt <= settingsDraft.value.startAt
        ? '结束时间需晚于开始时间'
        : !settingsDraft.value.unlimited && settingsDraft.value.claimEndAt &&
            settingsDraft.value.endAt &&
            settingsDraft.value.claimEndAt < settingsDraft.value.endAt
          ? '领奖结束时间不能早于活动结束时间'
          : '';
  if (!settingsError.value && settingSupports('source') && !settingsDraft.value.source.trim()) settingsError.value = '请填写配置备注';
  if (settingsError.value) return;
  settingsSaving.value = true;
  try {
    await api(
      `/shops/${encodeURIComponent(props.shopId)}/settings`,
      json('PUT', { ...settingsDraft.value, version: snapshot.value.version })
    );
    settingsOpen.value = false;
    editing.value = false;
    await load();
    window.$message?.success('商店设置已保存');
  } catch (e) {
    settingsError.value = errorMessage(e);
  } finally {
    settingsSaving.value = false;
  }
}
function remove(row: ShopProduct) {
  const version = snapshot.value?.version;
  window.$dialog?.warning({
    title: '移除商品？',
    content: `${row.name || row.id} 将从后续可售配置移除，已有购买记录不变。`,
    positiveText: '移除',
    negativeText: '取消',
    onPositiveClick: async () => {
      const result = await run(() =>
        api(
          `/shops/${encodeURIComponent(props.shopId)}/products/${encodeURIComponent(row.id)}`,
          json('DELETE', { version })
        )
      );
      if (result !== undefined) await load();
    }
  });
}
function resourceName(id: string) {
  return catalogue.value[id]?.name || (id ? '资源名称暂未载入' : '未配置');
}
function renderResourceNames(row: ShopProduct) {
  return h('div', { class: 'shop-cell-stack' }, row.rewards.map(reward =>
    h('div', { class: 'shop-resource-name', title: reward.resourceId }, [
      h('span', resourceName(reward.resourceId)),
      ...(catalogue.value[reward.resourceId]?.qualityLabel ? [h(ItemQuality, { item: catalogue.value[reward.resourceId] })] : [])
    ])
  ));
}
function renderLines(lines: string[]) {
  return h('div', { class: 'shop-cell-stack shop-rule-lines' }, lines.length ? lines.map(line => h('span', line)) : [h('span', { class: 'ops-muted' }, '不限')]);
}
function limitLines(row: ShopProduct) {
  if (props.shopId === 'vip') return ['每日 1 份'];
  return [
    ...(supports('dailyLimit') && row.dailyLimit > 0 ? [`每日 ${row.dailyLimit.toLocaleString()} 次`] : []),
    ...(supports('totalLimit') && row.totalLimit > 0 ? [`累计 ${row.totalLimit.toLocaleString()} 次`] : []),
    ...(supports('stock') && row.stock >= 0 ? [`${props.shopId === 'pet-market' ? '每周库存' : '库存'} ${row.stock.toLocaleString()}`] : [])
  ];
}
function qualificationLines(row: ShopProduct) {
  return [
    ...(props.shopId === 'vip' ? ['需有已确认付款记录'] : []),
    ...(supports('minLevel') && row.minLevel > 1 ? [`角色 ${row.minLevel} 级`] : []),
    ...(supports('minVip') && row.minVip > 0 ? [`VIP ${row.minVip}`] : []),
    ...(supports('guildLevel') && row.guildLevel > 1 ? [`公会 ${row.guildLevel} 级`] : []),
    ...(supports('buyCondition') && row.buyCondition > 0 ? [row.buyCondition === 2 ? '会长' : '副会长／会长'] : []),
    ...(supports('maxRank') && row.maxRank > 0 ? [`竞技前 ${row.maxRank.toLocaleString()} 名`] : [])
  ];
}
const columns = computed<DataTableColumns<ShopProduct>>(() => [
  {
    title: '图片', key: 'image', width: 72,
    render: row => h('div', { class: 'shop-cell-stack' }, row.rewards.map(reward => h(ResourceImage, { item: catalogue.value[reward.resourceId], size: 34 })))
  },
  {
    title: '商品名称', key: 'name', minWidth: 225,
    render: row => h('div', [
      ...(row.name && !row.rewards.some(reward => row.name === catalogue.value[reward.resourceId]?.name) ? [h('div', { class: 'shop-custom-name' }, row.name)] : []),
      renderResourceNames(row)
    ])
  },
  {
    title: '获得数量', key: 'rewardAmounts', width: 110,
    render: row => h('div', { class: 'shop-cell-stack shop-number-cell' }, row.rewards.map(reward => h('span', Number(reward.amount).toLocaleString())))
  },
  {
    title: '消耗资源', key: 'costResource', minWidth: 180,
    render: row => h('div', { class: 'shop-cell-stack' }, [
      ...(row.cost?.resourceId ? [h('div', { class: 'shop-cost-resource', title: row.cost.resourceId }, [h(ResourceImage, { item: catalogue.value[row.cost.resourceId], size: 28 }), h('span', resourceName(row.cost.resourceId))])] : []),
      ...(row.contributionCost > 0 ? [h('span', '公会贡献')] : []),
      ...(!row.cost?.resourceId && !row.contributionCost ? [h('span', { class: 'ops-muted' }, '未配置')] : [])
    ])
  },
  {
    title: '消耗数量', key: 'costAmount', width: 110,
    render: row => h('div', { class: 'shop-cell-stack shop-number-cell' }, [
      ...(row.cost?.resourceId ? [h('span', Number(row.cost.amount).toLocaleString())] : []),
      ...(row.contributionCost > 0 ? [h('span', row.contributionCost.toLocaleString())] : [])
    ])
  },
  { title: '限购与库存', key: 'limits', minWidth: 125, render: row => renderLines(limitLines(row)) },
  { title: '购买资格', key: 'qualifications', minWidth: 140, render: row => renderLines(qualificationLines(row)) },
  {
    title: '状态', key: 'enabled', width: 85,
    render: row => h('div', { class: 'shop-cell-stack' }, [h(NSwitch, {
      value: row.enabled,
      size: 'small',
      loading: toggling.value === row.id,
      disabled: !canWrite.value || loading.value || saving.value || Boolean(toggling.value),
      'aria-label': `启用商品：${row.name || '商品'}`,
      'onUpdate:value': (value: boolean) => toggleProduct(row, value)
    }), h('span', { class: 'ops-muted' }, row.enabled ? '已启用' : '已停用')])
  },
  {
    title: '操作', key: 'action', width: 135, fixed: 'right',
    render: row => h(NSpace, { size: 4, wrap: false }, () => [
      h(NButton, { size: 'small', secondary: true, onClick: () => edit(row) }, () => productReadonly(row) ? '查看' : '编辑'),
      ...(!productReadonly(row) ? [h(NButton, { size: 'small', quaternary: true, type: 'error', onClick: () => remove(row) }, () => '移除')] : [])
    ])
  }
]);
function beforeUnload(event: BeforeUnloadEvent) {
  if (dirty.value && canWrite.value) {
    event.preventDefault();
    event.returnValue = '';
  }
}
window.addEventListener('beforeunload', beforeUnload);
onBeforeUnmount(() => {
  generation += 1;
  window.removeEventListener('beforeunload', beforeUnload);
});
onBeforeRouteLeave(() => {
  if (!dirty.value || !canWrite.value) return true;
  return new Promise<boolean>(resolve =>
    window.$dialog?.warning({
      title: '离开当前商品？',
      content: '未保存的修改将丢失。',
      positiveText: '离开',
      negativeText: '继续编辑',
      onPositiveClick: () => resolve(true),
      onNegativeClick: () => resolve(false),
      onClose: () => resolve(false),
      onMaskClick: () => resolve(false)
    })
  );
});
watch(
  () => props.shopId,
  () => {
    editing.value = false;
    definition.value = undefined;
    snapshot.value = undefined;
    q.value = '';
    state.value = null;
    load();
  },
  { immediate: true }
);
</script>

<template>
  <div class="shop-workspace-root">
    <ManagementWorkspace
      v-model:section="section"
      :title="editing ? `${selected ? (readonly ? '查看' : '编辑') : '新建'}商品 · ${title}` : title"
      :editing="editing"
      :saving="saving"
      :readonly="readonly"
      :dirty="dirty"
      :sections="sections.filter(row => row.key !== 'publish' || hasPublishFields)"
      @back="discard"
      @save="save"
    >
      <template #title-extra><ContextHelp v-if="definition?.notes" title="商店规则" :text="definition.notes" /></template>
      <template v-if="!editing" #actions>
        <NButton :loading="loading" @click="load">刷新</NButton>
        <NButton v-if="canWrite && snapshot && definition?.settingsFields?.length" @click="configure()">
          商店设置
        </NButton>
        <NButton
          v-if="canWrite && definition && definition.canCreate !== false"
          type="primary"
          @click="beginCreate"
        >
          新建商品
        </NButton>
      </template>
      <template #alert>
        <NAlert v-if="error || saveError" type="error" :title="saveError || error" />
      </template>
      <template v-if="!editing">
        <div v-if="snapshot && showShopStatus" class="shop-status-line">
          <NSpace align="center" :size="8">
            <NTag
              v-if="settingSupports('enabled')"
              :type="snapshot.settings.enabled ? 'success' : 'default'"
              :bordered="false"
              size="small"
            >
              {{ snapshot.settings.enabled ? '营业中' : '已停用' }}
            </NTag>
            <NTag v-if="snapshot.settings.unlimited" :bordered="false" size="small">不限时</NTag>
            <span v-else-if="snapshot.settings.startAt || snapshot.settings.endAt" class="ops-muted">
              {{ formatTime(snapshot.settings.startAt) }} →
              {{ snapshot.settings.endAt ? formatTime(snapshot.settings.endAt) : '不限' }}
            </span>
          </NSpace>
        </div>
        <NAlert v-if="shopId === 'vip' && snapshot && !snapshot.items.some(item => item.enabled)" type="info" title="暂无开放的会员礼包" class="mb-16px">
          下方保留全部 {{ snapshot.items.length }} 件已停用商品，可查看和编辑内容、价格与购买资格。商品启用后才会在游戏中显示。
        </NAlert>
        <NCard :bordered="false" class="ops-panel">
          <NTabs v-if="isPopularityShop" :value="shopId" type="line" class="mb-20px" @update:value="router.push(`/shops/${$event}`)">
            <NTab name="popularity">声望商店商品</NTab>
            <NTab name="popularity-rank">声望商店奖励</NTab>
          </NTabs>
          <div class="ops-filters">
            <NInput v-model:value="q" class="ops-search" clearable placeholder="搜索商品名称或编号" />
            <NSelect
              v-model:value="state"
              clearable
              :options="[
                { label: '启用', value: 'enabled' },
                { label: '停用', value: 'disabled' }
              ]"
              placeholder="全部状态"
              class="ops-filter-select"
            />
          </div>
          <p v-if="catalogueError" class="ops-inline-note">{{ catalogueError }}</p>
          <NDataTable
            :columns="columns"
            :data="rows"
            :loading="loading"
            :row-key="(row: ShopProduct) => row.id"
            :bordered="false"
            :pagination="{ pageSize: 20 }"
            :scroll-x="1285"
          />
        </NCard>
      </template>
      <template v-else>
        <NForm label-placement="top" :disabled="saving || readonly">
          <section v-show="section === 'product'">
            <h3 class="mb-20px">商品与消耗</h3>
            <div class="ops-date-grid">
              <NFormItem v-if="selected" label="商品编号"><span class="ops-mono ops-muted">{{ draft.id }}</span></NFormItem>
              <NFormItem v-if="!['vip', 'pet-market', 'union-fight'].includes(shopId) && !isLegacyProduct" label="显示名称">
                <NInput v-model:value="draft.name" placeholder="商品名称" />
              </NFormItem>
            </div>
            <NFormItem label="获得资源" required>
              <RewardList
                v-model:value="draft.rewards"
                :disabled="saving || readonly"
                :resources="shopId === 'union-fight' ? snapshot?.resources || [] : undefined"
                :max="['union-fight', 'vip', 'pet-market'].includes(shopId) || isLegacyProduct ? 1 : 50"
              />
            </NFormItem>
            <template v-if="supports('cost')">
              <NFormItem label="消耗资源" required>
                <div class="resource-cost-line">
                  <ResourceSelect
                    v-model:value="draft.cost.resourceId"
                    :disabled="saving || readonly"
                    commerce-cost
                    :recommended-ids="recommendedCosts"
                    placeholder="搜索可消耗的货币、材料或道具"
                  />
                  <NInputNumber
                    v-model:value="draft.cost.amount"
                    :disabled="readonly || saving"
                    :min="shopId === 'union-fight' ? 0 : 1"
                    :precision="0"
                  />
                </div>
              </NFormItem>
            </template>
            <NFormItem v-if="supports('contributionCost')" label="公会贡献消耗">
              <NInputNumber v-model:value="draft.contributionCost" :min="0" :precision="0" />
            </NFormItem>
            <NFormItem v-if="supports('discountBps')" label="售价比例">
              <NInputNumber
                :value="draft.discountBps / 100"
                :min="0.01"
                :max="100"
                :precision="2"
                @update:value="draft.discountBps = Math.round(($event ?? 100) * 100)"
              >
                <template #suffix>%</template>
              </NInputNumber>
            </NFormItem>
          </section>
          <section v-show="section === 'limits'">
            <h3 class="mb-20px">资格与限购</h3>
            <NGrid cols="1 m:2" responsive="screen" :x-gap="22">
              <NGi v-if="supports('dailyLimit')">
                <NFormItem label="每日限购">
                  <NInputNumber
                    v-model:value="draft.dailyLimit"
                    :disabled="readonly || saving"
                    :min="1"
                    :precision="0"
                  />
                </NFormItem>
              </NGi>
              <NGi v-if="supports('totalLimit')">
                <NFormItem label="累计限购（0 不限）">
                  <NInputNumber v-model:value="draft.totalLimit" :min="0" :precision="0" />
                </NFormItem>
              </NGi>
              <NGi v-if="supports('stock')">
                <NFormItem
                  :label="
                    shopId === 'union-fight'
                      ? '可购买次数'
                      : shopId === 'pet-market'
                        ? '每周库存（1–100）'
                        : '库存（-1 不限）'
                  "
                >
                  <NInputNumber
                    v-model:value="draft.stock"
                    :min="['union-fight', 'pet-market'].includes(shopId) ? 1 : -1"
                    :max="shopId === 'pet-market' ? 100 : undefined"
                    :precision="0"
                  />
                </NFormItem>
              </NGi>
              <NGi v-if="supports('minLevel')">
                <NFormItem label="最低角色等级">
                  <NInputNumber v-model:value="draft.minLevel" :min="1" :precision="0" />
                </NFormItem>
              </NGi>
              <NGi v-if="supports('minVip')">
                <NFormItem label="最低 VIP 等级">
                  <NSelect v-model:value="draft.minVip" :options="vipOptions" />
                </NFormItem>
              </NGi>
              <NGi v-if="supports('guildLevel')">
                <NFormItem label="最低公会等级">
                  <NInputNumber v-model:value="draft.guildLevel" :min="1" :precision="0" />
                </NFormItem>
              </NGi>
              <NGi v-if="supports('buyCondition')">
                <NFormItem label="公会职务资格">
                  <NSelect
                    v-model:value="draft.buyCondition"
                    :options="[
                      { value: 0, label: '全部成员' },
                      { value: 1, label: '副会长 / 会长' },
                      { value: 2, label: '会长' }
                    ]"
                  />
                </NFormItem>
              </NGi>
              <NGi v-if="supports('maxRank')">
                <NFormItem label="可兑换的最末排名">
                  <NInputNumber v-model:value="draft.maxRank" :min="1" :precision="0" />
                  <template #feedback>填写 500，表示历史最高排名进入前 500 名可兑换。</template>
                </NFormItem>
              </NGi>
              <NGi v-if="supports('priority')">
                <NFormItem label="展示优先级"><NInputNumber v-model:value="draft.priority" :precision="0" /></NFormItem>
              </NGi>
              <NGi v-if="supports('weight')">
                <NFormItem label="刷新权重">
                  <NInputNumber v-model:value="draft.weight" :min="1" :precision="0" />
                </NFormItem>
              </NGi>
            </NGrid>
            <NEmpty
              v-if="
                ![
                  'dailyLimit',
                  'totalLimit',
                  'stock',
                  'minLevel',
                  'minVip',
                  'guildLevel',
                  'buyCondition',
                  'maxRank',
                  'priority',
                  'weight'
                ].some(supports)
              "
              description="此商店没有可编辑的资格或限购字段"
            />
          </section>
          <section v-show="section === 'publish'">
            <h3 class="mb-20px">状态与备注</h3>
            <NFormItem v-if="supports('enabled')" label="启用商品">
              <NSwitch v-model:value="draft.enabled" aria-label="启用商品" />
            </NFormItem>
            <NFormItem v-if="supports('day')" label="报价日期">
              <NDatePicker
                :value="draft.day ? new Date(`${draft.day}T00:00:00+08:00`).getTime() : null"
                type="date"
                clearable
                @update:value="draft.day = $event ? new Date($event + 8 * 3600000).toISOString().slice(0, 10) : ''"
              />
            </NFormItem>
            <NFormItem v-if="supports('refreshMillis')" label="刷新间隔">
              <NInputNumber
                :value="draft.refreshMillis / 60000"
                :min="1"
                :max="1440"
                :precision="0"
                @update:value="draft.refreshMillis = ($event || 0) * 60000"
              >
                <template #suffix>分钟</template>
              </NInputNumber>
            </NFormItem>
            <NFormItem v-if="supports('source')" label="配置备注">
              <NInput v-model:value="draft.source" type="textarea" :maxlength="1000" placeholder="选填：记录本次调整用途" />
            </NFormItem>
            <p class="ops-inline-note">修改仅影响后续购买，已购买的订单不变。</p>
          </section>
        </NForm>
      </template>
    </ManagementWorkspace>
    <NModal
      v-model:show="settingsOpen"
      preset="card"
      :title="`商店设置 · ${title}`"
      class="ops-modal shop-settings-modal"
      :mask-closable="!settingsSaving"
      :closable="!settingsSaving"
    >
      <NAlert v-if="settingsError" type="error" :title="settingsError" class="mb-20px" />
      <NAlert
        v-if="needsPeriod"
        type="info"
        title="保存商店设置后即可添加商品。"
        class="mb-20px"
      />
      <NForm label-placement="top" :disabled="settingsSaving">
        <NFormItem v-if="settingSupports('enabled') && !needsPeriod" label="营业状态">
          <NSwitch v-model:value="settingsDraft.enabled" aria-label="营业状态" />
        </NFormItem>
        <NFormItem v-if="settingSupports('slots')" label="展示槽位">
          <NInputNumber
            v-model:value="settingsDraft.slots"
            :min="1"
            :max="16"
            :precision="0"
          />
        </NFormItem>
        <NFormItem v-if="settingSupports('unlimited')" label="不限时">
          <NSwitch v-model:value="settingsDraft.unlimited" aria-label="不限时" />
        </NFormItem>
        <p v-if="!settingsDraft.unlimited" class="ops-muted mb-4">日期留空时暂停营业，也可到活动排期安排营业时段。</p>
        <div v-if="!settingsDraft.unlimited" class="ops-date-grid">
          <NFormItem v-if="settingSupports('startAt')" label="开始时间">
            <NDatePicker
              :value="settingsDraft.startAt || null"
              :disabled="settingsSaving"
              type="datetime"
              clearable
              @update:value="settingsDraft.startAt = $event || 0"
            />
          </NFormItem>
          <NFormItem v-if="settingSupports('endAt')" label="结束时间">
            <NDatePicker
              :value="settingsDraft.endAt || null"
              :disabled="settingsSaving"
              type="datetime"
              clearable
              @update:value="settingsDraft.endAt = $event || 0"
            />
          </NFormItem>
        </div>
        <NFormItem v-if="!settingsDraft.unlimited && settingSupports('claimEndAt')" label="领奖截止">
          <NDatePicker
            :value="settingsDraft.claimEndAt || null"
            :disabled="settingsSaving"
            type="datetime"
            clearable
            @update:value="settingsDraft.claimEndAt = $event || 0"
          />
        </NFormItem>
        <NFormItem v-if="settingSupports('source')" label="配置备注">
          <NInput
            v-model:value="settingsDraft.source"
            :disabled="shopId === 'popularity-rank'"
            type="textarea"
            :maxlength="1000"
          />
        </NFormItem>
      </NForm>
      <div class="ops-drawer-actions">
        <NButton :disabled="settingsSaving" @click="settingsOpen = false">取消</NButton>
        <NButton type="primary" :loading="settingsSaving" @click="saveSettings">
          保存设置
        </NButton>
      </div>
    </NModal>
  </div>
</template>

<style scoped>
.shop-workspace-root {
  width: 100%;
}
.shop-status-line {
  line-height: 24px;
}
.shop-workspace-root :deep(.management-workspace) {
  gap: 12px;
}
.shop-workspace-root :deep(.shop-cell-stack) {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.shop-workspace-root :deep(.shop-cell-stack > *) {
  min-height: 36px;
  display: flex;
  align-items: center;
}
.shop-workspace-root :deep(.shop-resource-name),
.shop-workspace-root :deep(.shop-cost-resource) {
  gap: 7px;
  line-height: 1.45;
}
.shop-workspace-root :deep(.shop-resource-name) {
  flex-wrap: wrap;
}
.shop-workspace-root :deep(.shop-custom-name) {
  margin-bottom: 6px;
  font-size: 12px;
  color: rgb(var(--base-text-color) / .6);
}
.shop-workspace-root :deep(.shop-number-cell) {
  font-variant-numeric: tabular-nums;
}
.shop-workspace-root :deep(.shop-rule-lines) {
  gap: 4px;
  font-size: 12px;
}
.shop-workspace-root :deep(.shop-rule-lines > *) {
  min-height: 0;
}
.shop-workspace-root :deep(.n-data-table-td) {
  padding-top: 10px;
  padding-bottom: 10px;
}
.shop-workspace-root :deep(.n-form-item-label) {
  font-size: 13px;
}
.shop-workspace-root :deep(section > h3) {
  font-size: 14px;
  font-weight: 500;
}
.shop-settings-modal {
  width: 680px;
  max-width: calc(100vw - 32px);
}
</style>
