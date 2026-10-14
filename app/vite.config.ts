import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

// No dev server is used for this project on developer PCs (see README):
// only `vite build` and `vitest run`.
export default defineConfig({
  plugins: [react()],
  base: '/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    restoreMocks: true,
    include: ['src/**/*.test.{ts,tsx}', 'electron/**/*.test.ts'],
  },
});
