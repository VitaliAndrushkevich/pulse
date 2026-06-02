import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';
import path from 'path';

export default defineConfig({
  plugins: [svelte({ hot: false })],
  test: {
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.{js,ts}'],
    globals: true
  },
  resolve: {
    alias: {
      $lib: path.resolve(__dirname, 'src/lib'),
      '$app/navigation': path.resolve(__dirname, 'src/__mocks__/$app/navigation.ts'),
    },
    conditions: ['browser']
  }
});
