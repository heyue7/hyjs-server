<script setup lang="ts">
import { computed, h, reactive, ref, watch } from 'vue';
import type { SelectOption } from 'naive-ui';
import { api, errorMessage, json } from '@/service/operations';
import type { Item, ItemsResponse } from '@/service/operations';
import type { GamePlayerCreate, GamePlayerDetail } from '@/service/player-operations';
import ResourceImage from './resource-image.vue';
const props = defineProps<{ show: boolean }>();
const emit = defineEmits<{ 'update:show': [value: boolean]; created: [player: GamePlayerDetail] }>();
const draft = reactive<GamePlayerCreate & { confirmPassword: string }>({ username: '', password: '', confirmPassword: '', name: '', heroId: 'hero_043', reason: '' });
const saving = ref(false);
const error = ref('');
const heroError = ref('');
const heroes = ref<Item[]>([]);
const heroIds = ['hero_001', 'hero_043', 'hero_044', 'hero_045', 'hero_046', 'hero_047', 'hero_048', 'hero_049', 'hero_050'];
const options = computed(() => heroIds.map(id => ({ value: id, label: heroes.value.find(hero => hero.id === id)?.name || id, item: heroes.value.find(hero => hero.id === id) })));
function reset() { Object.assign(draft, { username: '', password: '', confirmPassword: '', name: '', heroId: 'hero_043', reason: '' }); error.value = ''; }
watch(() => props.show, async show => {
  reset();
  if (!show || heroes.value.length) return;
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
  if (!/^[A-Za-z0-9_-]{3,32}$/.test(draft.username)) return '登录账号需为3–32位字母、数字、下划线或连字符';
  if (Array.from(draft.password).length < 6 || new TextEncoder().encode(draft.password).length > 72) return '密码至少6个字符，UTF-8编码不超过72字节';
  if (draft.password !== draft.confirmPassword) return '两次密码输入不一致';
  if (!draft.name || draft.name.trim() !== draft.name || /[\p{Cc}\p{Cf}]/u.test(draft.name) || nameWidth(draft.name) > 12) return '角色名最多6个汉字或12个英文字符，前后不留空格';
  if (!heroIds.includes(draft.heroId)) return '请选择可用的初始伙伴';
  if (!draft.reason.trim()) return '请填写创建原因';
  return '';
}
async function save() {
  if (saving.value) return;
  error.value = validate();
  if (error.value) return;
  saving.value = true;
  try {
    const result = await api<GamePlayerDetail>('/game-players', json('POST', { username: draft.username, password: draft.password, name: draft.name, heroId: draft.heroId, reason: draft.reason.trim() }));
    reset();
    emit('update:show', false);
    emit('created', result);
    window.$message?.success('游戏用户已创建，可使用新账号登录');
  } catch (e) { error.value = errorMessage(e); }
  finally { saving.value = false; }
}
function renderHero(option: SelectOption) {
  const row = option as SelectOption & { item?: Item };
  return h('div', { class: 'resource-select-option' }, [h(ResourceImage, { item: row.item, size: 32, preview: false }), h('span', String(option.label))]);
}
</script>

<template>
  <NModal :show="show" preset="card" title="新增游戏用户" class="ops-modal game-player-create" :mask-closable="!saving" :closable="!saving" @update:show="!saving && emit('update:show', $event)">
    <NAlert v-if="error" type="error" :title="error" class="mb-20px" />
    <NForm label-placement="top" :disabled="saving" @submit.prevent="save">
      <div class="create-account-section"><h3>登录账号</h3><p>用于进入游戏，创建后不会返回密码。</p></div>
      <NFormItem label="登录账号" required><NInput v-model:value="draft.username" :maxlength="32" autocomplete="off" placeholder="3–32位字母、数字、_ 或 -" /></NFormItem>
      <div class="create-account-grid">
        <NFormItem label="密码" required><NInput v-model:value="draft.password" type="password" show-password-on="click" autocomplete="new-password" placeholder="至少6个字符" /></NFormItem>
        <NFormItem label="确认密码" required><NInput v-model:value="draft.confirmPassword" type="password" show-password-on="click" autocomplete="new-password" placeholder="再次输入密码" /></NFormItem>
      </div>
      <div class="create-account-section"><h3>初始角色</h3><p>按原新手规则创建，不额外发放资源。</p></div>
      <NFormItem label="角色名称" required><NInput v-model:value="draft.name" :maxlength="12" placeholder="最多6个汉字或12个英文字符" /></NFormItem>
      <NFormItem label="初始伙伴" required><NSelect v-model:value="draft.heroId" :options="options" :render-label="renderHero" filterable /></NFormItem>
      <NAlert v-if="heroError" type="warning" :title="heroError" class="mb-18px" />
      <NFormItem label="创建原因" required><NInput v-model:value="draft.reason" type="textarea" :maxlength="300" placeholder="记录此次创建的用途" /></NFormItem>
      <div class="ops-drawer-actions mt-12px"><NButton :disabled="saving" @click="emit('update:show', false)">取消</NButton><NButton type="primary" :loading="saving" attr-type="submit">创建游戏用户</NButton></div>
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
