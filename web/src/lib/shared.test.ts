import { describe, expect, it } from 'vitest';
import { resolveApiBaseUrl, resolveSiteUrl } from './shared';

// These branches decide which backend the app talks to and which origin it
// claims to be, and both failures are silent: pointing at the wrong API renders
// an error page, and a preview claiming the production origin corrupts its
// sitemap and Open Graph URLs with nothing to notice.

describe('resolveApiBaseUrl', () => {
  it('uses the hosted API in production', () => {
    expect(resolveApiBaseUrl({ NODE_ENV: 'production' })).toBe(
      'https://anime-metadata-db.vercel.app',
    );
  });

  // `next dev` should just work against `make api` with no configuration.
  it('uses a local server in development', () => {
    expect(resolveApiBaseUrl({ NODE_ENV: 'development' })).toBe('http://localhost:8080');
  });

  // The test hook the e2e suite relies on, including its offline instance.
  it('an explicit override wins in every environment', () => {
    expect(resolveApiBaseUrl({ NODE_ENV: 'production', API_BASE_URL: 'http://127.0.0.1:8123' })).toBe(
      'http://127.0.0.1:8123',
    );
    expect(resolveApiBaseUrl({ NODE_ENV: 'development', API_BASE_URL: 'http://127.0.0.1:9' })).toBe(
      'http://127.0.0.1:9',
    );
  });

  it('falls back to the hosted API when nothing is set', () => {
    expect(resolveApiBaseUrl({})).toBe('https://anime-metadata-db.vercel.app');
  });
});

describe('resolveSiteUrl', () => {
  it('uses the canonical origin in production', () => {
    expect(resolveSiteUrl({ VERCEL_ENV: 'production', VERCEL_URL: 'whatever.vercel.app' })).toBe(
      'https://anime-metadata-web.vercel.app',
    );
  });

  it('a preview describes itself, not the live site', () => {
    expect(resolveSiteUrl({ VERCEL_ENV: 'preview', VERCEL_URL: 'branch-abc123.vercel.app' })).toBe(
      'https://branch-abc123.vercel.app',
    );
  });

  // The failure the reviewer flagged: if system environment variables are not
  // exposed, VERCEL_URL is absent and the preview would otherwise be left
  // claiming production. Falling back is the only option, but it must at least
  // be the canonical origin rather than something malformed.
  it('falls back to the canonical origin when VERCEL_URL is missing', () => {
    expect(resolveSiteUrl({ VERCEL_ENV: 'preview' })).toBe('https://anime-metadata-web.vercel.app');
  });

  it('ignores VERCEL_URL outside a preview', () => {
    expect(resolveSiteUrl({ VERCEL_URL: 'branch-abc123.vercel.app' })).toBe(
      'https://anime-metadata-web.vercel.app',
    );
  });

  it('uses the canonical origin locally', () => {
    expect(resolveSiteUrl({})).toBe('https://anime-metadata-web.vercel.app');
  });
});
