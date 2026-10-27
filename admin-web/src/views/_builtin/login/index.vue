<script setup lang="ts">
import { reactive, ref } from 'vue';
import type { FormInst } from 'naive-ui';
import { useAuthStore } from '@/store/modules/auth';
const auth = useAuthStore();
const form = ref<FormInst | null>(null);
const model = reactive({ username: '', password: '' });
async function submit(event?: Event) {
  if (event instanceof KeyboardEvent && event.isComposing) return;
  if (auth.loginLoading) return;
  try {
    await form.value?.validate();
  } catch {
    return;
  }
  await auth.login(model.username, model.password);
  model.password = '';
}
</script>

<template>
  <div class="hyjs-login">
    <div class="login-brand">
      <SystemLogo class="text-32px text-primary" />
      <span>灵魂之花</span>
    </div>
    <NCard :bordered="false" class="login-card">
      <h1>登录运营台</h1>
      <NForm
        ref="form"
        :model="model"
        :rules="{
          username: { required: true, message: '请输入账号' },
          password: { required: true, message: '请输入密码' }
        }"
        label-placement="top"
        @submit.prevent="submit"
      >
        <NFormItem label="账号" path="username">
          <NInput
            v-model:value="model.username"
            autocomplete="username"
            placeholder="运营账号"
            :disabled="auth.loginLoading"
          />
        </NFormItem>
        <NFormItem label="密码" path="password">
          <NInput
            v-model:value="model.password"
            type="password"
            show-password-on="click"
            autocomplete="current-password"
            placeholder="请输入密码"
            :disabled="auth.loginLoading"
            @keydown.enter.prevent="submit"
          />
        </NFormItem>
        <NButton type="primary" block attr-type="submit" :loading="auth.loginLoading">登录</NButton>
      </NForm>
    </NCard>
  </div>
</template>

<style scoped>
.hyjs-login {
  min-height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  background: rgb(var(--layout-bg-color));
  padding: 24px;
}
.login-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 24px;
  font-size: 20px;
  font-weight: 600;
}
.login-card {
  width: 420px;
  max-width: 100%;
  border-radius: 6px;
  border: 1px solid rgb(var(--base-text-color) / .08);
}
.login-card :deep(.n-card__content) {
  padding: 24px;
}
h1 {
  font-size: 18px;
  font-weight: 600;
  margin: 0 0 24px;
}
@media (max-width: 480px) {
  .hyjs-login { padding: 16px; }
  .login-card :deep(.n-card__content) { padding: 20px 16px; }
}
</style>
