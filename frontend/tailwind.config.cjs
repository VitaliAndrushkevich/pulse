/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./src/**/*.{html,js,svelte,ts}'],
  darkMode: ['selector', '[data-theme="dark"]'],
  theme: {
    extend: {
      colors: {
        brand: {
          50: 'var(--color-brand-50, #f0f9ff)',
          100: 'var(--color-brand-100, #e0f2fe)',
          200: 'var(--color-brand-200, #bae6fd)',
          300: 'var(--color-brand-300, #7dd3fc)',
          400: 'var(--color-brand-400, #38bdf8)',
          500: 'var(--color-brand-500, #0ea5e9)',
          600: 'var(--color-brand-600, #0284c7)',
          700: 'var(--color-brand-700, #0369a1)',
          800: 'var(--color-brand-800, #075985)',
          900: 'var(--color-brand-900, #0c4a6e)',
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
