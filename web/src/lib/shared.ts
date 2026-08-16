export const appName = 'anime-metadata-db';
export const docsRoute = '/docs';
export const docsImageRoute = '/og/docs';
export const docsContentRoute = '/llms.mdx/docs';

export const gitConfig = {
  user: 'michael-freling',
  repo: 'anime-metadata-db',
  branch: 'main',
};

// --- Deployment configuration ----------------------------------------------
//
// None of this is secret, so it lives here in the repository rather than in the
// Vercel dashboard. A value typed into a hosting UI is invisible to code
// review, absent from git history and impossible to grep for, and it drifts
// between projects without anyone noticing. This app has no credentials at all,
// so there is nothing to define in Vercel.
//
// Where a value genuinely differs by environment, it is derived from what the
// platform already provides (NODE_ENV, VERCEL_ENV, VERCEL_URL) rather than from
// something a human has to remember to set.

// The read-only Connect API's public origin. The API is a separate Vercel
// project built from the Go code at the repository root.
const API_ORIGIN = 'https://anime-metadata-db.vercel.app';

// This site's own canonical origin, used for metadataBase (so Open Graph image
// URLs are absolute and fetchable by crawlers), the sitemap and robots.txt.
const SITE_ORIGIN = 'https://anime-metadata-web.vercel.app';

// The dataset API, called only from server components (see lib/api.ts), so it
// is never exposed to the browser and needs no NEXT_PUBLIC prefix.
//
// `next dev` points at a local `make api` automatically, so developing against
// a local server needs no configuration either.
//
// API_BASE_URL exists for the end-to-end suite, which boots one app instance
// against a local API and a second against a closed port to exercise the
// API-down path. It is a test hook, NOT a deployment setting — nothing should
// ever set it in Vercel.
// Only the variables these functions actually read, so a test can pass a
// partial environment without having to satisfy all of NodeJS.ProcessEnv.
type DeployEnv = Partial<Record<'API_BASE_URL' | 'NODE_ENV' | 'VERCEL_ENV' | 'VERCEL_URL', string>>;

export function resolveApiBaseUrl(env: DeployEnv = process.env): string {
  if (env.API_BASE_URL) return env.API_BASE_URL;
  return env.NODE_ENV === 'development' ? 'http://localhost:8080' : API_ORIGIN;
}

export const apiBaseUrl = resolveApiBaseUrl();

// A preview deployment describes itself with the URL Vercel assigns it, which
// the platform injects automatically. Production and local both use the
// canonical origin, so a preview's metadata never claims to be the live site.
// Taken as a function rather than a constant so callers that run per request
// (robots.txt, the sitemap) read the environment then, instead of whatever was
// present when the module was first loaded during the build. Vercel guarantees
// VERCEL_URL at runtime; at build time it depends on the project exposing
// system environment variables, so a build-time read can silently fall back to
// the production origin and make a preview claim to be the live site.
export function resolveSiteUrl(env: DeployEnv = process.env): string {
  if (env.VERCEL_ENV === 'preview' && env.VERCEL_URL) return `https://${env.VERCEL_URL}`;
  return SITE_ORIGIN;
}

// The build-time value. metadataBase in the root layout has to be a constant,
// since statically generated pages bake their Open Graph URLs in, so that one
// cannot avoid the caveat above.
export const siteUrl = resolveSiteUrl();
