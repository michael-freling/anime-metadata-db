import { expect, test } from '@playwright/test';

// /browse is the only way into the dataset: what used to be three pages
// (Browse, Seasons, Search) are now filters on one. These run against the real
// dataset, so the figures below are the committed ones.

const results = (page: import('@playwright/test').Page) => page.getByTestId('results').locator('a');

test('with no filters it lists every kind of thing in the dataset', async ({ page }) => {
  await page.goto('/browse');

  const body = page.locator('main');
  for (const section of ['Shows', 'Releases', 'Characters', 'Voice actors']) {
    await expect(body.getByRole('heading', { name: section })).toBeVisible();
  }
});

test('a text query searches shows, releases, characters and actors at once', async ({ page }) => {
  await page.goto('/browse?q=demon');

  const body = page.locator('main');
  await expect(body.getByRole('heading', { name: 'Shows' })).toBeVisible();
  await expect(body.getByRole('link', { name: 'Demon Slayer' }).first()).toBeVisible();
  // A title match reaches releases too, which the old Seasons page could not do.
  await expect(body.getByRole('heading', { name: 'Releases' })).toBeVisible();
});

test('a character is findable by name, with its cast shown', async ({ page }) => {
  await page.goto('/browse');

  const body = page.locator('main');
  await body.getByRole('searchbox').fill('tanjir');
  await body.getByRole('button', { name: 'Apply' }).click();

  // Filters live in the URL, so any view is linkable and survives a reload.
  await expect(page).toHaveURL(/\/browse\?.*q=tanjir/);
  await expect(body.getByRole('heading', { name: 'Characters' })).toBeVisible();
  await expect(body.getByText(/Voiced by .*Natsuki Hanae/)).toBeVisible();
});

test('the quarter filter reproduces what the Seasons page used to show', async ({ page }) => {
  await page.goto('/browse?year=2026&quarter=winter');

  const body = page.locator('main');
  // 80 is the figure the coverage documentation states for Winter 2026.
  await expect(body.getByText(/80 matches/)).toBeVisible();
  // Choosing a year or quarter means asking about releases, so the other
  // kinds are not listed unasked.
  await expect(body.getByRole('heading', { name: 'Characters' })).toHaveCount(0);
});

test('a kind chip narrows to one result type and can be paged', async ({ page }) => {
  await page.goto('/browse');
  await page.locator('main').getByRole('link', { name: 'Characters' }).first().click();

  await expect(page).toHaveURL(/kind=characters/);
  const body = page.locator('main');
  await expect(body.getByRole('heading', { name: 'Shows' })).toHaveCount(0);
  await expect(body.getByRole('link', { name: 'Next →' })).toBeVisible();
});

test('paging a narrowed view keeps its filters', async ({ page }) => {
  await page.goto('/browse?kind=releases&year=2026&quarter=winter');

  const next = page.getByRole('link', { name: 'Next →' });
  await expect(next).toBeVisible();
  await next.click();

  await expect(page).toHaveURL(/year=2026/);
  await expect(page).toHaveURL(/quarter=winter/);
  // A pager that dropped the filter would silently start listing everything.
  await expect(page.locator('main').getByText(/80 matches/)).toBeVisible();
});

test('paging the catalogue yields every entry exactly once', async ({ page }) => {
  await page.goto('/browse?kind=shows');

  const total = Number(
    (await page.locator('main').getByText(/Showing \d+ of ([\d,]+)/).innerText())
      .match(/of ([\d,]+)/)![1]
      .replace(/,/g, ''),
  );

  const seen: string[] = [];
  for (let guard = 0; guard < 40; guard++) {
    seen.push(
      ...(await results(page).evaluateAll((els) =>
        els.map((el) => (el as HTMLAnchorElement).getAttribute('href')!),
      )),
    );
    const next = page.getByRole('link', { name: 'Next →' });
    if (!(await next.count())) break;
    await page.goto((await next.getAttribute('href'))!);
  }

  expect(seen.length).toBe(total);
  expect(new Set(seen).size).toBe(total);
});

test('a result opens the entry it names', async ({ page }) => {
  await page.goto('/browse?kind=shows');

  const card = results(page).first();
  const href = await card.getAttribute('href');
  const title = (await card.locator('span').first().innerText()).trim();

  await card.click();
  await expect(page).toHaveURL(new RegExp(`${href}$`));
  await expect(page.getByRole('heading', { level: 1, name: title })).toBeVisible();
});

test('a character result opens its page and links on to the voice actor', async ({ page }) => {
  await page.goto('/browse?kind=characters&q=tanjir');
  await page.locator('main').getByRole('link', { name: /Tanjir/ }).first().click();

  await expect(page).toHaveURL(/\/characters\//);
  await expect(page.getByRole('heading', { name: 'Voiced by' })).toBeVisible();
  // Appearances carry the series title, not a raw id.
  await expect(page.locator('main').getByRole('link', { name: 'Demon Slayer' })).toBeVisible();

  await page.locator('main').getByRole('link', { name: 'Natsuki Hanae' }).first().click();
  await expect(page).toHaveURL(/\/staff\//);
  await expect(page.getByRole('heading', { name: 'Roles' })).toBeVisible();
  await expect(page.locator('main').getByRole('link', { name: /Tanjir/ }).first()).toBeVisible();
});

test('a series page renders its seasons, films and cast', async ({ page }) => {
  await page.goto('/browse/demon-slayer');

  await expect(page.getByRole('heading', { level: 1, name: 'Demon Slayer' })).toBeVisible();
  await expect(page.getByText('Season 1', { exact: true })).toBeVisible();
  await expect(page.getByText('Spring 2019 · 26 episodes')).toBeVisible();
  await expect(page.getByText(/Season 2 · Fall 2021 · 7 episodes/)).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Films' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Cast' })).toBeVisible();
});

test('year spans never start before the dataset covers anything', async ({ page }) => {
  await page.goto('/browse?kind=shows');

  const metas = await results(page)
    .locator('span')
    .nth(1)
    .evaluateAll((els) => els.map((el) => el.textContent ?? ''));
  const years = metas.flatMap((m) => m.match(/\b\d{4}\b/g) ?? []).map(Number);

  expect(years.length).toBeGreaterThan(0);
  for (const y of years) expect(y).toBeGreaterThanOrEqual(2006);
});

test('filters that match nothing say so', async ({ page }) => {
  await page.goto('/browse?q=zzzznotathing');
  await expect(page.getByText(/Nothing matches these filters/)).toBeVisible();
});

test('unknown ids are 404s, not empty pages', async ({ page }) => {
  expect((await page.goto('/browse/no-such-series'))?.status()).toBe(404);
  expect((await page.goto('/characters/no-such-character'))?.status()).toBe(404);
  expect((await page.goto('/staff/no-such-person'))?.status()).toBe(404);
});

test('no horizontal overflow on a phone', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 800 });
  await page.goto('/browse');

  const overflows = await page.evaluate(
    () => document.documentElement.scrollWidth > document.documentElement.clientWidth + 1,
  );
  expect(overflows).toBe(false);
});
