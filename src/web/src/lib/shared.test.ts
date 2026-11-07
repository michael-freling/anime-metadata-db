import { describe, expect, it } from 'vitest';
import { resolveApiBaseUrl, resolveApiServer, resolveSiteUrl } from './shared';

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

  // An empty value is not a usable base URL; treating it as one would point
  // every request at the app's own origin.
  it('treats an empty override as unset', () => {
    expect(resolveApiBaseUrl({ API_BASE_URL: '', NODE_ENV: 'production' })).toBe(
      'https://anime-metadata-db.vercel.app',
    );
    expect(resolveApiBaseUrl({ API_BASE_URL: '', NODE_ENV: 'development' })).toBe(
      'http://localhost:8080',
    );
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

// The label is what the API reference prints beside the playground's target.
// It comes from the same branch as the URL precisely so it cannot describe a
// different one — a contributor running against `make api` must not be told
// they are calling the deployed service.
describe('resolveApiServer', () => {
  it('labels the deployed origin in production', () => {
    const server = resolveApiServer({ NODE_ENV: 'production' });
    expect(server.url).toBe('https://anime-metadata-db.vercel.app');
    expect(server.label).toBe('Deployed API');
  });

  it('labels localhost as local, and says how to start it', () => {
    const server = resolveApiServer({ NODE_ENV: 'development' });
    expect(server.url).toBe('http://localhost:8080');
    expect(server.label).toBe('Local API (cd api && go run ./cmd/api)');
  });

  it('names the override rather than guessing what it points at', () => {
    const server = resolveApiServer({ API_BASE_URL: 'http://127.0.0.1:8123' });
    expect(server.url).toBe('http://127.0.0.1:8123');
    expect(server.label).toBe('Set by API_BASE_URL');
  });

  // The property that matters: whatever the environment, the label is decided
  // with the URL, so the two describe the same thing.
  it('agrees with resolveApiBaseUrl in every environment', () => {
    for (const env of [
      { NODE_ENV: 'production' },
      { NODE_ENV: 'development' },
      { API_BASE_URL: 'http://example.test' },
      { API_BASE_URL: '', NODE_ENV: 'development' },
      {},
    ]) {
      expect(resolveApiServer(env).url).toBe(resolveApiBaseUrl(env));
    }
  });
});
