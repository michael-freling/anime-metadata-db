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

// Unifying the pages deleted the guard that caught this, and the bug came
// straight back: proto3 gives scalars no presence, so release_year: 0 reaches
// the API as "no year filter" and it answers with every release across every
// year — under a heading claiming to be filtered. A bad year must be rejected,
// never quietly dropped.
test('a year the dataset does not cover is a 404, not an unfiltered list', async ({ page }) => {
  expect((await page.goto('/browse?year=0&quarter=winter'))?.status()).toBe(404);
  expect((await page.goto('/browse?year=abcd&quarter=winter'))?.status()).toBe(404);
  expect((await page.goto('/browse?year=1850'))?.status()).toBe(404);
  expect((await page.goto('/browse?year=3000'))?.status()).toBe(404);
});

test('an unknown quarter is a 404', async ({ page }) => {
  expect((await page.goto('/browse?quarter=notaquarter'))?.status()).toBe(404);
});

// A year cannot narrow a character, but asking for one is not an error: the
// query must still run rather than returning nothing and blaming the dataset.
test('a kind that ignores the date filters still returns its results', async ({ page }) => {
  await page.goto('/browse?kind=characters&year=2026&q=tanjir');

  const body = page.locator('main');
  await expect(body.getByRole('heading', { name: 'Characters' })).toBeVisible();
  await expect(body.getByRole('link', { name: /Tanjir/ }).first()).toBeVisible();
  await expect(body.getByText(/Nothing matches these filters/)).toHaveCount(0);
  // And it says the date was ignored rather than leaving it to be inferred.
  await expect(body.getByText(/do not narrow characters/)).toBeVisible();
});

test('a kind chip does not carry a filter it cannot honour', async ({ page }) => {
  await page.goto('/browse?year=2026&quarter=winter');
  await page.locator('main').getByRole('link', { name: 'Characters' }).first().click();

  await expect(page).toHaveURL(/kind=characters/);
  await expect(page).not.toHaveURL(/year=/);
  await expect(page).not.toHaveURL(/quarter=/);
});

// Titles come back resolved by the API from Accept-Language, so switching is a
// server round trip. The dataset carries an English translation and a native
// original, which is what the two options mean — offering more would be a lie.
test('the language switch changes the titles the API returns', async ({ page }) => {
  await page.goto('/browse?q=demon');
  const body = page.locator('main');
  await expect(body.getByRole('link', { name: 'Demon Slayer' }).first()).toBeVisible();

  await body.getByRole('button', { name: '日本語' }).click();

  await expect(body.getByRole('link', { name: '鬼滅の刃' }).first()).toBeVisible();
  await expect(body.getByRole('link', { name: 'Demon Slayer' })).toHaveCount(0);
});

test('the chosen language persists across pages', async ({ page }) => {
  await page.goto('/browse');
  const japanese = page.locator('main').getByRole('button', { name: '日本語' });
  await japanese.click();

  // Wait for the switch to actually take effect rather than sleeping: the
  // action's response is what stores the cookie, and navigating before it
  // lands would race it. The active state is the signal.
  await expect(japanese).toHaveAttribute('aria-current', 'true');

  // A preference, not a view: it survives navigation without riding in the URL.
  await page.goto('/browse/demon-slayer');
  await expect(page.getByRole('heading', { level: 1, name: '鬼滅の刃' })).toBeVisible();
  await expect(page).not.toHaveURL(/lang=/);
});

// Ids are slugs the reader never typed and cannot act on. They were printed in
// every detail-page subtitle; what belongs there is what the thing is and how
// much of it there is.
// A blanket check rather than one assertion per known field: the subtitles were
// fixed once and slugs still reached readers through the roles list, untitled
// films and specials, and cast fallbacks. This catches whichever one is missed
// next, without hardcoding a list of ids — a page's own links name every id it
// knows about, and none of them should be visible as text.
test('no page shows a reader an id it links to', async ({ page }) => {
  for (const path of [
    '/browse',
    '/browse?q=demon',
    '/browse/demon-slayer',
    '/characters/tanjiro-kamado',
    '/staff/ayako-kawasumi',
  ]) {
    await page.goto(path);
    const body = page.locator('main').last();

    const ids = new Set(
      (
        await body.locator('a[href]').evaluateAll((els) =>
          els.map((el) => (el as HTMLAnchorElement).getAttribute('href') ?? ''),
        )
      )
        .filter((href) => /^\/(browse|characters|staff)\/[^?#]+$/.test(href))
        .map((href) => href.split('/').pop()!),
    );
    expect(ids.size, `${path} links to nothing`).toBeGreaterThan(0);

    const text = await body.innerText();
    for (const id of ids) {
      // Matched on word boundaries, not as a bare substring. An id is a slug,
      // so a leaked one is surrounded by punctuation or whitespace; a substring
      // test also fires on any name that happens to contain one, which is not
      // a leak. The Demon Slayer cast is the case: the character `suma` sits
      // beside "Zenitsu Agatsuma", and "Agat-suma" contains it.
      const leaked = new RegExp(`\\b${id.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}\\b`);
      expect(text, `${path} displays the id "${id}"`).not.toMatch(leaked);
    }
  }
});

test('detail pages describe the entry instead of printing its id', async ({ page }) => {
  await page.goto('/browse/demon-slayer');
  await expect(page.getByText(/Series · 5 seasons · 2 films/)).toBeVisible();
  await expect(page.getByText('demon-slayer', { exact: true })).toHaveCount(0);

  await page.goto('/characters/tanjiro-kamado');
  await expect(page.getByText(/appearance/)).toBeVisible();
  await expect(page.getByText('tanjiro-kamado', { exact: true })).toHaveCount(0);

  await page.goto('/staff/natsuki-hanae');
  await expect(page.getByText(/role/)).toBeVisible();
  await expect(page.getByText('natsuki-hanae', { exact: true })).toHaveCount(0);

  // The roles list named its series by id and its language by code.
  const body = page.locator('main').last();
  await expect(body).toContainText('Demon Slayer');
  await expect(body).toContainText('Japanese');
  await expect(body).not.toContainText('demon-slayer');
});

// The series page used to render a capped cast: tensei-shitara-slime-datta-ken
// has 148 characters and the page showed 100 — no count, nothing to indicate 48
// were missing. GetSeries now returns a series whole, so the fix is not a pager
// but the absence of one: every character is on the page, and nothing claims
// otherwise.
test('a large cast renders in full rather than being cut off', async ({ page }) => {
  await page.goto('/browse/tensei-shitara-slime-datta-ken');
  const body = page.locator('main').last();
  await expect(body.getByRole('heading', { name: 'Cast' })).toBeVisible();

  const links = await body
    .locator('a[href^="/characters/"]')
    .evaluateAll((els) => els.map((e) => (e as HTMLAnchorElement).getAttribute('href')!));
  // Comfortably past the old cap of 25 and the older one of 100, so a
  // reintroduced limit fails here rather than passing quietly.
  expect(links.length).toBeGreaterThan(100);
  expect(new Set(links).size).toBe(links.length);

  // Nothing on a series page may claim a partial view any more.
  await expect(body.getByText(/Showing \d+ of/)).toHaveCount(0);
  await expect(body.getByRole('link', { name: /Next/ })).toHaveCount(0);
});

// A franchise page renders each of its series in one GetFranchise call, cast
// included. Each cast is previewed rather than shown whole, so where the
// preview cuts, the page has to say so and link onward.
test('a franchise renders every series, previewing each cast honestly', async ({ page }) => {
  await page.goto('/browse/fate');
  const body = page.locator('main').last();

  // Both series of the franchise, each with its own cast section — the whole
  // page comes from one request, so a missing one is a real regression rather
  // than a slow call.
  await expect(body.getByRole('heading', { name: 'Fate/Zero' })).toBeVisible();
  await expect(body.getByRole('heading', { name: 'Fate/stay night' })).toBeVisible();
  const casts = body.getByRole('heading', { name: 'Cast' });
  expect(await casts.count()).toBeGreaterThan(1);

  // Where a cast is cut to the preview, the count must be the real one and the
  // link out must be there. Vacuous while every Fate series has a small cast,
  // which is why the assertions above carry the test.
  for (const hint of await body.getByText(/Showing \d+ of \d+ —/).all()) {
    const [, shown, total] = (await hint.textContent())!.match(/Showing (\d+) of (\d+)/)!;
    expect(Number(total)).toBeGreaterThan(Number(shown));
    await expect(hint.getByRole('link')).toBeVisible();
  }
});

// Counting from `.length` was wrong when collections were capped: capping
// episodes at 25 turned "26 episodes" into "25 episodes" on the series page,
// silently. Nothing is capped now, so `.length` IS the count — and this asserts
// that directly, on the case that used to differ.
test('episode counts are the real counts', async ({ page }) => {
  await page.goto('/browse/demon-slayer');
  const body = page.locator('main').last();
  // Season 1 has 26 episodes, one more than the old embed cap, so a
  // reintroduced cap reads 25 here and this fails.
  await expect(body.getByText('Spring 2019 · 26 episodes')).toBeVisible();
});

// A staff page is one call now, and shows every role. Whatever it claims in the
// subtitle has to be what it rendered — there is no "rest" to link to.
test('a staff page shows every role it claims', async ({ page }) => {
  await page.goto('/staff/ayako-kawasumi');
  const body = page.locator('main').last();
  const subtitle = await body.locator('header p').textContent();
  const claimed = Number(subtitle!.match(/(\d+)/)![1]);
  expect(claimed).toBeGreaterThan(0);
  await expect(body.locator('a[href^="/characters/"]')).toHaveCount(claimed);
  await expect(body.getByText(/Showing \d+ of/)).toHaveCount(0);
});
