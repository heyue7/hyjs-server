import { computed, onBeforeUnmount, ref } from 'vue';
import { onBeforeRouteLeave } from 'vue-router';
import { useServerScopeStore } from '@/store/modules/server-scope';
import type { DialogReactive } from 'naive-ui';

/** Keep a draft in memory. Call capture after explicitly opening/resetting it. */
export function useDraftGuard(options: {
  active: () => boolean;
  value: () => unknown;
  busy: () => boolean;
  close: () => void;
  writable?: () => boolean;
  label?: string;
  route?: boolean;
  scope?: boolean;
}) {
  const baseline = ref('');
  const contents = () => JSON.stringify(options.value()) ?? '';
  const dirty = computed(() => options.active() && (options.writable?.() ?? true) && contents() !== baseline.value);
  function capture() { baseline.value = contents(); }
  function clear() { baseline.value = ''; }
  let pending: { promise: Promise<boolean>; dispose: () => void } | null = null;
  function confirmDiscard() {
    if (pending) return pending.promise;
    let resolveConfirmation!: (value: boolean) => void;
    const promise = new Promise<boolean>(resolve => { resolveConfirmation = resolve; });
    let accepted = false;
    let settled = false;
    const settle = (value: boolean) => { if (!settled) { settled = true; pending = null; resolveConfirmation(value); } };
    let dialog: DialogReactive;
    const dismiss = (value: boolean) => { accepted = value; dialog.destroy(); return false; };
    if (!window.$dialog) return Promise.resolve(false);
    dialog = window.$dialog.warning({
      title: `放弃未保存的${options.label || '修改'}？`,
      positiveText: '放弃', negativeText: '继续编辑',
      onPositiveClick: () => dismiss(true), onNegativeClick: () => dismiss(false),
      onClose: () => dismiss(false), onMaskClick: () => dismiss(false), onEsc: () => dismiss(false),
      // Let the confirmation leave before closing a parent editor or changing routes.
      onAfterLeave: () => settle(accepted)
    });
    pending = { promise, dispose: () => { dialog.destroy(); settle(false); } };
    return promise;
  }
  async function canLeave() {
    if (!options.active()) return true;
    if (options.busy()) return false;
    if (!dirty.value) return true;
    const confirmed = await confirmDiscard();
    return confirmed && (!options.active() || !options.busy());
  }
  async function requestClose() {
    if (!await canLeave()) return false;
    if (options.busy()) return false;
    options.close();
    clear();
    return true;
  }
  function beforeUnload(event: BeforeUnloadEvent) {
    if (!options.active()) return;
    if (!options.busy() && !dirty.value) return;
    event.preventDefault(); event.returnValue = '';
  }
  window.addEventListener('beforeunload', beforeUnload);
  onBeforeUnmount(() => { pending?.dispose(); clear(); window.removeEventListener('beforeunload', beforeUnload); });
  if (options.route !== false) onBeforeRouteLeave(canLeave);
  if (options.scope !== false) {
    const removeGuard = useServerScopeStore().addSwitchGuard(async () => {
      if (!await canLeave() || (options.active() && options.busy())) return false;
      if (options.active()) { options.close(); clear(); }
      return true;
    });
    onBeforeUnmount(removeGuard);
  }
  return { dirty, capture, clear, canLeave, requestClose };
}
