# web

The documentation site for `anime-metadata-db`, and — in time — the UI for
browsing the dataset itself. A [Next.js](https://nextjs.org) App Router project
using [Fumadocs](https://fumadocs.dev), deployed on Vercel as its own project
rooted at this directory.

The read-only Connect API is a **separate** Vercel project built from the Go
code at the repository root; this app is unrelated to that build. Browse pages
call the API from server components, so the browser never makes a cross-origin
request and the API needs no CORS configuration.

## Running it

```sh
npm install
npm run dev     # http://localhost:3000
npm run build   # production build, same as CI and Vercel run
npm run lint
npm run types:check
```

## Layout

| Path | Holds |
|---|---|
| `content/docs/` | The published guides, as MDX. |
| `content/docs/development/` | **Internal** design docs — see below. |
| `content/docs/**/meta.json` | Sidebar ordering (this replaced Hugo's `weight`). |
| `src/lib/source.ts` | The content source, and the internal-docs gate. |
| `src/lib/shared.ts` | Site name, canonical URL, GitHub coordinates. |
| `src/app/(home)/` | The landing page. |
| `src/app/docs/` | The docs routes. |
| `src/proxy.ts` | Serves Markdown for `.md` URLs and `Accept: text/markdown`. Must live at `src/`, not the project root, or Next.js never loads it. |

## Internal docs

Everything under `content/docs/development/` is internal: it renders on the dev
server so it can be read and edited locally, and is excluded from every other
build so it is never published. The rule is the path, not a per-page flag, so a
new page added there is internal automatically.

`.github/workflows/web.yml` asserts this against a running production build —
internal routes must 404 across every representation the site serves (HTML, the
`.md` route, the LLM text routes, OG images, search and the sitemap) while the
public pages still return 200. If you change `src/lib/source.ts`, that job is
what tells you whether you broke it.

## Adding a page

See [Contributing to the docs](content/docs/development/contributing-to-docs.mdx).
