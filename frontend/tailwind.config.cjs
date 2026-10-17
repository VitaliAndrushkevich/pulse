/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  darkMode: ['selector', '[data-theme="dark"]'],
  theme: {
    extend: {
      borderRadius: {
        md: '0.25rem',
        lg: '0.375rem',
        xl: '0.375rem',
        '2xl': '0.5rem',
      },
      boxShadow: {
        sm: '0 1px 0 color-mix(in srgb, var(--color-border) 35%, transparent)',
      },
      colors: {
        // Existing indigo action utilities share Pulse's semantic brand scale.
        // Keep older form components consistent while they migrate to brand-*.
        indigo: Object.fromEntries([50, 100, 200, 300, 400, 500, 600, 700, 800, 900].map(
          (shade) => [shade, `var(--color-brand-${shade})`]
        )),
        brand: {
          50: 'var(--color-brand-50, #f0f9ff)',
          100: 'var(--color-brand-100, #e0f2fe)',
          200: 'var(--color-brand-200, #bae6fd)',
          300: 'var(--color-brand-300, #7dd3fc)',
          400: 'var(--color-brand-400, #38bdf8)',
          500: 'var(--color-brand-500, #0284c7)',
          600: 'var(--color-brand-600, #0369a1)',
          700: 'var(--color-brand-700, #075985)',
          800: 'var(--color-brand-800, #0c4a6e)',
          900: 'var(--color-brand-900, #083b59)',
        },
        surface: 'var(--color-bg-surface)',
        page: 'var(--color-bg-page)',
        primary: 'var(--color-text-primary)',
        secondary: 'var(--color-text-secondary)',
        border: 'var(--color-border)',
        success: 'var(--color-success, #10b981)',
        warning: 'var(--color-warning, #f59e0b)',
        error: 'var(--color-error, #ef4444)',
      },
      borderColor: {
        DEFAULT: 'var(--color-border)',
      },
      textColor: {
        brand: { 500: 'var(--color-brand-primary)', 600: 'var(--color-brand-primary)', 700: 'var(--color-brand-hover)' },
        indigo: { 500: 'var(--color-brand-primary)', 600: 'var(--color-brand-primary)', 700: 'var(--color-brand-hover)' },
        primary: 'var(--color-text-primary)',
        secondary: 'var(--color-text-secondary)',
      },
      backgroundColor: {
        surface: 'var(--color-bg-surface)',
        page: 'var(--color-bg-page)',
      },
      fontFamily: {
        brand: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
      },
    },
  },
  plugins: [],
};
