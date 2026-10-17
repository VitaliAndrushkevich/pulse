/**
 * Unit tests: heartbeatStore actions and HeartbeatForm client-side validation.
 *
 * Store tests exercise setHeartbeats / updateHeartbeat / removeHeartbeat /
 * getById / clear and the derived counts. Form tests exercise the client-side
 * validation (name required; interval/grace bounds) and the submit path.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import type { Heartbeat } from '$lib/types';
import { heartbeatStore } from '$lib/stores/heartbeats.svelte';

// --- i18n + api mocks (shared by the form render tests) ---

vi.mock('$lib/i18n', () => ({
  t: (key: string, params?: Record<string, string | number>) => {
    const translations: Record<string, string> = {
      'heartbeats.fields.name': 'Name',
      'heartbeats.fields.expectedInterval': 'Expected interval',
      'heartbeats.fields.grace': 'Grace period',
      'heartbeats.fields.status': 'Status',
      'heartbeats.form.nameRequired': 'Name is required',
      'heartbeats.form.intervalInvalid': 'Expected interval must be a positive number of seconds',
      'heartbeats.form.graceInvalid': 'Grace period must be zero or a positive number of seconds',
      'heartbeats.form.intervalHelp': 'How often this job is expected to check in',
      'heartbeats.form.graceHelp': 'Extra time allowed before a missed check-in alerts',
      'common.save': 'Save',
      'common.cancel': 'Cancel',
      'common.active': 'Active',
      'common.paused': 'Paused',
      'common.creating': 'Creating…',
      'common.saving': 'Saving…',
      'monitors.tags.title': 'Tags',
      'monitors.tags.description': 'Add key-value tags',
      'monitors.tags.addTag': 'Add tag'
    };
    let result = translations[key] ?? key;
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        result = result.replace(`{${k}}`, String(v));
      }
    }
    return result;
  }
}));

vi.mock('$lib/api', () => ({
  getTags: vi.fn().mockResolvedValue([]),
  getTagValues: vi.fn().mockResolvedValue([])
}));

// Import after mocks so the component picks them up.
import HeartbeatForm from '$lib/../components/HeartbeatForm.svelte';

function makeHeartbeat(overrides: Partial<Heartbeat> = {}): Heartbeat {
  return {
    id: overrides.id ?? '11111111-1111-1111-1111-111111111111',
    name: overrides.name ?? 'nightly-backup',
    expected_interval_seconds: overrides.expected_interval_seconds ?? 86400,
    grace_seconds: overrides.grace_seconds ?? 3600,
    status: overrides.status ?? 'active',
    state: overrides.state ?? 'new',
    last_success_at: overrides.last_success_at ?? null,
    next_expected_deadline: overrides.next_expected_deadline ?? null,
    tags: overrides.tags ?? [],
    token_prefix: overrides.token_prefix ?? 'a1B2c3D4',
    token_created_at: overrides.token_created_at ?? '2025-01-01T00:00:00Z',
    history_retention_days: overrides.history_retention_days ?? 30,
    created_at: overrides.created_at ?? '2025-01-01T00:00:00Z',
    updated_at: overrides.updated_at ?? '2025-01-01T00:00:00Z'
  };
}

describe('heartbeatStore', () => {
  beforeEach(() => {
    heartbeatStore.clear();
  });

  it('setHeartbeats replaces the collection and keys by id', () => {
    const a = makeHeartbeat({ id: 'a', name: 'a' });
    const b = makeHeartbeat({ id: 'b', name: 'b' });
    heartbeatStore.setHeartbeats([a, b]);

    expect(heartbeatStore.totalCount).toBe(2);
    expect(heartbeatStore.getById('a')?.name).toBe('a');
    expect(heartbeatStore.getById('b')?.name).toBe('b');
  });

  it('updateHeartbeat adds a new entry and replaces an existing one', () => {
    const a = makeHeartbeat({ id: 'a', name: 'a' });
    heartbeatStore.updateHeartbeat(a);
    expect(heartbeatStore.totalCount).toBe(1);

    heartbeatStore.updateHeartbeat({ ...a, name: 'renamed' });
    expect(heartbeatStore.totalCount).toBe(1);
    expect(heartbeatStore.getById('a')?.name).toBe('renamed');
  });

  it('removeHeartbeat deletes by id and is a no-op for unknown ids', () => {
    const a = makeHeartbeat({ id: 'a' });
    heartbeatStore.setHeartbeats([a]);

    heartbeatStore.removeHeartbeat('missing');
    expect(heartbeatStore.totalCount).toBe(1);

    heartbeatStore.removeHeartbeat('a');
    expect(heartbeatStore.totalCount).toBe(0);
    expect(heartbeatStore.getById('a')).toBeUndefined();
  });

  it('derives up and down counts from state', () => {
    heartbeatStore.setHeartbeats([
      makeHeartbeat({ id: '1', state: 'up' }),
      makeHeartbeat({ id: '2', state: 'up' }),
      makeHeartbeat({ id: '3', state: 'down' }),
      makeHeartbeat({ id: '4', state: 'paused' })
    ]);

    expect(heartbeatStore.upCount).toBe(2);
    expect(heartbeatStore.downCount).toBe(1);
  });

  it('clear empties the store', () => {
    heartbeatStore.setHeartbeats([makeHeartbeat()]);
    heartbeatStore.clear();
    expect(heartbeatStore.totalCount).toBe(0);
  });
});

describe('HeartbeatForm — client-side validation', () => {
  it('blocks submit and shows an error when the name is empty', async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(HeartbeatForm, {
      props: {
        mode: 'create',
        initialValues: { name: '', expected_interval_seconds: 60, grace_seconds: 0 },
        onSubmit,
        onCancel: vi.fn()
      }
    });

    await fireEvent.submit(screen.getByTestId('heartbeat-form'));

    await waitFor(() => {
      expect(screen.getByTestId('hb-name-error')).toBeTruthy();
    });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('blocks submit when the interval is below the minimum', async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(HeartbeatForm, {
      props: {
        mode: 'create',
        initialValues: { name: 'job', expected_interval_seconds: 0, grace_seconds: 0 },
        onSubmit,
        onCancel: vi.fn()
      }
    });

    await fireEvent.submit(screen.getByTestId('heartbeat-form'));

    await waitFor(() => {
      expect(screen.getByTestId('hb-interval-error')).toBeTruthy();
    });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('blocks submit when the grace period is negative', async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(HeartbeatForm, {
      props: {
        mode: 'create',
        initialValues: { name: 'job', expected_interval_seconds: 60, grace_seconds: -5 },
        onSubmit,
        onCancel: vi.fn()
      }
    });

    await fireEvent.submit(screen.getByTestId('heartbeat-form'));

    await waitFor(() => {
      expect(screen.getByTestId('hb-grace-error')).toBeTruthy();
    });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('submits valid values with trimmed name', async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(HeartbeatForm, {
      props: {
        mode: 'create',
        initialValues: { name: '  nightly-backup  ', expected_interval_seconds: 86400, grace_seconds: 3600 },
        onSubmit,
        onCancel: vi.fn()
      }
    });

    await fireEvent.submit(screen.getByTestId('heartbeat-form'));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'nightly-backup',
        expected_interval_seconds: 86400,
        grace_seconds: 3600,
        status: 'active'
      })
    );
  });
});
