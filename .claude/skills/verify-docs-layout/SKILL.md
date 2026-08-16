---
name: verify-docs-layout
description: Build and serve the web/ docs site, capture screenshots of the landing and guide pages (desktop light, dark, mobile) with headless Chromium, then visually verify the layouts actually look good. Use when changing the docs site theme, landing page, navigation, or any page layout — or whenever asked to check how the docs render.
---

# Verify docs layout

The docs site lives in `web/` (Next.js + Fumadocs). A clean `next build`
does **not** prove the pages look right — real problems (duplicate titles,
invisible buttons, code fences leaking as literal backticks, overflow, broken
dark mode) only show up in the rendered pixels. This skill renders the site and
lets you look at it.

## How to run

From the repository root:

```bash
bash .claude/skills/verify-docs-layout/scripts/capture.sh
```

What it does:
1. Ensures a headless Chromium is available via Playwright, staging any missing
   system libraries into `web/.screenshots/` (no root required). Idempotent.
2. Runs `npm run build` — the **production** build, from which the internal
   Development section is excluded, matching what real users see.
3. Serves it with `next start` and screenshots the key pages.
4. Writes PNGs to `web/.screenshots/out/` and prints their paths.

Options (environment variables):
- `INTERNAL=1` — capture the Development section too. That tree only exists on
  the dev server, so this runs `next dev` instead of the production build.
- `PORT=8199` — change the local serve port.

To screenshot a page not in the default set, pass paths as extra args, e.g.
`... /scripts/capture.sh / /docs/using-the-api`. Do not add a trailing slash —
Fumadocs URLs have none, and a trailing slash redirects.

## Then: actually look

**You must Read each PNG** in `web/.screenshots/out/` and judge it — do not
skip this and assume a successful build means good layout. For every screenshot,
check:

- **No duplicated title.** Fumadocs renders the front-matter `title` through
  `DocsTitle`; a body `# Heading` produces a second identical title. There must
  be exactly one title per page.
- **Buttons/links are visible** and have real labels (watch for invisible buttons
  from bad/undefined CSS colors — white-on-white).
- **Code blocks render as highlighted code**, not literal ``` fences or raw
  backticks (a common failure when fenced code is wrapped in a shortcode/`<div>`).
- **Feature cards** have their icons and text; nothing empty or clipped.
- **No horizontal overflow** or content running off the edge; sensible spacing
  and vertical rhythm (hero → buttons → cards → content).
- **Dark mode** (the `*-dark.png`) is readable and the accent color still shows.
- **Mobile** (the `*-mobile.png`): nav collapses to a hamburger, no overflow,
  text wraps.
- **No "Page Not Found"** — every captured page resolved.

Report concrete findings (page + issue + fix). If you fix anything, **re-run the
script and re-Read the screenshots** to confirm the fix actually rendered — that
verify-fix-reverify loop is the point of this skill.

## Notes

- The library staging in `setup-browser.sh` is tuned for Ubuntu (24.04 "noble").
  On another distro it may report unmapped libraries; install the browser deps by
  your platform's normal means and re-run.
- Everything the skill downloads lives under `web/.screenshots/` (gitignored).
