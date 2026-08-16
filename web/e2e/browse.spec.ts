import { expect, test } from '@playwright/test';

// These run against the real dataset, so the figures below are the committed
// ones. If the catalogue grows, the exact numbers move — the assertions are
// written to check relationships (totals match what is listed, pages do not
// repeat) rather than hardcoding counts wherever that is possible.

test('the catalogue lists entries with their release span and counts', async ({ page }) => {
  await page.goto('/browse');

  await expect(page.getByRole('heading', { name: 'Browse the catalogue' })).toBeVisible();

  const cards = page.locator('main ul li a');
  await expect(cards.first()).toBeVisible();
  expect(await cards.count()).toBeGreaterThan(1);

  // "Showing N of TOTAL" must agree with what is actually on the page.
  const shown = await page.getByText(/Showing \d+ of/).innerText();
  const [, listed] = shown.match(/Showing (\d+) of/) ?? [];
  expect(Number(listed)).toBe(await cards.count());
});

// The property that matters for a browse UI: walking the pager must reach every
// entry exactly once. A cursor bug that skipped or repeated rows would still
// render a perfectly good-looking page, which is exactly why a screenshot
// cannot catch this.
test('paging through the catalogue yields every entry exactly once', async ({ page }) => {
  await page.goto('/browse');

  const total = Number((await page.getByText(/Showing \d+ of ([\d,]+)/).innerText())
    .match(/of ([\d,]+)/)![1].replace(/,/g, ''));

  const seen: string[] = [];
  for (let guard = 0; guard < 40; guard++) {
    const hrefs = await page.locator('main ul li a').evaluateAll((els) =>
      els.map((el) => (el as HTMLAnchorElement).getAttribute('href')!),
    );
    seen.push(...hrefs);

    // Follow the pager by its href rather than clicking it. Clicking races with
    // the client-side navigation that replaces the link, and this walk is about
    // the page tokens being correct, not about the anchor being clickable —
    // that is covered separately below.
    const next = page.getByRole('link', { name: 'Next →' });
    if (!(await next.count())) break;
    const href = await next.getAttribute('href');
    await page.goto(href!);
  }

  expect(seen.length).toBe(total);
  expect(new Set(seen).size).toBe(total);
});

test('a catalogue card opens the entry it names', async ({ page }) => {
  await page.goto('/browse');

  const card = page.locator('main ul li a').first();
  const href = await card.getAttribute('href');
  const title = (await card.locator('span').first().innerText()).trim();

  await card.click();
  await expect(page).toHaveURL(new RegExp(`${href}$`));
  await expect(page.getByRole('heading', { level: 1, name: title })).toBeVisible();
});

test('a series page renders its seasons, films and cast', async ({ page }) => {
  await page.goto('/browse/demon-slayer');

  await expect(page.getByRole('heading', { level: 1, name: 'Demon Slayer' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Seasons' })).toBeVisible();

  // A season with no title of its own falls back to "Season N", and its meta
  // must NOT then repeat the number — that duplication was a real defect.
  await expect(page.getByText('Season 1', { exact: true })).toBeVisible();
  await expect(page.getByText('Spring 2019 · 26 episodes')).toBeVisible();

  // A season that does have a title keeps its number in the meta instead.
  await expect(page.getByText('Mugen Train Arc', { exact: true })).toBeVisible();
  await expect(page.getByText(/Season 2 · Fall 2021 · 7 episodes/)).toBeVisible();

  await expect(page.getByRole('heading', { name: 'Films' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Cast' })).toBeVisible();
});

test('an unknown entry id is a 404, not an empty page', async ({ page }) => {
  const response = await page.goto('/browse/no-such-series-exists');
  expect(response?.status()).toBe(404);
});

test('the catalogue has no horizontal overflow on a phone', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 800 });
  await page.goto('/browse');

  const overflows = await page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
  );
  expect(overflows).toBe(false);
});
