import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import BrandLockup from '../BrandLockup.svelte';

describe('BrandLockup', () => {
  describe('variant="full" (default)', () => {
    it('renders SVG and wordmark span', () => {
      const { container } = render(BrandLockup, { props: { variant: 'full' } });

      const svg = container.querySelector('svg');
      expect(svg).not.toBeNull();

      const wordmark = container.querySelector('.brand-wordmark');
      expect(wordmark).not.toBeNull();
      expect(wordmark!.textContent?.trim()).toBe('Pulse');
    });

    it('renders with default variant="full" when no variant prop provided', () => {
      const { container } = render(BrandLockup);

      const svg = container.querySelector('svg');
      expect(svg).not.toBeNull();

      const wordmark = container.querySelector('.brand-wordmark');
      expect(wordmark).not.toBeNull();
    });
  });

  describe('variant="compact"', () => {
    it('renders SVG only, no wordmark', () => {
      const { container } = render(BrandLockup, { props: { variant: 'compact' } });

      const svg = container.querySelector('svg');
      expect(svg).not.toBeNull();

      const wordmark = container.querySelector('.brand-wordmark');
      expect(wordmark).toBeNull();
    });
  });

  describe('default size=32', () => {
    it('produces correct SVG dimensions', () => {
      const { container } = render(BrandLockup, { props: { size: 32, variant: 'full' } });

      const svg = container.querySelector('svg');
      expect(svg).not.toBeNull();
      expect(svg!.getAttribute('width')).toBe('32');
      expect(svg!.getAttribute('height')).toBe('32');
    });

    it('produces correct gap (32/4 = 8px)', () => {
      const { container } = render(BrandLockup, { props: { size: 32, variant: 'full' } });

      const lockup = container.querySelector('.brand-lockup') as HTMLElement;
      expect(lockup.style.gap).toBe('8px');
    });

    it('produces correct wordmark font-size (32*0.625 = 20px)', () => {
      const { container } = render(BrandLockup, { props: { size: 32, variant: 'full' } });

      const wordmark = container.querySelector('.brand-wordmark') as HTMLElement;
      expect(wordmark.style.fontSize).toBe('20px');
    });
  });

  describe('custom size prop', () => {
    it('calculates correct gap for size=64 (64/4 = 16px)', () => {
      const { container } = render(BrandLockup, { props: { size: 64, variant: 'full' } });

      const lockup = container.querySelector('.brand-lockup') as HTMLElement;
      expect(lockup.style.gap).toBe('16px');
    });

    it('calculates correct font-size for size=48 (48*0.625 = 30px)', () => {
      const { container } = render(BrandLockup, { props: { size: 48, variant: 'full' } });

      const wordmark = container.querySelector('.brand-wordmark') as HTMLElement;
      expect(wordmark.style.fontSize).toBe('30px');
    });

    it('sets SVG width and height to match size prop', () => {
      const { container } = render(BrandLockup, { props: { size: 48, variant: 'full' } });

      const svg = container.querySelector('svg');
      expect(svg!.getAttribute('width')).toBe('48');
      expect(svg!.getAttribute('height')).toBe('48');
    });
  });

  describe('SVG attributes', () => {
    it('has correct viewBox attribute', () => {
      const { container } = render(BrandLockup);

      const svg = container.querySelector('svg');
      expect(svg!.getAttribute('viewBox')).toBe('0 0 32 32');
    });

    it('has fill="none"', () => {
      const { container } = render(BrandLockup);

      const svg = container.querySelector('svg');
      expect(svg!.getAttribute('fill')).toBe('none');
    });

    it('has stroke="currentColor"', () => {
      const { container } = render(BrandLockup);

      const svg = container.querySelector('svg');
      expect(svg!.getAttribute('stroke')).toBe('currentColor');
    });

    it('has stroke-width="3.2"', () => {
      const { container } = render(BrandLockup);

      const svg = container.querySelector('svg');
      expect(svg!.getAttribute('stroke-width')).toBe('3.2');
    });

    it('has stroke-linecap="round"', () => {
      const { container } = render(BrandLockup);

      const svg = container.querySelector('svg');
      expect(svg!.getAttribute('stroke-linecap')).toBe('round');
    });

    it('has stroke-linejoin="round"', () => {
      const { container } = render(BrandLockup);

      const svg = container.querySelector('svg');
      expect(svg!.getAttribute('stroke-linejoin')).toBe('round');
    });

    it('has aria-hidden="true" for decorative purposes', () => {
      const { container } = render(BrandLockup);

      const svg = container.querySelector('svg');
      expect(svg!.getAttribute('aria-hidden')).toBe('true');
    });
  });

  describe('accessibility', () => {
    it('container has brand-lockup class for styling hooks', () => {
      const { container } = render(BrandLockup);

      const lockup = container.querySelector('.brand-lockup');
      expect(lockup).not.toBeNull();
    });
  });
});
