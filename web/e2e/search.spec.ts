import { expect, test } from '@playwright/test';

// Search runs against the committed dataset, so these names are real entries.
// The point of the page is finding a character or a voice actor, which neither
// /browse (titles only) nor /seasons could do at all.

test('a character can be found by name', async ({ page }) => {
  await page.goto('/search');
  // The site navigation renders its own search control inside <main>, so
  // scoping is not enough to disambiguate: its accessible name is
  // "Search Ctrl K" while the form's is exactly "Search".
  const body = page.locator('main');
  await body.getByRole('searchbox').fill('tanjir');
  await body.getByRole('button', { name: 'Search', exact: true }).click();

  // The query lives in the URL, so a search is shareable and survives a reload.
  await expect(page).toHaveURL(/\/search\?q=tanjir/);
  await expect(body.getByRole('heading', { name: 'Characters' })).toBeVisible();

  const card = body.getByRole('link', { name: /Tanjir/ }).first();
  await expect(card).toBeVisible();
  // Results carry the cast, so a searcher sees who plays the character without
  // opening the page.
  await expect(body.getByText(/Voiced by .*Natsuki Hanae/)).toBeVisible();
});

test('a character result opens its page, which links to the voice actor', async ({ page }) => {
  await page.goto('/search?q=tanjir');
  await page.locator('main').getByRole('link', { name: /Tanjir/ }).first().click();

  await expect(page).toHaveURL(/\/characters\//);
  await expect(page.getByRole('heading', { name: 'Voiced by' })).toBeVisible();

  // Appearances are labelled with the series title, not the raw id — the API
  // denormalizes it so the page needs no extra call per appearance.
  await expect(page.locator('main').getByRole('link', { name: 'Demon Slayer' })).toBeVisible();

  await page.locator('main').getByRole('link', { name: 'Natsuki Hanae' }).first().click();
  await expect(page).toHaveURL(/\/staff\//);
  await expect(page.getByRole('heading', { name: 'Roles' })).toBeVisible();
  // And back the other way, so the graph is navigable in both directions.
  await expect(page.locator('main').getByRole('link', { name: /Tanjir/ }).first()).toBeVisible();
});

test('a voice actor can be found by name', async ({ page }) => {
  await page.goto('/search?q=natsuki');
  await expect(page.locator('main').getByRole('heading', { name: 'Voice actors' })).toBeVisible();
  await expect(page.locator('main').getByRole('link', { name: 'Natsuki Hanae' }).first()).toBeVisible();
});

test('a title still matches, alongside the cast', async ({ page }) => {
  await page.goto('/search?q=demon');
  await expect(page.locator('main').getByRole('heading', { name: 'Series and franchises' })).toBeVisible();
  await expect(page.locator('main').getByRole('link', { name: 'Demon Slayer' }).first()).toBeVisible();
});

test('search is case-insensitive and ignores surrounding whitespace', async ({ page }) => {
  await page.goto('/search?q=%20TANJIR%20');
  await expect(page.locator('main').getByRole('link', { name: /Tanjir/ }).first()).toBeVisible();
});

test('a query with no matches says so rather than showing an empty page', async ({ page }) => {
  await page.goto('/search?q=zzzznotathing');
  await expect(page.getByText(/Nothing matches/)).toBeVisible();
  await expect(page.locator('main').getByRole('heading', { name: 'Characters' })).toHaveCount(0);
});

test('an empty search invites a query instead of listing everything', async ({ page }) => {
  await page.goto('/search');
  await expect(page.getByText(/Type a name to begin/)).toBeVisible();
  await expect(page.locator('main').getByRole('heading', { name: 'Characters' })).toHaveCount(0);
});

test('an unknown character id is a 404', async ({ page }) => {
  expect((await page.goto('/characters/no-such-character'))?.status()).toBe(404);
  expect((await page.goto('/staff/no-such-person'))?.status()).toBe(404);
});
