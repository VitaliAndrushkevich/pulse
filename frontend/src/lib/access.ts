import type { User } from './types';

/** Shared UI policy. API middleware independently enforces every permission. */
export function isAdminRoute(path: string): boolean {
  return path === '/notifications' || path.startsWith('/notifications/') ||
    /^\/(monitors|heartbeats)\/(create|[^/]+\/edit)\/?$/.test(path);
}

export function canAccessRoute(user: User | null, path: string): boolean {
  if (!user || !user.enabled) return false;
  if (user.must_change_password) return path === '/change-password';
  return user.role === 'admin' || !isAdminRoute(path);
}
