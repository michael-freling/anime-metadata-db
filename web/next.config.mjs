import { fileURLToPath } from 'node:url';
import { createMDX } from 'fumadocs-mdx/next';

const withMDX = createMDX();

/** @type {import('next').NextConfig} */
const config = {
  reactStrictMode: true,
  turbopack: {
    // The repository root carries its own package-lock.json for the Vercel CLI
    // dev dependency, so Turbopack's workspace-root inference picks /work and
    // warns. This app is self-contained; pin the root to web/.
    root: fileURLToPath(new URL('.', import.meta.url)),
  },
};

export default withMDX(config);
