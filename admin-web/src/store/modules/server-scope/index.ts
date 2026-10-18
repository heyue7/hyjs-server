import { computed, ref } from 'vue';
import { defineStore } from 'pinia';
import { getServers } from '@/service/servers';
import type { GameServer } from '@/service/servers';
import { errorMessage } from '@/service/operations';

export const serverScopedRoutes = new Set(['players', 'wallets', 'feedback', 'activity-schedules']);
export const useServerScopeStore = defineStore('server-scope', () => {
  const saved = Number(sessionStorage.getItem('hyjs-admin-server'));
  const serverId = ref<number | null>(Number.isSafeInteger(saved) && saved > 0 ? saved : null);
  const servers = ref<GameServer[]>([]);
  const loading = ref(false);
  const loaded = ref(false);
  const error = ref('');
  const currentServer = computed(() => servers.value.find(row => row.id === serverId.value));
  let pending: Promise<void> | null = null;
  const switchGuards = new Set<() => boolean | Promise<boolean>>();
  let changeGeneration = 0;
  function addSwitchGuard(guard: () => boolean | Promise<boolean>) {
    switchGuards.add(guard);
    return () => switchGuards.delete(guard);
  }
  async function requestServerChange(id: number | null) {
    changeGeneration += 1;
    const generation = changeGeneration;
    if (id === serverId.value) return true;
    const allowed = await [...switchGuards].reduce<Promise<boolean>>(
      (previous, guard) => previous.then(ok => (ok ? guard() : false)),
      Promise.resolve(true)
    );
    if (!allowed || generation !== changeGeneration) return false;
    setServer(id);
    return true;
  }

  function setServer(id: number | null) {
    serverId.value = id && Number.isSafeInteger(id) && id > 0 && servers.value.some(row => row.id === id) ? id : null;
    if (serverId.value) sessionStorage.setItem('hyjs-admin-server', String(serverId.value));
    else sessionStorage.removeItem('hyjs-admin-server');
  }
  async function loadServers() {
    if (pending) return pending;
    loading.value = true;
    error.value = '';
    pending = (async () => {
      try {
        servers.value = (await getServers()).items;
        loaded.value = true;
        if (serverId.value && !servers.value.some(row => row.id === serverId.value)) setServer(null);
      } catch (e) {
        error.value = errorMessage(e);
      } finally {
        loading.value = false;
        pending = null;
      }
    })();
    return pending;
  }
  function serverQuery(path: string) {
    if (!currentServer.value) throw new Error('请先选择区服');
    return `${path}${path.includes('?') ? '&' : '?'}serverId=${currentServer.value.id}`;
  }
  return {
    serverId,
    servers,
    currentServer,
    loading,
    loaded,
    error,
    setServer,
    requestServerChange,
    addSwitchGuard,
    loadServers,
    serverQuery
  };
});
