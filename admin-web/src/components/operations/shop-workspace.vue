<script setup lang="ts">
import { computed, h, onBeforeUnmount, ref, watch } from 'vue';
import { onBeforeRouteLeave, useRouter } from 'vue-router';
import { NButton, NSpace, NTag } from 'naive-ui';
import type { DataTableColumns } from 'naive-ui';
import { api, errorMessage, formatTime, json } from '@/service/operations';
import { newShopProduct, shopLabels } from '@/service/management-operations';
import type { ShopDefinition, ShopProduct, ShopSnapshot } from '@/service/management-operations';
import type { Item, ItemsResponse } from '@/service/operations';
import { useOperations } from '@/composables/useOperations';
import ManagementWorkspace from './management-workspace.vue';
import RewardList from './reward-list.vue';
import RewardSummary from './reward-summary.vue';
import ResourceSelect from './resource-select.vue';
const props = defineProps<{ shopId: string }>();
const router = useRouter();
const isPopularityShop = computed(() => ['popularity', 'popularity-rank'].includes(props.shopId));
const { canWrite, loading, error, run } = useOperations();
const definition = ref<ShopDefinition>();
const snapshot = ref<ShopSnapshot>();
const currencyItems = ref<Item[]>([]);
const q = ref('');
const state = ref<string | null>(null);
const editing = ref(false);
const selected = ref<ShopProduct | null>(null);
const draft = ref<ShopProduct>(newShopProduct());
const original = ref('');
const saving = ref(false);
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
const settingsFrozen = computed(() => isFrozen.value && !settingsDraft.value.newPeriod);
const isFrozen = computed(() => Boolean(snapshot.value?.settings.frozen));
const needsPeriod = computed(
  () => !['vip', 'union-fight', 'pet-market'].includes(props.shopId) && !snapshot.value?.settings.periodId
);
const productReadonly = (row: ShopProduct | null) =>
  !canWrite.value || (isFrozen.value && (!row || row.id.startsWith('period:')));
const readonly = computed(() => productReadonly(selected.value));
const fixedCurrency = computed(
  () =>
    (
      ({
        vip: { id: 'diamond', name: '钻石' },
        'union-fight': { id: 'unionFightToken', name: '公会战代币' }
      }) as Record<string, { id: string; name: string }>
    )[props.shopId]
);
const fixedPrice = computed(() => props.shopId === 'ordinary' && Boolean(selected.value?.id.startsWith('legacy:')));
const petCurrencies = computed<Item[]>(() =>
  ['coinpet', 'diamond'].map(
    id =>
      currencyItems.value.find(item => item.id === id) || {
        id,
        name: id === 'coinpet' ? '精灵币' : '钻石',
        category: 'currency'
      }
  )
);
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
const supports = (key: string) =>
  fields.value.includes(key) &&
  (!fixedPrice.value || ['rewards', 'cost', 'dailyLimit', 'minLevel', 'minVip', 'priority', 'enabled'].includes(key));
const hasPublishFields = computed(() =>
  (supports('enabled') && (props.shopId !== 'ordinary' || fixedPrice.value)) ||
  ['day', 'refreshMillis', 'source'].some(supports)
);
const showShopStatus = computed(() => Boolean(snapshot.value && (
  settingSupports('unlimited') || settingSupports('enabled') || snapshot.value.settings.slots || snapshot.value.settings.startAt || snapshot.value.settings.endAt
)));
const dirty = computed(() => editing.value && JSON.stringify(draft.value) !== original.value);
const rows = computed(() =>
  (snapshot.value?.items || []).filter(
    row =>
      (state.value === null || row.enabled === (state.value === 'enabled')) &&
      `${row.name} ${row.id} ${row.rewards.map(r => r.resourceId).join(' ')}`
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
    const currencies =
      props.shopId === 'pet-market'
        ? await api<ItemsResponse>('/items?ids=coinpet,diamond&page=1&pageSize=100')
        : undefined;
    return { directory, data, currencies };
  });
  if (result && current === generation) {
    definition.value = result.directory.items.find(row => row.id === props.shopId);
    snapshot.value = { ...result.data, items: result.data.items || [] };
    currencyItems.value = result.currencies?.items || [];
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
  if (!row && fixedCurrency.value) draft.value.cost.resourceId = fixedCurrency.value.id;
  original.value = JSON.stringify(draft.value);
  section.value = 'product';
  saveError.value = '';
  editing.value = true;
}
function beginCreate() {
  if (needsPeriod.value) configure(false);
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
function configure(newPeriod = false) {
  if (!snapshot.value) return;
  settingsDraft.value = {
    ...snapshot.value.settings,
    source: snapshot.value.settings.source || '用户运营配置',
    newPeriod,
    ...(newPeriod
      ? { enabled: false, startAt: Math.max(Date.now(), snapshot.value.settings.startAt + 1), endAt: 0, claimEndAt: 0 }
      : {})
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
    window.$message?.success(settingsDraft.value.newPeriod ? '已复制为关闭的新一期，可继续编辑商品' : '商店设置已保存');
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
const columns = computed<DataTableColumns<ShopProduct>>(() => [
  ...(props.shopId === 'popularity-rank'
    ? [
        {
          title: '排名资格',
          key: 'maxRank',
          minWidth: 135,
          render: (row: ShopProduct) => (row.maxRank > 0 ? `前 ${row.maxRank.toLocaleString()} 名` : '待配置')
        }
      ]
    : []),
  {
    title: '商品',
    key: 'name',
    minWidth: 280,
    render: row =>
      h('div', [
        h('strong', row.name || '未命名商品'),
        h(RewardSummary, {
          rewards: row.rewards,
          resources: props.shopId === 'union-fight' ? snapshot.value?.resources || [] : undefined
        })
      ])
  },
  {
    title: '消耗',
    key: 'cost',
    minWidth: 190,
    render: row =>
      h('div', [
        ...(props.shopId === 'union-fight'
          ? [h('span', `公会战代币 × ${row.cost?.amount || 0}`)]
          : row.cost?.resourceId
            ? [
                h(RewardSummary, {
                  rewards: [row.cost],
                  resources: props.shopId === 'pet-market' ? petCurrencies.value : undefined
                })
              ]
            : []),
        ...(row.contributionCost > 0
          ? [h('div', { class: 'ops-muted mt-5px' }, `公会贡献 × ${row.contributionCost}`)]
          : []),
        ...(!row.cost?.resourceId && !row.contributionCost ? [h('span', '未配置消耗')] : [])
      ])
  },
  {
    title: '状态',
    key: 'enabled',
    width: 100,
    render: row =>
      h(
        NTag,
        {
          type: row.enabled ? 'success' : 'default',
          bordered: false,
          size: 'small'
        },
        () => (row.enabled ? '启用' : '关闭')
      )
  },
  {
    title: '操作',
    key: 'action',
    width: 155,
    render: row =>
      h(NSpace, { size: 5 }, () => [
        h(NButton, { size: 'small', secondary: true, onClick: () => edit(row) }, () =>
          productReadonly(row) ? '查看' : '编辑'
        ),
        ...(!productReadonly(row)
          ? [
              h(
                NButton,
                {
                  size: 'small',
                  quaternary: true,
                  type: 'error',
                  onClick: () => remove(row)
                },
                () => '移除'
              )
            ]
          : [])
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
      <template v-if="!editing" #actions>
        <NButton :loading="loading" @click="load">刷新</NButton>
        <NButton v-if="canWrite && snapshot && definition?.settingsFields?.length" @click="configure(false)">
          商店设置
        </NButton>
        <NButton v-if="canWrite && isFrozen" type="primary" @click="configure(true)">复制为新一期</NButton>
        <NButton
          v-else-if="canWrite && definition && definition.canCreate !== false"
          type="primary"
          @click="beginCreate"
        >
          新建商品
        </NButton>
      </template>
      <template #alert>
        <NAlert v-if="error || saveError" type="error" :title="saveError || error" />
        <NAlert
          v-if="isFrozen"
          type="info"
          title="本期已有购买记录，商品价格已锁定。请复制为新一期后调整。"
        />
      </template>
      <template v-if="!editing">
        <details v-if="definition?.notes" class="ops-help"><summary>商店规则</summary><p>{{ definition.notes }}</p></details>
        <NCard v-if="snapshot && showShopStatus" :bordered="false" class="ops-panel">
          <NSpace align="center" justify="space-between">
            <NSpace>
              <NTag
                v-if="settingSupports('enabled')"
                :type="snapshot.settings.enabled ? 'success' : 'default'"
                :bordered="false"
              >
                {{ snapshot.settings.enabled ? '当前配置已开放' : '当前配置已关闭' }}
              </NTag>
              <span v-if="snapshot.settings.slots" class="ops-muted">展示 {{ snapshot.settings.slots }} 个槽位</span>
              <NTag v-if="snapshot.settings.unlimited" :bordered="false" size="small">不限时</NTag>
              <span v-else-if="snapshot.settings.startAt || snapshot.settings.endAt" class="ops-muted">
                {{ formatTime(snapshot.settings.startAt) }} →
                {{ snapshot.settings.endAt ? formatTime(snapshot.settings.endAt) : '不限' }}
              </span>
            </NSpace>
            <span class="ops-muted">{{ snapshot.items.length }} 件商品</span>
          </NSpace>
        </NCard>
        <NCard :bordered="false" class="ops-panel">
          <NTabs v-if="isPopularityShop" :value="shopId" type="line" class="mb-20px" @update:value="router.push(`/shops/${$event}`)">
            <NTab name="popularity">声望商店商品</NTab>
            <NTab name="popularity-rank">声望商店奖励</NTab>
          </NTabs>
          <div class="ops-filters">
            <NInput v-model:value="q" class="ops-search" clearable placeholder="搜索商品名称或编号" />
            <span v-if="snapshot && !showShopStatus" class="ops-muted">{{ snapshot.items.length }} 件商品</span>
            <NSelect
              v-model:value="state"
              clearable
              :options="[
                { label: '启用', value: 'enabled' },
                { label: '关闭', value: 'disabled' }
              ]"
              placeholder="全部状态"
              class="ops-filter-select"
            />
          </div>
          <NDataTable
            :columns="columns"
            :data="rows"
            :loading="loading"
            :row-key="(row: ShopProduct) => row.id"
            :bordered="false"
            :pagination="{ pageSize: 20 }"
            :scroll-x="800"
          />
        </NCard>
      </template>
      <template v-else>
        <NForm label-placement="top" :disabled="saving || readonly">
          <section v-show="section === 'product'">
            <h3 class="mb-20px">商品与消耗</h3>
            <NAlert
              v-if="fixedPrice"
              type="info"
              title="此商品使用固定售价和每日限购，暂不支持修改。"
              class="mb-20px"
            />
            <div class="ops-date-grid">
              <NFormItem v-if="selected" label="商品编号"><span class="ops-mono ops-muted">{{ draft.id }}</span></NFormItem>
              <NFormItem v-if="!['vip', 'pet-market', 'union-fight'].includes(shopId) && !fixedPrice" label="显示名称">
                <NInput v-model:value="draft.name" placeholder="商品名称" />
              </NFormItem>
            </div>
            <NFormItem label="获得资源" required>
              <RewardList
                v-model:value="draft.rewards"
                :disabled="saving || readonly"
                :resources="shopId === 'union-fight' ? snapshot?.resources || [] : undefined"
                :max="['union-fight', 'vip', 'pet-market'].includes(shopId) || fixedPrice ? 1 : 50"
              />
            </NFormItem>
            <template v-if="supports('cost')">
              <NFormItem label="消耗资源" required>
                <div class="resource-cost-line">
                  <NInput v-if="fixedCurrency" :value="fixedCurrency.name" readonly />
                  <ResourceSelect
                    v-else
                    v-model:value="draft.cost.resourceId"
                    :disabled="saving || readonly || fixedPrice"
                    :resources="shopId === 'pet-market' ? petCurrencies : undefined"
                  />
                  <NInputNumber
                    v-model:value="draft.cost.amount"
                    :disabled="readonly || saving || fixedPrice"
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
            <p class="ops-muted mb-20px">仅显示此商店实际支持的资格和库存规则。</p>
            <NGrid cols="1 m:2" responsive="screen" :x-gap="22">
              <NGi v-if="supports('dailyLimit')">
                <NFormItem label="每日限购">
                  <NInputNumber
                    v-model:value="draft.dailyLimit"
                    :disabled="readonly || saving || fixedPrice"
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
                  <template #feedback>填写 500 表示前 500 名可兑换；消费已发布的竞技排名资格。</template>
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
            <NFormItem v-if="supports('enabled') && (shopId !== 'ordinary' || fixedPrice)" label="商品状态">
              <NSwitch v-model:value="draft.enabled">
                <template #checked>启用</template>
                <template #unchecked>关闭</template>
              </NSwitch>
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
      :title="settingsDraft.newPeriod ? '复制为新一期' : `商店设置 · ${title}`"
      class="ops-modal shop-settings-modal"
      :mask-closable="!settingsSaving"
      :closable="!settingsSaving"
    >
      <NAlert v-if="settingsError" type="error" :title="settingsError" class="mb-20px" />
      <NAlert
        v-if="settingsDraft.newPeriod"
        type="info"
        title="复制后保持关闭，商品按新期编辑。请重新确认生效时间与报价。"
        class="mb-20px"
      />
      <NAlert
        v-if="needsPeriod && !settingsDraft.newPeriod"
        type="info"
        title="保存商店设置后即可添加商品。"
        class="mb-20px"
      />
      <NForm label-placement="top" :disabled="settingsSaving">
        <p v-if="settingsDraft.periodId" class="ops-muted mb-20px">
          当前期
          <span class="ops-mono">{{ settingsDraft.periodId }}</span>
        </p>
        <NFormItem v-if="settingSupports('enabled') && !settingsDraft.newPeriod && !needsPeriod" label="开放状态">
          <NSwitch v-model:value="settingsDraft.enabled" />
        </NFormItem>
        <NFormItem v-if="settingSupports('slots')" label="展示槽位">
          <NInputNumber
            v-model:value="settingsDraft.slots"
            :disabled="settingsFrozen"
            :min="1"
            :max="16"
            :precision="0"
          />
        </NFormItem>
        <NFormItem v-if="settingSupports('unlimited')" label="不限时">
          <NSwitch v-model:value="settingsDraft.unlimited" />
          <span class="ml-3 ops-muted">持续营业，原商品资格、库存与限购继续生效</span>
        </NFormItem>
        <p v-if="!settingsDraft.unlimited" class="ops-muted mb-4">日期留空时暂停营业，也可到活动排期安排营业时段。</p>
        <div v-if="!settingsDraft.unlimited" class="ops-date-grid">
          <NFormItem v-if="settingSupports('startAt') || settingsDraft.newPeriod" label="开始时间">
            <NDatePicker
              :value="settingsDraft.startAt || null"
              :disabled="settingsSaving"
              type="datetime"
              clearable
              @update:value="settingsDraft.startAt = $event || 0"
            />
          </NFormItem>
          <NFormItem v-if="settingSupports('endAt') || settingsDraft.newPeriod" label="结束时间">
            <NDatePicker
              :value="settingsDraft.endAt || null"
              :disabled="settingsSaving"
              type="datetime"
              clearable
              @update:value="settingsDraft.endAt = $event || 0"
            />
          </NFormItem>
        </div>
        <NFormItem v-if="!settingsDraft.unlimited && (settingSupports('claimEndAt') || settingsDraft.newPeriod)" label="领奖截止">
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
            :disabled="settingsFrozen || shopId === 'popularity-rank'"
            type="textarea"
            :maxlength="1000"
          />
        </NFormItem>
      </NForm>
      <div class="ops-drawer-actions">
        <NButton :disabled="settingsSaving" @click="settingsOpen = false">取消</NButton>
        <NButton type="primary" :loading="settingsSaving" @click="saveSettings">
          {{ settingsDraft.newPeriod ? '创建新一期' : '保存设置' }}
        </NButton>
      </div>
    </NModal>
  </div>
</template>

<style scoped>
.shop-workspace-root {
  width: 100%;
}
.shop-settings-modal {
  width: 680px;
  max-width: calc(100vw - 32px);
}
</style>
