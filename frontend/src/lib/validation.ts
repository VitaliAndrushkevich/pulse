// Monitor form and login validation functions

import type { MonitorType } from './types';

export interface ValidationResult {
  valid: boolean;
  error: string | null;
}

const VALID_MONITOR_TYPES: MonitorType[] = ['http', 'http3', 'tcp', 'udp', 'websocket', 'grpc', 'dns', 'icmp', 'smtp'];

/** Validates monitor name: non-empty, max 255 characters */
export function validateName(name: string): ValidationResult {
  if (name.trim().length === 0) {
    return { valid: false, error: 'Name is required' };
  }
  if (name.length > 255) {
    return { valid: false, error: 'Name must be at most 255 characters' };
  }
  return { valid: true, error: null };
}

/** Validates monitor type: must be one of the allowed types */
export function validateType(type: string): ValidationResult {
  if (!VALID_MONITOR_TYPES.includes(type as MonitorType)) {
    return { valid: false, error: 'Type must be one of: http, http3, tcp, udp, websocket, grpc, dns, icmp, smtp' };
  }
  return { valid: true, error: null };
}

/** Validates monitor target: non-empty, max 2048 characters */
export function validateTarget(target: string): ValidationResult {
  if (target.trim().length === 0) {
    return { valid: false, error: 'Target is required' };
  }
  if (target.length > 2048) {
    return { valid: false, error: 'Target must be at most 2048 characters' };
  }
  return { valid: true, error: null };
}

/** Validates interval_seconds: between 10 and 86400 */
export function validateInterval(interval: number): ValidationResult {
  if (interval < 10 || interval > 86400) {
    return { valid: false, error: 'Interval must be between 10 and 86400 seconds' };
  }
  return { valid: true, error: null };
}

/** Validates timeout_seconds: between 1 and 300 */
export function validateTimeout(timeout: number): ValidationResult {
  if (timeout < 1 || timeout > 300) {
    return { valid: false, error: 'Timeout must be between 1 and 300 seconds' };
  }
  return { valid: true, error: null };
}

/** Validates email format: must contain @ with local and domain parts */
export function validateEmail(email: string): ValidationResult {
  // Basic RFC 5322 pattern: local@domain with at least one char on each side
  // and domain must have at least one dot with chars around it
  const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  if (!emailRegex.test(email)) {
    return { valid: false, error: 'Please enter a valid email address' };
  }
  return { valid: true, error: null };
}

/** Validates password: must be non-empty */
export function validatePassword(password: string): ValidationResult {
  if (password.length === 0) {
    return { valid: false, error: 'Password is required' };
  }
  return { valid: true, error: null };
}
