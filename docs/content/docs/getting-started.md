---
title: Getting Started
weight: 1
---

# Getting Started

## Prerequisites

- [Hugo extended](https://gohugo.io/installation/) (the theme requires the
  extended edition for SCSS support)
- [Go](https://go.dev/dl/) (used to fetch the theme via Hugo Modules)

## Running locally

From the repository root:

```bash
make serve
```

This starts the Hugo development server with live reload at
<http://localhost:1313>.

## Adding content

Create a new Markdown file under `docs/content/docs/`:

```markdown
---
title: My Page
weight: 2
---

# My Page

Your content here.
```

The `weight` front matter controls the ordering in the left-hand menu.
