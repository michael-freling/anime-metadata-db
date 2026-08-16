import { expect, test } from '@playwright/test';

test('the seasons index links every year in the dataset', async ({ page }) => {
  await page.goto('/seasons');

  await expect(page.getByRole('heading', { name: 'Seasons' })).toBeVisible();
  // The index is derived from the works themselves, so 2026 — the bulk of the
  // committed dataset — must be present without anything hardcoding it.
  await expect(page.getByRole('link', { name: 'Winter' }).first()).toBeVisible();
  await expect(page.locator('text=2026').first()).toBeVisible();
});

// The seasonal chart is the view the dataset exists to serve, and its total is
// independently checkable: the coverage documentation states Winter 2026 = 80.
// If the flattening of seasons out of the hierarchy ever regressed, this is
// where it would show.
test('Winter 2026 reports the same total the coverage figures do', async ({ page }) => {
  await page.goto('/seasons/2026/winter');

  await expect(page.getByRole('heading', { level: 1, name: 'Winter 2026' })).toBeVisible();
  await expect(page.getByText(/80 releases premiered this quarter/)).toBeVisible();
});

test('a quarter filter excludes films and specials', async ({ page }) => {
  await page.goto('/seasons/2026/winter');

  // Every card in a quarter view is a season, so each carries a season number.
  const metas = await page.getByTestId('results').locator('a span').nth(1).innerText();
  expect(metas).toMatch(/Season \d+/);
});

test('the seasonal chart pages without losing its filter', async ({ page }) => {
  await page.goto('/seasons/2026/winter');

  const next = page.getByRole('link', { name: 'Next →' });
  await expect(next).toBeVisible();
  await next.click();

  await expect(page).toHaveURL(/\/seasons\/2026\/winter\?token=/);
  // The heading and total must survive the page change — a pager that dropped
  // the filter would silently start listing the whole dataset.
  await expect(page.getByRole('heading', { level: 1, name: 'Winter 2026' })).toBeVisible();
  await expect(page.getByText(/80 releases premiered this quarter/)).toBeVisible();
});

test('an unknown quarter is a 404 rather than an empty chart', async ({ page }) => {
  const response = await page.goto('/seasons/2026/notaquarter');
  expect(response?.status()).toBe(404);
});

test('a non-numeric year is a 404', async ({ page }) => {
  const response = await page.goto('/seasons/abcd/winter');
  expect(response?.status()).toBe(404);
});

// Year 0 is the dangerous one. proto3 gives scalars no presence, so
// release_year: 0 reaches the API as "no year filter" — without a guard this
// rendered every winter release across every year under a "Winter 0" heading.
test('year 0 is a 404, not the whole dataset under a "Winter 0" heading', async ({ page }) => {
  const response = await page.goto('/seasons/0/winter');
  expect(response?.status()).toBe(404);
  await expect(page.locator('body')).not.toContainText('Winter 0');
});

test('a year outside what the dataset covers is a 404', async ({ page }) => {
  expect((await page.goto('/seasons/1850/winter'))?.status()).toBe(404);
  expect((await page.goto('/seasons/3000/winter'))?.status()).toBe(404);
});
