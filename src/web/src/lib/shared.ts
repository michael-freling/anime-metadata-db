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

// The API the app talks to, and what to call it.
//
// Both come out of one decision on purpose. The API reference labels its
// playground server with this, and a label chosen by a second function reading
// the same environment is free to disagree with the URL — telling a contributor
// running against `make api` that they are calling the deployed service. There
// is only one branch here, so there is nothing to keep in step.
export interface ApiServer {
  url: string;
  /** Human label for the URL, shown beside it in the API reference. */
  label: string;
}

export function resolveApiServer(env: DeployEnv = process.env): ApiServer {
  // Deliberately a truthy check, not `??`: an empty API_BASE_URL — from a shell
  // exporting an unset variable, or an env-file substitution that resolved to
  // nothing — is not a usable base URL, and treating it as one would point
  // every request at the app's own origin. Falling back to the default is the
  // only sensible reading, so it is pinned by a test.
  if (env.API_BASE_URL) {
    // Named rather than described, because this branch is a test hook and the
    // URL could be anything; claiming it is "local" or "deployed" would be a
    // guess. Saying where it came from is the one thing that is certainly true.
    return { url: env.API_BASE_URL, label: 'Set by API_BASE_URL' };
  }
  if (env.NODE_ENV === 'development') {
    return { url: 'http://localhost:8080', label: 'Local API (cd api && go run ./cmd/api)' };
  }
  return { url: API_ORIGIN, label: 'Deployed API' };
}

export function resolveApiBaseUrl(env: DeployEnv = process.env): string {
  return resolveApiServer(env).url;
}

export const apiServer = resolveApiServer();
export const apiBaseUrl = apiServer.url;

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
