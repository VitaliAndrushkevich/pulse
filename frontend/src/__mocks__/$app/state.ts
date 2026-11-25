import { vi } from 'vitest';

/** Plain-object stand-in for SvelteKit's `$app/state` outside the Kit runtime. */
export const page = {
  url: new URL('http://localhost/monitors/test-monitor-123'),
  params: { id: 'test-monitor-123' } as Record<string, string | undefined>,
  route: { id: '/monitors/[id]' },
  status: 200,
  error: null,
  data: {},
  form: null,
  state: {}
};

export const navigating = { current: null };
export const updated = { current: false, check: vi.fn() };
