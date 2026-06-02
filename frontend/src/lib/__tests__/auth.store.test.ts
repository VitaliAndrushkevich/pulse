import { describe, it, expect, beforeEach } from 'vitest';
import { getToken, setToken, clearToken, isAuthenticated } from '../stores/auth.svelte';

describe('AuthStore', () => {
  beforeEach(() => {
    // Clear state between tests
    clearToken();
    localStorage.clear();
  });

  describe('getToken()', () => {
    it('returns null when no token is stored', () => {
      expect(getToken()).toBeNull();
    });

    it('returns the token after setToken is called', () => {
      setToken('test-jwt-token');
      expect(getToken()).toBe('test-jwt-token');
    });
  });

  describe('setToken()', () => {
    it('stores the token in localStorage', () => {
      setToken('my-jwt');
      expect(localStorage.getItem('pulse_jwt')).toBe('my-jwt');
    });

    it('overwrites a previously stored token', () => {
      setToken('first-token');
      setToken('second-token');
      expect(getToken()).toBe('second-token');
      expect(localStorage.getItem('pulse_jwt')).toBe('second-token');
    });
  });

  describe('clearToken()', () => {
    it('removes the token from state', () => {
      setToken('token-to-clear');
      clearToken();
      expect(getToken()).toBeNull();
    });

    it('removes the token from localStorage', () => {
      setToken('token-to-clear');
      clearToken();
      expect(localStorage.getItem('pulse_jwt')).toBeNull();
    });
  });

  describe('isAuthenticated()', () => {
    it('returns false when no token is stored', () => {
      expect(isAuthenticated()).toBe(false);
    });

    it('returns true after setToken is called', () => {
      setToken('auth-token');
      expect(isAuthenticated()).toBe(true);
    });

    it('returns false after clearToken is called', () => {
      setToken('auth-token');
      clearToken();
      expect(isAuthenticated()).toBe(false);
    });
  });

  describe('localStorage persistence', () => {
    it('persists token to localStorage on setToken', () => {
      setToken('new-token');
      expect(localStorage.getItem('pulse_jwt')).toBe('new-token');
    });

    it('removes token from localStorage on clearToken', () => {
      setToken('token');
      clearToken();
      expect(localStorage.getItem('pulse_jwt')).toBeNull();
    });
  });
});
