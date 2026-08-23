import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { cache } from 'react';
import { Code, ConnectError } from '@connectrpc/connect';
import { localizedApi } from '@/lib/api';
import type { Character, Franchise, Series } from '@/lib/gen/anime/v1/anime_pb';
import { ReleaseSeason, SpecialFormat } from '@/lib/gen/anime/v1/anime_pb';
import { ApiError, isBadRequest, PageHeader, Pager, plural } from '@/components/browse';
import { humanizeId } from '@/lib/format';

// An id names either a franchise or a series, and the API has a separate call
// for each. Try series first — they outnumber franchises heavily — and fall
// back. A genuine not-found is the only error swallowed here; anything else
// propagates so an outage is not reported as a missing series.
// Wrapped in cache() because generateMetadata and the page body both need it,
// and each call can issue two RPCs (getSeries, then getFranchise on NotFound).
// Connect's unary calls are POSTs, which Next's automatic request memoization
// does not dedupe, so without this a franchise page costs four round trips
// instead of two.
const load = cache(async (id: string): Promise<{ series?: Series; franchise?: Franchise } | null> => {
  try {
    const api = await localizedApi();
    const { series } = await api.getSeries({ id });
    if (series) return { series };
  } catch (err) {
    if (!(err instanceof ConnectError) || err.code !== Code.NotFound) throw err;
  }
  try {
    const api = await localizedApi();
    const { franchise } = await api.getFranchise({ id });
    if (franchise) return { franchise };
  } catch (err) {
    if (!(err instanceof ConnectError) || err.code !== Code.NotFound) throw err;
  }
  return null;
});

// How many cast members one page of a series shows, and how many a franchise
// page previews per series before pointing at that series' own page.
const CAST_LIMIT = 24;
const CAST_PREVIEW = 6;

// Cast is a page of a series' cast, not the whole of it.
//
// GetSeries embeds a series' cast, but that embed is capped, so rendering it
// directly silently dropped everyone past the cap — 48 of the 148 characters in
// tensei-shitara-slime-datta-ken, with nothing on the page to say so. Asking
// ListCharacters instead means the count is honest and the rest is reachable.
type Cast = { items: Character[]; total: number; nextToken: string };

const loadCast = cache(async (seriesID: string, token: string, limit: number): Promise<Cast> => {
  const api = await localizedApi();
  const res = await api.listCharacters({ seriesId: seriesID, limit, pageToken: token });
  return { items: res.characters, total: res.totalSize, nextToken: res.nextPageToken };
});

export async function generateMetadata({
  params,
}: {
  params: Promise<{ id: string }>;
}): Promise<Metadata> {
  const { id } = await params;
  try {
    const found = await load(id);
    const title = found?.series?.title ?? found?.franchise?.title;
    if (title) {
      return { title, description: `${title} — seasons, films and episodes in anime-metadata-db.` };
    }
  } catch {
    // Metadata must never break the page; fall through to the id.
  }
  return { title: humanizeId(id) };
}

const SEASON_LABEL: Record<number, string> = {
  [ReleaseSeason.WINTER]: 'Winter',
  [ReleaseSeason.SPRING]: 'Spring',
  [ReleaseSeason.SUMMER]: 'Summer',
  [ReleaseSeason.FALL]: 'Fall',
};

const FORMAT_LABEL: Record<number, string> = {
  [SpecialFormat.FORMAT_OVA]: 'OVA',
  [SpecialFormat.FORMAT_ONA]: 'ONA',
  [SpecialFormat.FORMAT_SPECIAL]: 'Special',
};

function Row({ title, meta }: { title: React.ReactNode; meta: React.ReactNode }) {
  return (
    <li className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1 border-b border-fd-border py-3 last:border-0">
      <span className="font-medium">{title}</span>
      <span className="text-sm text-fd-muted-foreground">{meta}</span>
    </li>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="mt-10">
      <h2 className="mb-2 text-lg font-semibold">{title}</h2>
      <ul>{children}</ul>
    </section>
  );
}

function SeriesBody({
  series,
  cast,
  pagerPath,
}: {
  series: Series;
  cast: Cast;
  // Set only for a page showing one series, where a single cursor describes the
  // whole view. A franchise page renders several casts at once, so it previews
  // each and links onward instead — the same reason /browse pages only its
  // single-kind views.
  pagerPath?: string;
}) {
  return (
    <>
      {series.seasonsTotal > 0 ? (
        <Section title="Seasons">
          {series.seasons.map((s) => (
            <Row
              key={s.id}
              title={s.title || `Season ${s.number}`}
              meta={[
                // Only label the number when the title is not already it.
                s.title ? `Season ${s.number}` : null,
                s.releaseYear
                  ? `${SEASON_LABEL[s.releaseSeason] ? `${SEASON_LABEL[s.releaseSeason]} ` : ''}${s.releaseYear}`
                  : null,
                s.episodesTotal ? plural(s.episodesTotal, 'episode') : null,
              ]
                .filter(Boolean)
                .join(' · ')}
            />
          ))}
        </Section>
      ) : null}

      {series.moviesTotal > 0 ? (
        <Section title="Films">
          {series.movies.map((m) => (
            <Row
              key={m.id}
              title={m.title || humanizeId(m.id)}
              meta={m.releaseYear ? String(m.releaseYear) : '—'}
            />
          ))}
        </Section>
      ) : null}

      {series.specialsTotal > 0 ? (
        <Section title="Specials">
          {series.specials.map((sp) => (
            <Row
              key={sp.id}
              title={sp.title || humanizeId(sp.id)}
              meta={[FORMAT_LABEL[sp.format], sp.releaseYear || null, sp.episodesTotal ? plural(sp.episodesTotal, 'episode') : null]
                .filter(Boolean)
                .join(' · ')}
            />
          ))}
        </Section>
      ) : null}

      {cast.total > 0 ? (
        <Section title="Cast">
          {cast.items.map((c) => (
            <Row
              key={c.id}
              title={
                <Link href={`/characters/${c.id}`} className="hover:underline">
                  {c.name || humanizeId(c.id)}
                </Link>
              }
              meta={c.voiceActors.map((v, i) => (
                // Keyed by staff *and* language: one actor can be cast twice
                // for the same character — the Japanese and English dub — so
                // the staff id alone is not unique. Same key as the character
                // page's own cast list.
                <span key={`${v.staffId}-${v.language}`}>
                  {i > 0 ? ', ' : null}
                  <Link href={`/staff/${v.staffId}`} className="hover:underline">
                    {v.staffName || humanizeId(v.staffId)}
                  </Link>
                </span>
              ))}
            />
          ))}
        </Section>
      ) : null}

      {cast.total > 0 && pagerPath ? (
        <Pager
          basePath={pagerPath}
          nextToken={cast.nextToken}
          shown={cast.items.length}
          total={cast.total}
        />
      ) : null}

      {cast.total > cast.items.length && !pagerPath ? (
        <p className="mt-3 text-sm text-fd-muted-foreground">
          Showing {cast.items.length} of {cast.total} —{' '}
          <Link href={`/browse/${series.id}`} className="underline">
            open {series.title || humanizeId(series.id)}
          </Link>{' '}
          for the rest.
        </p>
      ) : null}
    </>
  );
}

export default async function EntryPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ token?: string }>;
}) {
  const { id } = await params;
  const { token = '' } = await searchParams;

  let found: Awaited<ReturnType<typeof load>>;
  let casts: Record<string, Cast> = {};
  try {
    found = await load(id);
    // One request per series whose cast is rendered: the series itself, or each
    // series of a franchise. Issued together rather than in sequence.
    const ids = found?.series
      ? [found.series.id]
      : (found?.franchise?.series.map((s) => s.id) ?? []);
    const limit = found?.series ? CAST_LIMIT : CAST_PREVIEW;
    casts = Object.fromEntries(
      await Promise.all(
        ids.map(async (seriesID) => [
          seriesID,
          await loadCast(seriesID, found?.series ? token : '', limit),
        ]),
      ),
    );
  } catch (err) {
    return (
      <main className="mx-auto w-full max-w-3xl flex-1 px-6 py-12">
        <PageHeader title={humanizeId(id)} />
        <ApiError
          detail={err instanceof ConnectError ? err.message : String(err)}
          badRequest={isBadRequest(err)}
        />
      </main>
    );
  }
  if (!found) notFound();

  const { series, franchise } = found;
  const title = series?.title || franchise?.title || humanizeId(id);

  return (
    <main className="mx-auto w-full max-w-3xl flex-1 px-6 py-12">
      <Link href="/browse" className="text-sm text-fd-muted-foreground hover:underline">
        ← Browse
      </Link>
      <div className="mt-4">
        <PageHeader
          title={title}
          // The id is a slug the reader never typed and cannot act on; what
          // they want to know is what kind of thing this is and how much of it
          // there is.
          subtitle={
            franchise
              ? `Franchise · ${plural(franchise.seriesTotal, 'series', 'series')}`
              : [
                  'Series',
                  series?.seasonsTotal ? plural(series.seasonsTotal, 'season') : null,
                  series?.moviesTotal ? plural(series.moviesTotal, 'film') : null,
                  series?.specialsTotal ? plural(series.specialsTotal, 'special') : null,
                ]
                  .filter(Boolean)
                  .join(' · ')
          }
        />
      </div>

      {series ? (
        <SeriesBody
          series={series}
          cast={casts[series.id] ?? { items: [], total: 0, nextToken: '' }}
          pagerPath={`/browse/${series.id}`}
        />
      ) : null}

      {franchise
        ? franchise.series.map((s) => (
            <div key={s.id} className="mt-12">
              <h2 className="text-xl font-semibold">
                <Link href={`/browse/${s.id}`} className="hover:underline">
                  {s.title || humanizeId(s.id)}
                </Link>
              </h2>
              <SeriesBody series={s} cast={casts[s.id] ?? { items: [], total: 0, nextToken: '' }} />
            </div>
          ))
        : null}
    </main>
  );
}
