import { computed, ref } from 'vue';
import { errorMessage } from '@/service/operations';
import { useAuthStore } from '@/store/modules/auth';
export function useOperations() {
  const auth = useAuthStore();
  const canWrite = computed(() => auth.userInfo.roles.some(role => ['operator', 'admin'].includes(role)));
  const loading = ref(false);
  const error = ref('');
  async function run<T>(fn: () => Promise<T>): Promise<T | undefined> {
    loading.value = true;
    error.value = '';
    try {
      return await fn();
    } catch (e) {
      error.value = errorMessage(e);
      return undefined;
    } finally {
      loading.value = false;
    }
  }
  return { canWrite, loading, error, run };
}
