import { describe, it, expect } from 'vitest';
import { filterSSLEntries, sortSSLEntries, getUrgencyTier } from './SSLWarnings.svelte';
import type { SSLExpiryEntry } from '$lib/types';

describe('SSLWarnings', () => {
  describe('filterSSLEntries', () => {
    it('includes entries with days_remaining <= 30', () => {
      const entries: SSLExpiryEntry[] = [
        { monitor_id: '1', monitor_name: 'A', days_remaining: 5, expires_at: '2024-02-01T00:00:00Z' },
        { monitor_id: '2', monitor_name: 'B', days_remaining: 30, expires_at: '2024-02-25T00:00:00Z' },
        { monitor_id: '3', monitor_name: 'C', days_remaining: 0, expires_at: '2024-01-15T00:00:00Z' },
        { monitor_id: '4', monitor_name: 'D', days_remaining: -3, expires_at: '2024-01-12T00:00:00Z' }
      ];
      const result = filterSSLEntries(entries);
      expect(result).toHaveLength(4);
    });

    it('excludes entries with days_remaining > 30', () => {
      const entries: SSLExpiryEntry[] = [
        { monitor_id: '1', monitor_name: 'A', days_remaining: 31, expires_at: '2024-03-01T00:00:00Z' },
        { monitor_id: '2', monitor_name: 'B', days_remaining: 100, expires_at: '2024-05-01T00:00:00Z' },
        { monitor_id: '3', monitor_name: 'C', days_remaining: 15, expires_at: '2024-02-10T00:00:00Z' }
      ];
      const result = filterSSLEntries(entries);
      expect(result).toHaveLength(1);
      expect(result[0].monitor_name).toBe('C');
    });

    it('returns empty array when no entries qualify', () => {
      const entries: SSLExpiryEntry[] = [
        { monitor_id: '1', monitor_name: 'A', days_remaining: 45, expires_at: '2024-04-01T00:00:00Z' }
      ];
      expect(filterSSLEntries(entries)).toHaveLength(0);
    });

    it('returns empty array for empty input', () => {
      expect(filterSSLEntries([])).toHaveLength(0);
    });
  });

  describe('sortSSLEntries', () => {
    it('sorts by days_remaining ascending', () => {
      const entries: SSLExpiryEntry[] = [
        { monitor_id: '1', monitor_name: 'B', days_remaining: 20, expires_at: '2024-02-20T00:00:00Z' },
        { monitor_id: '2', monitor_name: 'A', days_remaining: 5, expires_at: '2024-02-05T00:00:00Z' },
        { monitor_id: '3', monitor_name: 'C', days_remaining: -1, expires_at: '2024-01-14T00:00:00Z' }
      ];
      const result = sortSSLEntries(entries);
      expect(result[0].days_remaining).toBe(-1);
      expect(result[1].days_remaining).toBe(5);
      expect(result[2].days_remaining).toBe(20);
    });

    it('uses alphabetical name as tiebreaker when days are equal', () => {
      const entries: SSLExpiryEntry[] = [
        { monitor_id: '1', monitor_name: 'Zebra', days_remaining: 10, expires_at: '2024-02-10T00:00:00Z' },
        { monitor_id: '2', monitor_name: 'Alpha', days_remaining: 10, expires_at: '2024-02-10T00:00:00Z' },
        { monitor_id: '3', monitor_name: 'Middle', days_remaining: 10, expires_at: '2024-02-10T00:00:00Z' }
      ];
      const result = sortSSLEntries(entries);
      expect(result[0].monitor_name).toBe('Alpha');
      expect(result[1].monitor_name).toBe('Middle');
      expect(result[2].monitor_name).toBe('Zebra');
    });

    it('does not mutate original array', () => {
      const entries: SSLExpiryEntry[] = [
        { monitor_id: '1', monitor_name: 'B', days_remaining: 20, expires_at: '2024-02-20T00:00:00Z' },
        { monitor_id: '2', monitor_name: 'A', days_remaining: 5, expires_at: '2024-02-05T00:00:00Z' }
      ];
      const original = [...entries];
      sortSSLEntries(entries);
      expect(entries).toEqual(original);
    });
  });

  describe('getUrgencyTier', () => {
    it('returns expired for days_remaining <= 0', () => {
      expect(getUrgencyTier(0)).toBe('expired');
      expect(getUrgencyTier(-1)).toBe('expired');
      expect(getUrgencyTier(-100)).toBe('expired');
    });

    it('returns critical for days_remaining 1–7', () => {
      expect(getUrgencyTier(1)).toBe('critical');
      expect(getUrgencyTier(4)).toBe('critical');
      expect(getUrgencyTier(7)).toBe('critical');
    });

    it('returns warning for days_remaining 8–30', () => {
      expect(getUrgencyTier(8)).toBe('warning');
      expect(getUrgencyTier(15)).toBe('warning');
      expect(getUrgencyTier(30)).toBe('warning');
    });
  });
});
