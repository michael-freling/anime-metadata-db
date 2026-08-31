import { describe, expect, it } from 'vitest';
import { methodSlug, operationName, slugsFor } from './openapi-slug';

describe('operationName', () => {
  it('takes the RPC name out of a spec path', () => {
    expect(operationName('/anime.v1.AnimeService/ListFranchises')).toBe('ListFranchises');
  });

  it('takes it out of a generated file name too', () => {
    // The generator names files after the operation id; both routes must land
    // on the same name, or the sidebar order and the page filenames disagree.
    expect(operationName('anime.v1.AnimeService.ListFranchises.mdx')).toBe('ListFranchises');
  });

  it('leaves a bare name alone', () => {
    expect(operationName('GetStats')).toBe('GetStats');
  });
});

describe('methodSlug', () => {
  it('kebab-cases the RPCs the schema actually has', () => {
    expect(methodSlug('GetStats')).toBe('get-stats');
    expect(methodSlug('ListFranchises')).toBe('list-franchises');
    expect(methodSlug('Search')).toBe('search');
    expect(methodSlug('ListAppearances')).toBe('list-appearances');
  });

  // The case the single-pass version got wrong: a run of capitals read as one
  // word collapses to an unreadable slug, and nothing downstream notices.
  it('splits an acronym from the word after it', () => {
    expect(methodSlug('GetAPIKey')).toBe('get-api-key');
    expect(methodSlug('ListRPCEndpoints')).toBe('list-rpc-endpoints');
  });

  it('handles a digit boundary', () => {
    expect(methodSlug('GetV2Series')).toBe('get-v2-series');
  });

  // Documented rather than fixed: telling a plural from a word needs a
  // dictionary. It is deterministic, which is what the uniqueness check needs.
  it('is deterministic on an acronym with a trailing plural', () => {
    expect(methodSlug('ListIDs')).toBe('list-i-ds');
    expect(methodSlug('ListIDs')).toBe(methodSlug('ListIDs'));
  });
});

describe('slugsFor', () => {
  it('maps every name to its slug', () => {
    const slugs = slugsFor(['GetStats', 'ListWorks']);
    expect(slugs.get('GetStats')).toBe('get-stats');
    expect(slugs.get('ListWorks')).toBe('list-works');
    expect(slugs.size).toBe(2);
  });

  // The failure that would actually break the site: one page overwrites the
  // other, a method disappears, and the sidebar points at the wrong one.
  it('refuses two names that collide, naming both', () => {
    expect(() => slugsFor(['GetAPIKey', 'GetApiKey'])).toThrowError(
      /GetApiKey and GetAPIKey both produce the page slug "get-api-key"/,
    );
  });

  it('accepts an empty schema', () => {
    expect(slugsFor([]).size).toBe(0);
  });
});
