// Naming for the generated API reference pages.
//
// This lives here rather than in scripts/generate-api-docs.mts so it can be
// tested. It decides the URL of every method page, and a mistake in it is not
// visible in the generated output — a wrong slug is still a working page, at
// an address nobody will link to twice.

/** The RPC name out of a fully-qualified operation id or path segment. */
export function operationName(qualified: string): string {
  const base = qualified.split('/').pop()!.replace(/\.mdx$/, '');
  return base.split('.').pop()!;
}

// methodSlug turns a PascalCase RPC name into the kebab-case URL the docs site
// uses everywhere else. The page title stays PascalCase, because that is what
// the method is actually called.
//
// Two passes, not one. A single lower→upper rule reads a run of capitals as one
// word, so `GetAPIKey` would collapse to `getapikey`; the second pass splits an
// acronym from the word that follows it. No RPC in the schema needs this today,
// which is exactly why it is worth handling — the day one does, the failure is
// a quietly wrong URL rather than an error.
//
// A trailing plural on an acronym (`ListIDs`) still splits as `list-i-ds`.
// Telling `IDs` from a word needs a dictionary, and a deterministic ugly answer
// beats a clever inconsistent one; the uniqueness check in the generator is
// what guards the case that actually breaks the site.
export function methodSlug(name: string): string {
  return name
    .replace(/([a-z0-9])([A-Z])/g, '$1-$2')
    .replace(/([A-Z]+)([A-Z][a-z])/g, '$1-$2')
    .toLowerCase();
}

// slugsFor maps every RPC name to its slug, refusing to return a set in which
// two names share one.
//
// Two operations with the same slug would overwrite each other's page: the
// second write wins, one method silently vanishes from the docs, and the
// sidebar lists a page that is really a different method. Nothing downstream
// would notice, so it is caught here.
export function slugsFor(names: readonly string[]): Map<string, string> {
  const byName = new Map<string, string>();
  const bySlug = new Map<string, string>();
  for (const name of names) {
    const slug = methodSlug(name);
    const taken = bySlug.get(slug);
    if (taken !== undefined) {
      throw new Error(
        `${name} and ${taken} both produce the page slug "${slug}"; ` +
          'one would overwrite the other, so rename an RPC or teach methodSlug to tell them apart',
      );
    }
    bySlug.set(slug, name);
    byName.set(name, slug);
  }
  return byName;
}
