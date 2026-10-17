import { describe, expect, it } from 'vitest';
import { canAccessRoute } from '../access';
import type { User } from '../types';

const viewer: User = { id: 'viewer', email: 'viewer@example.com', role: 'read_only', enabled: true, must_change_password: false, created_at: '' };
describe('route permissions', () => {
  it('allows readers to inspect monitoring and personal settings', () => {
    for (const path of ['/', '/monitors', '/monitors/id', '/heartbeats', '/heartbeats/id', '/settings']) {
      expect(canAccessRoute(viewer, path), path).toBe(true);
    }
  });
  it('denies direct navigation to administrative pages', () => {
    for (const path of ['/monitors/create', '/monitors/id/edit', '/heartbeats/create', '/heartbeats/id/edit', '/notifications', '/notifications/id/edit']) {
      expect(canAccessRoute(viewer, path), path).toBe(false);
      expect(canAccessRoute({ ...viewer, role: 'admin' }, path), path).toBe(true);
    }
  });
  it('restricts first login to changing password even for administrators', () => {
    for (const role of ['admin', 'read_only'] as const) {
      const user = { ...viewer, role, must_change_password: true };
      expect(canAccessRoute(user, '/change-password')).toBe(true);
      expect(canAccessRoute(user, '/')).toBe(false);
      expect(canAccessRoute(user, '/settings')).toBe(false);
    }
  });
  it('denies disabled and unresolved sessions', () => {
    expect(canAccessRoute(null, '/')).toBe(false);
    expect(canAccessRoute({ ...viewer, enabled: false }, '/')).toBe(false);
  });
});
