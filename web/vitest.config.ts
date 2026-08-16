import { defineConfig } from 'vitest/config';
import { fileURLToPath } from 'node:url';

export default defineConfig({
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  test: {
    include: ['src/**/*.test.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text-summary'],
      // Scoped to the logic worth gating. Server components are mostly JSX
      // composition over the API, and behaviour there is covered by the
      // Playwright suite against a real server — chasing a line-coverage
      // number through them would buy ceremony rather than safety.
      include: ['src/lib/format.ts', 'src/lib/shared.ts'],
      thresholds: { lines: 100, functions: 100, branches: 100, statements: 100 },
    },
  },
});
