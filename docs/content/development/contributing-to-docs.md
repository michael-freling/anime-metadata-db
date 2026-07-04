---
title: Contributing to the docs
weight: 1
---

# Contributing to the docs

This documentation site is built with [Hugo](https://gohugo.io) and the
[Hextra](https://github.com/imfing/hextra) theme. This page is
internal (part of the local-only **Development** section) and is not published
to the live site.

## Running locally

From the repository root:

```bash
make serve
```

This starts the Hugo dev server with live reload at <http://localhost:1313>,
building drafts — so the Development section (this page, the proposals) is
visible locally but excluded from the published build.

## Adding a public page

Create a Markdown file under `docs/content/docs/`:

```markdown
---
title: My Page
weight: 4
---

# My Page

Your content here.
```

`weight` controls the ordering in the left-hand menu.

## Adding an internal page

Put it anywhere under `docs/content/development/`. That section sets
`draft: true` and cascades it to every child, so `make serve` shows it while the
production build (`make build`) — which is what CI publishes to GitHub Pages —
leaves it out. No per-page front matter is needed.

## Publishing

`make build` renders the production site into `docs/public/`. CI does this and
deploys to GitHub Pages on every push to `main` (see
`.github/workflows/docs.yml`); you don't publish by hand.
