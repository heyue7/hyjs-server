<script setup lang="ts">
import { computed, ref } from 'vue';

const props = defineProps<{ modelValue: string; label: string; disabled?: boolean }>();
const emit = defineEmits<{ 'update:modelValue': [value: string] }>();
const inputError = ref('');
const numericValue = computed(() => /^\d+$/.test(props.modelValue) ? Number(props.modelValue) : null);
const invalidNumber = computed(() => props.modelValue !== '' && !/^(0|[1-9]\d{0,2})$/.test(props.modelValue));
const canStep = computed(() => props.modelValue === '' || !invalidNumber.value);

function preventNonDigit(event: InputEvent) {
  if (event.data && /\D/.test(event.data)) {
    event.preventDefault();
    inputError.value = '只能输入数字';
  }
}
function onPaste(event: ClipboardEvent) {
  const value = event.clipboardData?.getData('text') || '';
  if (value && /\D/.test(value)) {
    event.preventDefault();
    inputError.value = '只能输入数字';
  }
}
function onInput(event: Event) {
  const input = event.target as HTMLInputElement;
  if (/\D/.test(input.value)) {
    input.value = props.modelValue;
    inputError.value = '只能输入数字';
    return;
  }
  inputError.value = '';
  emit('update:modelValue', input.value);
}
function step(delta: number) {
  if (props.disabled || !canStep.value) return;
  const value = numericValue.value;
  const next = value === null ? 0 : value + delta;
  if (next < 0 || next > 999) return;
  inputError.value = '';
  emit('update:modelValue', String(next));
}
</script>

<template>
  <div class="version-segment">
    <label :for="`version-${label}`">{{ label }}</label>
    <div class="segment-control" :class="{ disabled, invalid: inputError || invalidNumber }">
      <input
        :id="`version-${label}`"
        :value="modelValue"
        :disabled="disabled"
        :aria-label="label"
        :aria-valuenow="numericValue !== null && !invalidNumber ? numericValue : undefined"
        aria-valuemin="0"
        aria-valuemax="999"
        :aria-invalid="inputError || invalidNumber ? 'true' : undefined"
        :aria-describedby="inputError || invalidNumber ? `client-version-help version-${label}-error` : 'client-version-help'"
        aria-required="true"
        role="spinbutton"
        type="text"
        inputmode="numeric"
        pattern="[0-9]*"
        autocomplete="off"
        @beforeinput="preventNonDigit"
        @paste="onPaste"
        @input="onInput"
        @keydown.up.prevent="step(1)"
        @keydown.down.prevent="step(-1)"
      >
      <div class="segment-stepper">
        <button type="button" tabindex="-1" :aria-label="`${label}加 1`" :disabled="disabled || !canStep || numericValue === 999" @click="step(1)">▲</button>
        <button type="button" tabindex="-1" :aria-label="`${label}减 1`" :disabled="disabled || !canStep || numericValue === 0" @click="step(-1)">▼</button>
      </div>
    </div>
    <span v-if="inputError || invalidNumber" :id="`version-${label}-error`" class="segment-error" role="alert">{{ inputError || '须为 0 至 999 的整数，且不含前导零' }}</span>
  </div>
</template>

<style scoped>
.version-segment { min-width: 0; }
.version-segment label { display: block; margin-bottom: 5px; font-size: 12px; line-height: 1.4; color: rgb(var(--base-text-color) / .7); }
.segment-control { display: flex; height: 52px; overflow: hidden; border: 1px solid rgb(var(--base-text-color) / .28); border-radius: 4px; background: rgb(var(--container-bg-color)); }
.segment-control.invalid { border-color: rgb(var(--error-color)); }
.segment-control:focus-within { border-color: rgb(var(--primary-color)); box-shadow: 0 0 0 2px rgb(var(--primary-color) / .18); }
.segment-control.disabled { opacity: .55; }
.segment-control input { width: 100%; min-width: 0; padding: 0 8px; border: 0; outline: 0; background: transparent; color: rgb(var(--base-text-color)); font: inherit; font-variant-numeric: tabular-nums; }
.segment-stepper { display: flex; flex: 0 0 32px; flex-direction: column; border-left: 1px solid rgb(var(--base-text-color) / .18); }
.segment-stepper button { flex: 1 0 24px; min-height: 24px; padding: 0; border: 0; background: transparent; color: rgb(var(--base-text-color) / .72); font-size: 10px; line-height: 1; cursor: pointer; }
.segment-stepper button + button { border-top: 1px solid rgb(var(--base-text-color) / .18); }
.segment-stepper button:hover:not(:disabled) { background: rgb(var(--base-text-color) / .08); }
.segment-stepper button:focus-visible { outline: 2px solid rgb(var(--primary-color)); outline-offset: -2px; }
.segment-stepper button:disabled { opacity: .3; cursor: not-allowed; }
.segment-error { display: block; margin-top: 4px; color: rgb(var(--error-color)); font-size: 12px; }
</style>
