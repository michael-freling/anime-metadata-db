import { expect, test } from '@playwright/test';

// /search and /seasons were separate pages before browsing was unified. They
// were live and linkable, so they redirect rather than 404 — and they carry
// their filters across, landing the reader on the same result set.

test('/search becomes the query box on /browse', async ({ page }) => {
  await page.goto('/search');
  await expect(page).toHaveURL(/\/browse$/);
});

test('/search?q= keeps the query', async ({ page }) => {
  await page.goto('/search?q=demon');
  await expect(page).toHaveURL(/\/browse\?q=demon/);
  await expect(page.locator('main').getByRole('link', { name: 'Demon Slayer' }).first()).toBeVisible();
});

test('/seasons becomes the releases view', async ({ page }) => {
  await page.goto('/seasons');
  await expect(page).toHaveURL(/kind=releases/);
});

test('a seasonal chart URL keeps its year and quarter', async ({ page }) => {
  await page.goto('/seasons/2026/winter');

  await expect(page).toHaveURL(/year=2026/);
  await expect(page).toHaveURL(/quarter=winter/);
  // And still shows the same set it used to.
  await expect(page.locator('main').getByText(/80 matches/)).toBeVisible();
});
