import { expect, test } from "@playwright/test";

// /search and /seasons were separate pages before browsing was unified. They
// were live and linkable, so they redirect rather than 404 — and they carry
// their filters across, landing the reader on the same result set.

test("/search becomes the query box on /browse", async ({ page }) => {
  await page.goto("/search");
  await expect(page).toHaveURL(/\/browse$/);
});

test("/search?q= keeps the query", async ({ page }) => {
  await page.goto("/search?q=demon");
  await expect(page).toHaveURL(/\/browse\?q=demon/);
  await expect(
    page.locator("main").getByRole("link", { name: "Demon Slayer" }).first(),
  ).toBeVisible();
});

// matchCount reads the "N matches" the page reports for the current view.
const matchCount = async (page: import("@playwright/test").Page) =>
  Number(
    (
      await page
        .locator("main")
        .getByText(/([\d,]+) matches/)
        .innerText()
    )
      .match(/([\d,]+) matches/)![1]
      .replace(/,/g, ""),
  );

test("/seasons becomes the releases view", async ({ page }) => {
  await page.goto("/seasons");
  await expect(page).toHaveURL(/kind=releases/);
});

test("a seasonal chart URL keeps its year and quarter", async ({ page }) => {
  await page.goto("/seasons/2026/winter");

  await expect(page).toHaveURL(/year=2026/);
  await expect(page).toHaveURL(/quarter=winter/);
  // Narrowed, not a particular number. This asserted "80 matches" — the figure
  // the coverage page carried when it was written — and broke when an import
  // pulled more 2026 installments in. What the redirect has to preserve is the
  // filter, and a filter that survived shows fewer matches than none.
  const narrowed = await matchCount(page);
  await page.goto("/browse?kind=releases");
  expect(narrowed).toBeGreaterThan(0);
  expect(narrowed).toBeLessThan(await matchCount(page));
});

// The old seasonal page rejected these outright. The redirect forwards them to
// /browse, so /browse is what has to reject them — verified here so the
// behaviour is pinned at the URL a reader would actually follow.
test("a seasonal URL with a bad year still 404s after redirecting", async ({
  page,
}) => {
  expect((await page.goto("/seasons/0/winter"))?.status()).toBe(404);
  expect((await page.goto("/seasons/abcd/winter"))?.status()).toBe(404);
});

test("a seasonal URL with an unknown quarter still 404s", async ({ page }) => {
  expect((await page.goto("/seasons/2026/notaquarter"))?.status()).toBe(404);
});
