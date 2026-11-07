import { expect, test } from '@playwright/test';
import { offlineBaseURL } from '../playwright.config';

// These run against a second instance of the app pointed at a closed port, so
// the failure is a real unreachable API rather than a mock. The point is that a
// dataset outage degrades to a message on the browse pages while the
// documentation — which is static and needs no API — stays up.

test.describe('with the dataset API unreachable', () => {
  test('the catalogue shows the error state instead of crashing', async ({ page }) => {
    const response = await page.goto(`${offlineBaseURL}/browse`);

    // A handled failure, not a 500 and not a stack trace.
    expect(response?.status()).toBe(200);
    await expect(page.getByText('The dataset API is unreachable')).toBeVisible();
    await expect(page.getByText(/documentation is unaffected/)).toBeVisible();
    await expect(page.locator('body')).not.toContainText('Application error');
  });

  test('the seasonal chart shows the error state', async ({ page }) => {
    await page.goto(`${offlineBaseURL}/seasons/2026/winter`);
    await expect(page.getByText('The dataset API is unreachable')).toBeVisible();
  });

  test('a series page shows the error state rather than a false 404', async ({ page }) => {
    const response = await page.goto(`${offlineBaseURL}/browse/demon-slayer`);

    // An outage must not be reported as "this series does not exist" — the
    // series page only swallows a genuine NotFound, and this asserts it.
    expect(response?.status()).toBe(200);
    await expect(page.getByText('The dataset API is unreachable')).toBeVisible();
  });

  test('the documentation still works', async ({ page }) => {
    await page.goto(`${offlineBaseURL}/docs/using-the-api`);
    await expect(page.getByRole('heading', { level: 1, name: 'Using the API' })).toBeVisible();
  });

  test('the landing page still works', async ({ page }) => {
    await page.goto(`${offlineBaseURL}/`);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  });
});
