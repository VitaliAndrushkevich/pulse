import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import WidgetShell from './WidgetShell.svelte';

vi.mock('$lib/i18n', () => ({
  t: (key: string) => {
    const translations: Record<string, string> = {
      'common.loading': 'Loading...',
      'common.retry': 'Retry',
    };
    return translations[key] ?? key;
  },
}));

describe('WidgetShell', () => {
  it('renders loading skeleton when loading is true', () => {
    const children = createRawSnippet(() => ({
      render: () => '<p>Content</p>',
    }));
    render(WidgetShell, {
      props: { loading: true, error: null, onRetry: null, children },
    });
    expect(screen.getByTestId('widget-skeleton')).toBeTruthy();
    expect(screen.queryByTestId('widget-error')).toBeNull();
    expect(screen.queryByText('Content')).toBeNull();
  });

  it('skeleton has aria-busy and aria-label for accessibility', () => {
    const children = createRawSnippet(() => ({
      render: () => '<p>Content</p>',
    }));
    render(WidgetShell, {
      props: { loading: true, error: null, onRetry: null, children },
    });
    const skeleton = screen.getByTestId('widget-skeleton');
    expect(skeleton.getAttribute('aria-busy')).toBe('true');
    expect(skeleton.getAttribute('aria-label')).toBe('Loading...');
  });

  it('skeleton contains animate-pulse elements', () => {
    const children = createRawSnippet(() => ({
      render: () => '<p>Content</p>',
    }));
    render(WidgetShell, {
      props: { loading: true, error: null, onRetry: null, children },
    });
    const skeleton = screen.getByTestId('widget-skeleton');
    const pulseElements = skeleton.querySelectorAll('.animate-pulse');
    expect(pulseElements.length).toBe(3);
  });

  it('renders error message when error is provided', () => {
    const children = createRawSnippet(() => ({
      render: () => '<p>Content</p>',
    }));
    render(WidgetShell, {
      props: { loading: false, error: 'Network failed', onRetry: vi.fn(), children },
    });
    expect(screen.getByTestId('widget-error')).toBeTruthy();
    expect(screen.getByText('Network failed')).toBeTruthy();
    expect(screen.queryByTestId('widget-skeleton')).toBeNull();
    expect(screen.queryByText('Content')).toBeNull();
  });

  it('renders retry button when onRetry is provided', () => {
    const onRetry = vi.fn();
    const children = createRawSnippet(() => ({
      render: () => '<p>Content</p>',
    }));
    render(WidgetShell, {
      props: { loading: false, error: 'Something broke', onRetry, children },
    });
    const retryBtn = screen.getByTestId('widget-retry');
    expect(retryBtn).toBeTruthy();
    expect(retryBtn.textContent).toBe('Retry');
  });

  it('calls onRetry when retry button is clicked', () => {
    const onRetry = vi.fn();
    const children = createRawSnippet(() => ({
      render: () => '<p>Content</p>',
    }));
    render(WidgetShell, {
      props: { loading: false, error: 'Something broke', onRetry, children },
    });
    screen.getByTestId('widget-retry').click();
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it('does not render retry button when onRetry is null', () => {
    const children = createRawSnippet(() => ({
      render: () => '<p>Content</p>',
    }));
    render(WidgetShell, {
      props: { loading: false, error: 'Something broke', onRetry: null, children },
    });
    expect(screen.queryByTestId('widget-retry')).toBeNull();
  });

  it('renders slot content when not loading and no error', () => {
    const children = createRawSnippet(() => ({
      render: () => '<p>Widget content</p>',
    }));
    render(WidgetShell, {
      props: { loading: false, error: null, onRetry: null, children },
    });
    expect(screen.getByText('Widget content')).toBeTruthy();
    expect(screen.queryByTestId('widget-skeleton')).toBeNull();
    expect(screen.queryByTestId('widget-error')).toBeNull();
  });

  it('loading takes priority over error (loading > error > content)', () => {
    const children = createRawSnippet(() => ({
      render: () => '<p>Content</p>',
    }));
    render(WidgetShell, {
      props: { loading: true, error: 'Some error', onRetry: vi.fn(), children },
    });
    expect(screen.getByTestId('widget-skeleton')).toBeTruthy();
    expect(screen.queryByTestId('widget-error')).toBeNull();
    expect(screen.queryByText('Content')).toBeNull();
  });

  it('error state has role=alert for accessibility', () => {
    const children = createRawSnippet(() => ({
      render: () => '<p>Content</p>',
    }));
    render(WidgetShell, {
      props: { loading: false, error: 'Failed', onRetry: null, children },
    });
    const errorEl = screen.getByTestId('widget-error');
    expect(errorEl.getAttribute('role')).toBe('alert');
  });
});
