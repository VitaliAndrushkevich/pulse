import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import FilterBar from './FilterBar.svelte';
import type { MonitorType, Tag } from '$lib/types';

// Mock the API calls used internally by FilterBar
vi.mock('$lib/api', () => ({
  getTags: vi.fn().mockResolvedValue(['env', 'team', 'region']),
  getTagValues: vi.fn().mockImplementation((key: string) => {
    const values: Record<string, string[]> = {
      env: ['production', 'staging'],
      team: ['platform', 'infra'],
      region: ['us-east-1', 'eu-west-1'],
    };
    return Promise.resolve(values[key] ?? []);
  }),
}));

const availableTypes: MonitorType[] = ['http', 'http3', 'tcp', 'udp', 'websocket'];

function defaultProps(overrides: Partial<{
  availableTypes: MonitorType[];
  activeFilters: { types: MonitorType[]; tags: Tag[] };
  onFilterChange: (filters: { types: MonitorType[]; tags: Tag[] }) => void;
}> = {}) {
  return {
    availableTypes,
    activeFilters: { types: [], tags: [] },
    onFilterChange: vi.fn(),
    ...overrides,
  };
}

describe('FilterBar', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // Requirement 7.1 — Renders type pills for available types
  it('renders type pills for available types when expanded', async () => {
    // Start with an active filter so the bar is expanded
    const props = defaultProps({
      activeFilters: { types: ['http'], tags: [] },
    });
    render(FilterBar, { props });

    await waitFor(() => {
      expect(screen.getByTestId('type-pill-http')).toBeTruthy();
      expect(screen.getByTestId('type-pill-http3')).toBeTruthy();
      expect(screen.getByTestId('type-pill-tcp')).toBeTruthy();
      expect(screen.getByTestId('type-pill-udp')).toBeTruthy();
      expect(screen.getByTestId('type-pill-websocket')).toBeTruthy();
    });
  });

  // Requirement 7.4 — Clicking a type pill toggles it (calls onFilterChange with updated types)
  it('clicking a type pill calls onFilterChange with updated types', async () => {
    const onFilterChange = vi.fn();
    // Start expanded by having an active filter
    const props = defaultProps({
      activeFilters: { types: ['http'], tags: [] },
      onFilterChange,
    });
    render(FilterBar, { props });

    await waitFor(() => {
      expect(screen.getByTestId('type-pill-tcp')).toBeTruthy();
    });

    // Click tcp pill to add it
    screen.getByTestId('type-pill-tcp').click();
    expect(onFilterChange).toHaveBeenCalledWith({
      types: ['http', 'tcp'],
      tags: [],
    });
  });

  // Requirement 7.4 — Deselecting a type pill removes it from the filter
  it('clicking an active type pill deselects it', async () => {
    const onFilterChange = vi.fn();
    const props = defaultProps({
      activeFilters: { types: ['http', 'tcp'], tags: [] },
      onFilterChange,
    });
    render(FilterBar, { props });

    await waitFor(() => {
      expect(screen.getByTestId('type-pill-http')).toBeTruthy();
    });

    // Click http pill to remove it
    screen.getByTestId('type-pill-http').click();
    expect(onFilterChange).toHaveBeenCalledWith({
      types: ['tcp'],
      tags: [],
    });
  });

  // Requirement 7.2 — Renders tag chips for active tags
  it('renders tag chips for active tags', async () => {
    const props = defaultProps({
      activeFilters: {
        types: [],
        tags: [
          { key: 'env', value: 'production' },
          { key: 'team', value: 'platform' },
        ],
      },
    });
    render(FilterBar, { props });

    await waitFor(() => {
      expect(screen.getByTestId('tag-chip-env-production')).toBeTruthy();
      expect(screen.getByTestId('tag-chip-team-platform')).toBeTruthy();
    });
  });

  // Requirement 7.4 — Clicking tag remove button calls onFilterChange without that tag
  it('clicking tag remove button calls onFilterChange without that tag', async () => {
    const onFilterChange = vi.fn();
    const props = defaultProps({
      activeFilters: {
        types: [],
        tags: [
          { key: 'env', value: 'production' },
          { key: 'team', value: 'platform' },
        ],
      },
      onFilterChange,
    });
    render(FilterBar, { props });

    await waitFor(() => {
      expect(screen.getByTestId('tag-remove-env-production')).toBeTruthy();
    });

    screen.getByTestId('tag-remove-env-production').click();
    expect(onFilterChange).toHaveBeenCalledWith({
      types: [],
      tags: [{ key: 'team', value: 'platform' }],
    });
  });

  // Requirement 7.3 — When no filters active and not expanded, shows collapsed "Filter" button
  it('shows collapsed Filter button when no filters active', () => {
    const props = defaultProps({
      activeFilters: { types: [], tags: [] },
    });
    render(FilterBar, { props });

    expect(screen.getByTestId('filter-expand-button')).toBeTruthy();
    expect(screen.queryByTestId('filter-bar')).toBeNull();
  });

  // Requirement 7.3 — Clicking "Filter" button expands the bar
  it('clicking Filter button expands the bar', async () => {
    const props = defaultProps({
      activeFilters: { types: [], tags: [] },
    });
    render(FilterBar, { props });

    const expandBtn = screen.getByTestId('filter-expand-button');
    expandBtn.click();

    await waitFor(() => {
      expect(screen.getByTestId('filter-bar')).toBeTruthy();
    });
  });
});
