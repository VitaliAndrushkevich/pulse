import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiRequest } from '../api';
import { clearToken, getToken, getUser, setSession } from '../stores/auth.svelte';
import type { User } from '../types';
const user: User = { id: '1', email: 'admin@example.com', role: 'admin', enabled: true, must_change_password: false, created_at: '' };
afterEach(() => { clearToken(); vi.unstubAllGlobals(); });
describe('session replacement while requests are pending', () => {
  for (const [status, code] of [[401, 'UNAUTHORIZED'], [403, 'PASSWORD_CHANGE_REQUIRED']] as const) {
    it(`ignores stale ${code} for the fresh session`, async () => {
      setSession({ token: 'old', user });
      let finish!: (value: Response) => void;
      vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>((resolve) => { finish = resolve; })));
      const pending = apiRequest('GET', '/monitors', undefined, { skipToast: true });
      const rejection = expect(pending).rejects.toHaveProperty('statusCode', status);
      setSession({ token: 'new', user });
      finish(new Response(JSON.stringify({ error: { code, message: 'Expired' } }), { status }));
      await rejection;
      expect(getToken()).toBe('new');
      expect(getUser()?.must_change_password).toBe(false);
    });
  }
});
