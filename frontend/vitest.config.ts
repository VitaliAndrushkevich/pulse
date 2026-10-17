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
      $lib: path.resolve(import.meta.dirname, 'src/lib'),
      '$app/navigation': path.resolve(import.meta.dirname, 'src/__mocks__/$app/navigation.ts'),
      '$app/stores': path.resolve(import.meta.dirname, 'src/__mocks__/$app/stores.ts'),
    },
    conditions: ['browser']
  }
});
