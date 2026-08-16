import { defineConfig, globalIgnores } from 'eslint/config';
import nextVitals from 'eslint-config-next/core-web-vitals';

const eslintConfig = defineConfig([
  ...nextVitals,
  globalIgnores([
    '.next/**',
    'out/**',
    'build/**',
    'next-env.d.ts',
    '.source/**',
    // Generated from proto/anime/v1/anime.proto by `make generate`; not ours to
    // lint or fix.
    'src/lib/gen/**',
  ]),
]);

export default eslintConfig;