import { api, errorMessage, json } from '../operations';

// Keep Soybean's flat-result signature for its optional route adapter. All requests use the same Go cookie contract.
interface RequestConfig { url: string; method?: string; data?: unknown; params?: Record<string, string | number | boolean>; }
type Result<T> = { data: T; error: null } | { data: null; error: Error };
export async function request<T = unknown>(config: RequestConfig): Promise<Result<T>> {
  const params = config.params ? new URLSearchParams(Object.entries(config.params).map(([key, value]) => [key, String(value)])) : null;
  const url = config.url + (params?.size ? `?${params}` : '');
  try {
    const options = config.data === undefined ? { method: config.method || 'GET' } : json(config.method || 'POST', config.data);
    return { data: await api<T>(url, options), error: null };
  } catch (error) { return { data: null, error: new Error(errorMessage(error)) }; }
}
