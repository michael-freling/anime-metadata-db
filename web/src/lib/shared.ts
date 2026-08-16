export const appName = 'anime-metadata-db';
export const docsRoute = '/docs';
export const docsImageRoute = '/og/docs';
export const docsContentRoute = '/llms.mdx/docs';

export const gitConfig = {
  user: 'michael-freling',
  repo: 'anime-metadata-db',
  branch: 'main',
};

// Canonical origin of the deployed site. Used for `metadataBase` (so Open Graph
// image URLs are absolute and fetchable by crawlers), the sitemap and
// robots.txt. Overridable so a preview deployment can describe itself.
export const siteUrl = process.env.NEXT_PUBLIC_SITE_URL ?? 'https://anime-metadata-db.vercel.app';

// The hosted read-only Connect API. The web app took the hostname the API used
// to answer on, so the API now has its own. Only server components call it (see
// lib/api.ts), so this is never exposed to the browser and needs no NEXT_PUBLIC
// prefix — point it at http://localhost:8080 to develop against `make api`.
export const apiBaseUrl = process.env.API_BASE_URL ?? 'https://anime-metadata-db-api.vercel.app';
