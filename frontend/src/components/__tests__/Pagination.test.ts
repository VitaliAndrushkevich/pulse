import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Pagination from '../Pagination.svelte';

// Mock i18n to avoid $effect outside component context
vi.mock('$lib/i18n', () => ({
  t: (key: string, params?: Record<string, string | number>) => {
    const translations: Record<string, string> = {
      'common.previous': 'Previous',
      'common.next': 'Next',
      'common.pageOf': 'Page {page} of {totalPages}',
    };
    let result = translations[key] ?? key;
    if (params) {
      for (const [k, v] of Object.entries(params)) {
        result = result.replace(`{${k}}`, String(v));
      }
    }
    return result;
  },
}));

describe('Pagination', () => {
  it('renders current page and total pages info', () => {
    render(Pagination, { props: { page: 2, totalPages: 5, onPageChange: vi.fn() } });
    expect(screen.getByText('Page 2 of 5')).toBeTruthy();
  });

  it('renders Previous and Next buttons', () => {
    render(Pagination, { props: { page: 1, totalPages: 3, onPageChange: vi.fn() } });
    expect(screen.getByRole('button', { name: 'Previous' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Next' })).toBeTruthy();
  });

  it('disables Previous button on page 1', () => {
    render(Pagination, { props: { page: 1, totalPages: 5, onPageChange: vi.fn() } });
    const prevBtn = screen.getByRole('button', { name: 'Previous' });
    expect(prevBtn).toHaveProperty('disabled', true);
  });

  it('enables Previous button on page > 1', () => {
    render(Pagination, { props: { page: 3, totalPages: 5, onPageChange: vi.fn() } });
    const prevBtn = screen.getByRole('button', { name: 'Previous' });
    expect(prevBtn).toHaveProperty('disabled', false);
  });

  it('disables Next button on last page', () => {
    render(Pagination, { props: { page: 5, totalPages: 5, onPageChange: vi.fn() } });
    const nextBtn = screen.getByRole('button', { name: 'Next' });
    expect(nextBtn).toHaveProperty('disabled', true);
  });

  it('enables Next button when not on last page', () => {
    render(Pagination, { props: { page: 2, totalPages: 5, onPageChange: vi.fn() } });
    const nextBtn = screen.getByRole('button', { name: 'Next' });
    expect(nextBtn).toHaveProperty('disabled', false);
  });

  it('disables both buttons when totalPages is 1', () => {
    render(Pagination, { props: { page: 1, totalPages: 1, onPageChange: vi.fn() } });
    const prevBtn = screen.getByRole('button', { name: 'Previous' });
    const nextBtn = screen.getByRole('button', { name: 'Next' });
    expect(prevBtn).toHaveProperty('disabled', true);
    expect(nextBtn).toHaveProperty('disabled', true);
  });

  it('calls onPageChange with page - 1 when Previous is clicked', async () => {
    const onPageChange = vi.fn();
    render(Pagination, { props: { page: 3, totalPages: 5, onPageChange } });
    const prevBtn = screen.getByRole('button', { name: 'Previous' });
    prevBtn.click();
    expect(onPageChange).toHaveBeenCalledWith(2);
  });

  it('calls onPageChange with page + 1 when Next is clicked', async () => {
    const onPageChange = vi.fn();
    render(Pagination, { props: { page: 2, totalPages: 5, onPageChange } });
    const nextBtn = screen.getByRole('button', { name: 'Next' });
    nextBtn.click();
    expect(onPageChange).toHaveBeenCalledWith(3);
  });

  it('does not call onPageChange when Previous is clicked on page 1', async () => {
    const onPageChange = vi.fn();
    render(Pagination, { props: { page: 1, totalPages: 5, onPageChange } });
    const prevBtn = screen.getByRole('button', { name: 'Previous' });
    prevBtn.click();
    expect(onPageChange).not.toHaveBeenCalled();
  });

  it('does not call onPageChange when Next is clicked on last page', async () => {
    const onPageChange = vi.fn();
    render(Pagination, { props: { page: 5, totalPages: 5, onPageChange } });
    const nextBtn = screen.getByRole('button', { name: 'Next' });
    nextBtn.click();
    expect(onPageChange).not.toHaveBeenCalled();
  });
});
