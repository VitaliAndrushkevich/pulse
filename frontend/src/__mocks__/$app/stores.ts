import { vi } from 'vitest';
import { readable } from 'svelte/store';

export const page = readable({
  url: new URL('http://localhost/monitors/test-monitor-123'),
  params: { id: 'test-monitor-123' },
  route: { id: '/monitors/[id]' },
  status: 200,
  error: null,
  data: {},
  form: null
});

export const navigating = readable(null);
export const updated = { check: vi.fn(), subscribe: readable(false).subscribe };
