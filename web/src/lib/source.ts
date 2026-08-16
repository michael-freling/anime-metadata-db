import { loader } from 'fumadocs-core/source';
import { lucideIconsPlugin } from 'fumadocs-core/source/lucide-icons';
import { docsContentRoute, docsImageRoute, docsRoute } from './shared';
import { defineDocs } from 'fumadocs-mdx/macro';
import { metaSchema, pageSchema } from 'fumadocs-core/source/schema';

// The `development/` tree (proposals, contributing) is internal: it renders on
// the dev server so it can be read and edited locally, but is excluded from any
// other build so it is never published. This replaces the Hugo setup's
// `draft: true` + `cascade` on the Development section node. Gating on the path
// rather than a frontmatter flag preserves the cascade's key property — a new
// page added under development/ is internal automatically, with nothing to
// remember.
//
// The gate is applied here, at the source, rather than at the MDX collection.
// Excluding it from the collection would be stronger — the internal MDX would
// not be compiled into the deployed bundle at all — but `defineDocs` is a macro
// whose `files` option must be an array of string literals, since it decides
// what gets bundled. It therefore cannot vary by environment, and a static
// exclusion would hide the tree from the dev server too, which is the half of
// this behaviour worth keeping.
//
// So the internal MDX *is* compiled into the server bundle; it is simply
// unreachable, because every route resolves pages through the filtered source
// below: the docs route, search, llms.txt/llms-full.txt, the .md content route,
// the OG image route and the sitemap all go through it. The CI job in
// .github/workflows/web.yml asserts that unreachability directly, against a
// running production server, so a regression fails the build rather than
// quietly publishing a design document.
const isDev = process.env.NODE_ENV === 'development';
const INTERNAL_PREFIX = 'development/';

const docs = defineDocs({
  dir: 'content/docs',
  docs: {
    schema: pageSchema,
    postprocess: {
      includeProcessedMarkdown: true,
    },
  },
  meta: {
    schema: metaSchema,
  },
});

const rawSource = docs.toFumadocsSource();

const publishedSource = {
  ...rawSource,
  files: isDev
    ? rawSource.files
    : rawSource.files.filter((file) => !file.path.startsWith(INTERNAL_PREFIX)),
};

// See https://fumadocs.dev/docs/headless/source-api for more info
export const source = loader({
  baseUrl: docsRoute,
  source: publishedSource,
  plugins: [lucideIconsPlugin()],
});

export function getPageImageUrl(page: (typeof source)['$inferPage']) {
  const segments = [...page.slugs, 'image.png'];

  return {
    segments,
    url: '/' + [page.locale, ...docsImageRoute.split('/'), ...segments].filter(Boolean).join('/'),
  };
}

export function getPageMarkdownUrl(page: (typeof source)['$inferPage']) {
  const segments = [...page.slugs, 'content.md'];

  return {
    segments,
    url: '/' + [page.locale, ...docsContentRoute.split('/'), ...segments].filter(Boolean).join('/'),
  };
}

export async function getLLMText(page: (typeof source)['$inferPage']) {
  const processed = await page.data.getText('processed');

  return `# ${page.data.title} (${page.url})

${processed}`;
}
