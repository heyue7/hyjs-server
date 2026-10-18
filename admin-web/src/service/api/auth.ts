import { api, errorMessage, json, setCsrfToken } from '../operations';
import type { Session } from '../operations';
// The compatibility token is a non-secret UI marker. Authentication belongs to the HttpOnly Go session cookie.
export async function fetchLogin(userName: string, password: string) {
  try {
    const session = await api<Session>('/auth/login', json('POST', { username: userName, password }));
    setCsrfToken(session.csrfToken);
    return { data: { token: 'cookie-session', refreshToken: '' }, error: null };
  } catch (error) {
    window.$message?.error(errorMessage(error));
    return { data: null, error };
  }
}
export async function fetchGetUserInfo() {
  try {
    const session = await api<Session>('/auth/me');
    setCsrfToken(session.csrfToken);
    return {
      data: { userId: session.user.username, userName: session.user.username, roles: [session.user.role], buttons: [] },
      error: null
    };
  } catch (error) {
    return { data: null, error };
  }
}
export async function fetchRefreshToken(_refreshToken: string) {
  return { data: { token: '', refreshToken: '' }, error: new Error('请重新登录') };
}
export async function fetchCustomBackendError(_code: string, _msg: string) {
  return { data: null, error: new Error('不提供演示接口') };
}
