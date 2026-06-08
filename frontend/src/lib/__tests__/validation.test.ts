import { describe, it, expect } from 'vitest';
import {
  validateName,
  validateType,
  validateTarget,
  validateInterval,
  validateTimeout,
  validateEmail,
  validatePassword
} from '../validation';

describe('validateName', () => {
  it('rejects empty string', () => {
    expect(validateName('')).toEqual({ valid: false, error: 'Name is required' });
  });

  it('rejects whitespace-only string', () => {
    expect(validateName('   ')).toEqual({ valid: false, error: 'Name is required' });
  });

  it('rejects string longer than 255 characters', () => {
    const longName = 'a'.repeat(256);
    expect(validateName(longName)).toEqual({ valid: false, error: 'Name must be at most 255 characters' });
  });

  it('accepts valid name', () => {
    expect(validateName('My Monitor')).toEqual({ valid: true, error: null });
  });

  it('accepts name at exactly 255 characters', () => {
    expect(validateName('a'.repeat(255))).toEqual({ valid: true, error: null });
  });
});

describe('validateType', () => {
  it('accepts all valid types', () => {
    for (const type of ['http', 'http3', 'tcp', 'udp', 'websocket', 'grpc']) {
      expect(validateType(type)).toEqual({ valid: true, error: null });
    }
  });

  it('rejects invalid type', () => {
    expect(validateType('ftp')).toEqual({ valid: false, error: 'Type must be one of: http, http3, tcp, udp, websocket, grpc' });
  });

  it('rejects empty string', () => {
    expect(validateType('')).toEqual({ valid: false, error: 'Type must be one of: http, http3, tcp, udp, websocket, grpc' });
  });
});

describe('validateTarget', () => {
  it('rejects empty string', () => {
    expect(validateTarget('')).toEqual({ valid: false, error: 'Target is required' });
  });

  it('rejects whitespace-only string', () => {
    expect(validateTarget('  ')).toEqual({ valid: false, error: 'Target is required' });
  });

  it('rejects string longer than 2048 characters', () => {
    const longTarget = 'a'.repeat(2049);
    expect(validateTarget(longTarget)).toEqual({ valid: false, error: 'Target must be at most 2048 characters' });
  });

  it('accepts valid target', () => {
    expect(validateTarget('https://example.com')).toEqual({ valid: true, error: null });
  });

  it('accepts target at exactly 2048 characters', () => {
    expect(validateTarget('a'.repeat(2048))).toEqual({ valid: true, error: null });
  });
});

describe('validateInterval', () => {
  it('rejects value below 10', () => {
    expect(validateInterval(9)).toEqual({ valid: false, error: 'Interval must be between 10 and 86400 seconds' });
  });

  it('rejects value above 86400', () => {
    expect(validateInterval(86401)).toEqual({ valid: false, error: 'Interval must be between 10 and 86400 seconds' });
  });

  it('accepts value at lower bound (10)', () => {
    expect(validateInterval(10)).toEqual({ valid: true, error: null });
  });

  it('accepts value at upper bound (86400)', () => {
    expect(validateInterval(86400)).toEqual({ valid: true, error: null });
  });

  it('accepts value in range', () => {
    expect(validateInterval(60)).toEqual({ valid: true, error: null });
  });
});

describe('validateTimeout', () => {
  it('rejects value below 1', () => {
    expect(validateTimeout(0)).toEqual({ valid: false, error: 'Timeout must be between 1 and 300 seconds' });
  });

  it('rejects value above 300', () => {
    expect(validateTimeout(301)).toEqual({ valid: false, error: 'Timeout must be between 1 and 300 seconds' });
  });

  it('accepts value at lower bound (1)', () => {
    expect(validateTimeout(1)).toEqual({ valid: true, error: null });
  });

  it('accepts value at upper bound (300)', () => {
    expect(validateTimeout(300)).toEqual({ valid: true, error: null });
  });

  it('accepts value in range', () => {
    expect(validateTimeout(30)).toEqual({ valid: true, error: null });
  });
});

describe('validateEmail', () => {
  it('rejects empty string', () => {
    expect(validateEmail('')).toEqual({ valid: false, error: 'Please enter a valid email address' });
  });

  it('rejects string without @', () => {
    expect(validateEmail('userexample.com')).toEqual({ valid: false, error: 'Please enter a valid email address' });
  });

  it('rejects string without domain part', () => {
    expect(validateEmail('user@')).toEqual({ valid: false, error: 'Please enter a valid email address' });
  });

  it('rejects string without local part', () => {
    expect(validateEmail('@example.com')).toEqual({ valid: false, error: 'Please enter a valid email address' });
  });

  it('rejects string without dot in domain', () => {
    expect(validateEmail('user@example')).toEqual({ valid: false, error: 'Please enter a valid email address' });
  });

  it('rejects email with spaces', () => {
    expect(validateEmail('user @example.com')).toEqual({ valid: false, error: 'Please enter a valid email address' });
  });

  it('accepts valid email', () => {
    expect(validateEmail('user@example.com')).toEqual({ valid: true, error: null });
  });

  it('accepts email with subdomain', () => {
    expect(validateEmail('user@mail.example.com')).toEqual({ valid: true, error: null });
  });

  it('accepts email with plus addressing', () => {
    expect(validateEmail('user+tag@example.com')).toEqual({ valid: true, error: null });
  });
});

describe('validatePassword', () => {
  it('rejects empty string', () => {
    expect(validatePassword('')).toEqual({ valid: false, error: 'Password is required' });
  });

  it('accepts any non-empty password', () => {
    expect(validatePassword('a')).toEqual({ valid: true, error: null });
  });

  it('accepts long password', () => {
    expect(validatePassword('supersecretpassword123!')).toEqual({ valid: true, error: null });
  });
});
