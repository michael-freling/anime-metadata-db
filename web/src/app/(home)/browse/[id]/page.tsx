import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { cache } from 'react';
import { Code, ConnectError } from '@connectrpc/connect';
import { localizedApi } from '@/lib/api';
import type { Franchise, Series } from '@/lib/gen/anime/v1/anime_pb';
import { ReleaseSeason, SpecialFormat } from '@/lib/gen/anime/v1/anime_pb';
import { ApiError, isBadRequest, PageHeader, plural } from '@/components/browse';
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
  [SpecialFormat.OVA]: 'OVA',
  [SpecialFormat.ONA]: 'ONA',
  [SpecialFormat.SPECIAL]: 'Special',
};

function Row({ title, meta }: { title: string; meta: string }) {
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

function SeriesBody({ series }: { series: Series }) {
  return (
    <>
      {series.seasons.length > 0 ? (
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
                s.episodes.length ? plural(s.episodes.length, 'episode') : null,
              ]
                .filter(Boolean)
                .join(' · ')}
            />
          ))}
        </Section>
      ) : null}

      {series.movies.length > 0 ? (
        <Section title="Films">
          {series.movies.map((m) => (
            <Row
              key={m.id}
              title={m.title || m.id}
              meta={m.releaseYear ? String(m.releaseYear) : '—'}
            />
          ))}
        </Section>
      ) : null}

      {series.specials.length > 0 ? (
        <Section title="Specials">
          {series.specials.map((sp) => (
            <Row
              key={sp.id}
              title={sp.title || sp.id}
              meta={[FORMAT_LABEL[sp.format], sp.releaseYear || null, sp.episodes.length ? plural(sp.episodes.length, 'episode') : null]
                .filter(Boolean)
                .join(' · ')}
            />
          ))}
        </Section>
      ) : null}

      {series.characters.length > 0 ? (
        <Section title="Cast">
          {series.characters.map((c) => (
            <Row
              key={c.id}
              title={c.name || c.id}
              meta={c.voiceActors.map((v) => v.staffName || v.staffId).join(', ')}
            />
          ))}
        </Section>
      ) : null}
    </>
  );
}

export default async function EntryPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  let found;
  try {
    found = await load(id);
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
              ? `Franchise · ${plural(franchise.series.length, 'series', 'series')}`
              : [
                  'Series',
                  series?.seasons.length ? plural(series.seasons.length, 'season') : null,
                  series?.movies.length ? plural(series.movies.length, 'film') : null,
                  series?.specials.length ? plural(series.specials.length, 'special') : null,
                ]
                  .filter(Boolean)
                  .join(' · ')
          }
        />
      </div>

      {series ? <SeriesBody series={series} /> : null}

      {franchise
        ? franchise.series.map((s) => (
            <div key={s.id} className="mt-12">
              <h2 className="text-xl font-semibold">
                <Link href={`/browse/${s.id}`} className="hover:underline">
                  {s.title || s.id}
                </Link>
              </h2>
              <SeriesBody series={s} />
            </div>
          ))
        : null}
    </main>
  );
}
