import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';
import path from 'path';

export default defineConfig({
  plugins: [svelte()],
  test: {
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.{js,ts}'],
    globals: true
  },
  resolve: {
    alias: {
      '$app/paths': path.resolve(import.meta.dirname, 'src/__mocks__/$app/paths.ts'),
      '$app/navigation': path.resolve(import.meta.dirname, 'src/__mocks__/$app/navigation.ts'),
      '$app/state': path.resolve(import.meta.dirname, 'src/__mocks__/$app/state.ts'),
    },
    conditions: ['browser']
  }
});
