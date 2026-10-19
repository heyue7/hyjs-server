<script setup lang="ts">
import { computed, h, reactive, ref, watch } from 'vue';
import type { SelectOption } from 'naive-ui';
import { api, errorMessage, json } from '@/service/operations';
import type { Item, ItemsResponse } from '@/service/operations';
import type { GamePlayerCreate, GamePlayerDetail } from '@/service/player-operations';
import ResourceImage from './resource-image.vue';
import { getGameAccounts } from '@/service/game-accounts';
import type { GameAccount } from '@/service/game-accounts';
import { useServerScopeStore } from '@/store/modules/server-scope';
const scope = useServerScopeStore();
const props = defineProps<{ show: boolean }>();
const emit = defineEmits<{ 'update:show': [value: boolean]; created: [player: GamePlayerDetail] }>();
const draft = reactive<GamePlayerCreate>({ accountId: null, name: '', heroId: 'hero_043', reason: '' });
const accounts = ref<GameAccount[]>([]);
const accountLoading = ref(false);
let accountGeneration = 0;
const accountOptions = computed(() => accounts.value.filter(row => row.enabled && !row.protected).map(row => ({ value: row.id, label: `${row.nickname} · ${row.username}` })));
async function searchAccounts(q = '') {
 const seq = ++accountGeneration; accountLoading.value = true;
 try { const data = await getGameAccounts(q,1,100); if(seq === accountGeneration) accounts.value = data.items; }
 catch(e) { if(seq === accountGeneration) error.value = errorMessage(e); }
 finally { if(seq === accountGeneration) accountLoading.value = false; }
}
function chooseAccount(id: number) {
 const user = accounts.value.find(row => row.id === id);
 if(user && !draft.name) draft.name = Array.from(user.nickname).slice(0,6).join('');
}
const saving = ref(false);
const error = ref('');
const heroError = ref('');
const heroes = ref<Item[]>([]);
const heroIds = ['hero_001', 'hero_043', 'hero_044', 'hero_045', 'hero_046', 'hero_047', 'hero_048', 'hero_049', 'hero_050'];
const options = computed(() => heroIds.map(id => ({ value: id, label: heroes.value.find(hero => hero.id === id)?.name || id, item: heroes.value.find(hero => hero.id === id) })));
function reset() { Object.assign(draft, { accountId: null, name: '', heroId: 'hero_043', reason: '' }); error.value = ''; }
watch(() => props.show, async show => {
  reset();
  if (!show) { accountGeneration += 1; return; }
  await searchAccounts();
  if (heroes.value.length) return;
  heroError.value = '';
  try { heroes.value = (await api<ItemsResponse>(`/items?${new URLSearchParams({ ids: heroIds.join(','), page: '1', pageSize: '100' })}`)).items; }
  catch (e) { heroError.value = errorMessage(e); }
});
function nameWidth(value: string) {
  let width = 0;
  for (let i = 0; i < value.length; i += 1) width += value.charCodeAt(i) <= 256 ? 1 : 2;
  return width;
}
function validate() {
  if (!draft.accountId) return '请选择全局用户账号';
  if (!draft.name || draft.name.trim() !== draft.name || /[\p{Cc}\p{Cf}]/u.test(draft.name) || nameWidth(draft.name) > 12) return '角色名最多6个汉字或12个英文字符，前后不留空格';
  if (!heroIds.includes(draft.heroId)) return '请选择可用的初始伙伴';
  if (!draft.reason.trim()) return '请填写创建原因';
  return '';
}
async function save() {
  if (saving.value) return;
  if (!scope.currentServer?.runtimeBound || !scope.currentServer.enabled) { error.value = '请选择已部署并开放的区服'; return; }
  error.value = validate();
  if (error.value) return;
  saving.value = true;
  try {
    const result = await api<GamePlayerDetail>(scope.serverQuery('/game-players'), json('POST', { serverId: scope.serverId, accountId: draft.accountId, name: draft.name, heroId: draft.heroId, reason: draft.reason.trim() }));
    reset();
    emit('update:show', false);
    emit('created', result);
    window.$message?.success('本服玩家已创建');
  } catch (e) { error.value = errorMessage(e); }
  finally { saving.value = false; }
}
function renderHero(option: SelectOption) {
  const row = option as SelectOption & { item?: Item };
  return h('div', { class: 'resource-select-option' }, [h(ResourceImage, { item: row.item, size: 32, preview: false }), h('span', String(option.label))]);
}
</script>

<template>
  <NModal :show="show" preset="card" title="新增本服玩家" class="ops-modal game-player-create" :mask-closable="!saving" :closable="!saving" @update:show="!saving && emit('update:show', $event)">
    <NAlert v-if="error" type="error" :title="error" class="mb-20px" />
    <NForm label-placement="top" :disabled="saving" @submit.prevent="save">
      <NFormItem label="所属用户" required><NSelect v-model:value="draft.accountId" :options="accountOptions" :loading="accountLoading" filterable remote placeholder="搜索账号或昵称" @search="searchAccounts" @update:value="chooseAccount" /></NFormItem>
      <p class="ops-inline-note mb-4">选择已有全局账号，在当前区服建立独立角色。新账号请先到用户管理创建。</p>
      <NFormItem label="角色名称" required><NInput v-model:value="draft.name" :maxlength="12" placeholder="最多6个汉字或12个英文字符" /></NFormItem>
      <NFormItem label="初始伙伴" required><NSelect v-model:value="draft.heroId" :options="options" :render-label="renderHero" filterable /></NFormItem>
      <NAlert v-if="heroError" type="warning" :title="heroError" class="mb-18px" />
      <NFormItem label="创建原因" required><NInput v-model:value="draft.reason" type="textarea" :maxlength="300" placeholder="记录此次创建的用途" /></NFormItem>
      <div class="ops-drawer-actions mt-12px"><NButton :disabled="saving" @click="emit('update:show', false)">取消</NButton><NButton type="primary" :loading="saving" attr-type="submit">创建本服玩家</NButton></div>
    </NForm>
  </NModal>
</template>

<style scoped>
.game-player-create { width: 640px; }
.create-account-section { margin: 4px 0 20px; }
.create-account-section h3 { font-size: 15px; font-weight: 600; margin: 0 0 6px; }
.create-account-section p { margin: 0; font-size: 12px; color: rgb(var(--base-text-color) / .52); }
.create-account-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 18px; }
@media (max-width: 600px) { .create-account-grid { grid-template-columns: 1fr; gap: 0; } }
</style>
