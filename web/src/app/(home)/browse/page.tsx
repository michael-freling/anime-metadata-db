import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { ConnectError } from '@connectrpc/connect';
import { datasetYearSpan, localizedApi } from '@/lib/api';
import { LanguageSwitch } from '@/components/language';
import { humanizeId } from '@/lib/format';
import { EntryKind, ReleaseSeason } from '@/lib/gen/anime/v1/anime_pb';
import {
  ApiError,
  Card,
  Grid,
  isBadRequest,
  PageHeader,
  Pager,
  plural,
  yearsLabel,
} from '@/components/browse';

export const metadata: Metadata = {
  title: 'Browse',
  description:
    'Browse and search the dataset: shows, individual releases by year and quarter, characters and voice actors.',
};

const LIMIT = 24;

// One page for every way into the dataset. Separate Browse, Seasons and Search
// pages meant three routes over the same records, none of which could answer a
// question that crossed them — "Winter 2026, something with this actor" needed
// two pages and a manual cross-reference.
//
// Everything is a filter over one result set, and every filter lives in the
// URL, so any view is linkable and the back button behaves.
const KINDS = [
  { value: 'all', label: 'Everything' },
  { value: 'shows', label: 'Shows' },
  { value: 'releases', label: 'Releases' },
  { value: 'characters', label: 'Characters' },
  { value: 'staff', label: 'Voice actors' },
] as const;

type Kind = (typeof KINDS)[number]['value'];

const QUARTERS = [
  { value: 'winter', label: 'Winter', enum: ReleaseSeason.WINTER },
  { value: 'spring', label: 'Spring', enum: ReleaseSeason.SPRING },
  { value: 'summer', label: 'Summer', enum: ReleaseSeason.SUMMER },
  { value: 'fall', label: 'Fall', enum: ReleaseSeason.FALL },
] as const;

type Params = { q?: string; kind?: string; year?: string; quarter?: string; token?: string };

function buildHref(params: Params) {
  const search = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v) search.set(k, v);
  }
  const qs = search.toString();
  return qs ? `/browse?${qs}` : '/browse';
}

export default async function BrowsePage({ searchParams }: { searchParams: Promise<Params> }) {
  const sp = await searchParams;
  const q = (sp.q ?? '').trim();
  const kind: Kind = (KINDS.find((k) => k.value === sp.kind)?.value ?? 'all') as Kind;
  const quarter = QUARTERS.find((s) => s.value === sp.quarter);
  const token = sp.token ?? '';

  const span = await datasetYearSpan().catch(() => ({ earliest: 0, latest: 0 }));
  const years =
    span.earliest && span.latest
      ? Array.from({ length: span.latest - span.earliest + 1 }, (_, i) => span.latest - i)
      : [];
  // A year or quarter that is not real must be rejected, never ignored.
  // Silently dropping it is the dangerous case: proto3 gives scalars no
  // presence, so release_year: 0 reaches the API as "no year filter" and it
  // answers with every release across every year — rendered under a heading
  // claiming to be filtered. /seasons/0/winter did exactly that.
  //
  // Enforced only when the dataset's span is known; if the API is unreachable
  // the span is 0/0 and a real year must not start 404ing.
  const rawYear = sp.year ?? '';
  if (rawYear !== '') {
    const valid = years.length > 0 ? years.includes(Number(rawYear)) : Number(rawYear) > 0;
    if (!valid) notFound();
  }
  if ((sp.quarter ?? '') !== '' && !QUARTERS.some((s) => s.value === sp.quarter)) notFound();

  const year = rawYear === '' ? 0 : Number(rawYear);

  // Year and quarter only describe releases, so choosing either means the
  // reader is asking about releases whether or not they said so.
  const datedFilter = year > 0 || quarter !== undefined;
  const effective: Kind = datedFilter && kind === 'all' ? 'releases' : kind;

  // ...but an explicit kind wins. Asking for characters in 2026 is not an
  // invalid request, it is a request a year cannot narrow — so the query still
  // runs and the page says the date was ignored, rather than returning nothing
  // and blaming the data.
  const dateApplies = effective === 'releases' || effective === 'all';
  const datesIgnored = datedFilter && !dateApplies;

  const showShows = effective === 'all' || effective === 'shows';
  const showReleases = effective === 'all' || effective === 'releases';
  const showCharacters = effective === 'all' || effective === 'characters';
  const showStaff = effective === 'all' || effective === 'staff';

  // Only the single-kind views paginate: a mixed view has several result sets
  // and one cursor cannot describe them all.
  const single = effective !== 'all';
  const limit = single ? LIMIT : 12;

  const api = await localizedApi();

  let shows, releases, characters, staff;
  let error: unknown = null;
  try {
    [shows, releases, characters, staff] = await Promise.all([
      showShows
        ? q
          ? api.search({ query: q, limit, pageToken: single ? token : '' })
          : api.listCatalog({ limit, pageToken: single ? token : '' })
        : null,
      showReleases
        ? api.listWorks({
            query: q,
            releaseYear: year,
            releaseSeason: quarter?.enum ?? ReleaseSeason.SEASON_UNSPECIFIED,
            limit,
            pageToken: single ? token : '',
          })
        : null,
      showCharacters ? api.listCharacters({ query: q, limit, pageToken: single ? token : '' }) : null,
      showStaff ? api.listStaff({ query: q, limit, pageToken: single ? token : '' }) : null,
    ]);
  } catch (err) {
    error = err;
  }

  // Search and the catalogue return different messages for the same idea, so
  // they are normalised here rather than branched over at render time.
  type ShowItem = {
    id: string;
    title: string;
    kind: EntryKind;
    works?: number;
    episodes?: number;
    firstReleaseYear?: number;
    latestReleaseYear?: number;
  };
  const showItems: ShowItem[] = !shows
    ? []
    : 'results' in shows
      ? shows.results.map((r) => ({ id: r.id, title: r.title, kind: r.kind }))
      : shows.entries.map((e) => ({
          id: e.id,
          title: e.title,
          kind: e.kind,
          works: e.works,
          episodes: e.episodes,
          firstReleaseYear: e.firstReleaseYear,
          latestReleaseYear: e.latestReleaseYear,
        }));

  const totals =
    (shows?.totalSize ?? 0) +
    (releases?.totalSize ?? 0) +
    (characters?.totalSize ?? 0) +
    (staff?.totalSize ?? 0);

  const chip = (active: boolean) =>
    `rounded-full border px-3 py-1.5 text-sm transition-colors ${
      active
        ? 'border-fd-primary bg-fd-primary text-fd-primary-foreground'
        : 'border-fd-border hover:bg-fd-accent'
    }`;

  return (
    <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-12">
      <PageHeader
        title="Browse"
        subtitle="Everything in the dataset — shows, individual releases, characters and the voice actors who play them. Narrow with the filters; every view is a link you can share."
      />

      <form action="/browse" method="get" className="flex flex-wrap gap-2">
        <input
          type="search"
          name="q"
          defaultValue={q}
          placeholder="Tanjirō, Demon Slayer, Natsuki Hanae…"
          aria-label="Search the dataset"
          className="min-w-[16rem] flex-1 rounded-lg border border-fd-border bg-fd-card px-4 py-2.5 text-sm outline-none focus-visible:ring-2 focus-visible:ring-fd-primary"
        />
        <select
          name="year"
          defaultValue={year || ''}
          aria-label="Release year"
          className="rounded-lg border border-fd-border bg-fd-card px-3 py-2.5 text-sm"
        >
          <option value="">Any year</option>
          {years.map((y) => (
            <option key={y} value={y}>
              {y}
            </option>
          ))}
        </select>
        <select
          name="quarter"
          defaultValue={quarter?.value ?? ''}
          aria-label="Release quarter"
          className="rounded-lg border border-fd-border bg-fd-card px-3 py-2.5 text-sm"
        >
          <option value="">Any quarter</option>
          {QUARTERS.map((s) => (
            <option key={s.value} value={s.value}>
              {s.label}
            </option>
          ))}
        </select>
        {/* Carried through the form so changing the query keeps the chosen kind. */}
        <input type="hidden" name="kind" value={kind} />
        <button
          type="submit"
          className="rounded-lg bg-fd-primary px-5 py-2.5 text-sm font-medium text-fd-primary-foreground transition-opacity hover:opacity-90"
        >
          Apply
        </button>
      </form>

      <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
        <nav aria-label="Result type" className="flex flex-wrap gap-2">
        {KINDS.map((k) => (
          <Link
            key={k.value}
            href={buildHref({
              q,
              kind: k.value === 'all' ? '' : k.value,
              // Year and quarter mean nothing to characters or actors, so they
              // are not carried onto those chips — otherwise clicking one
              // silently applies a filter it cannot honour.
              ...(k.value === 'all' || k.value === 'releases'
                ? { year: sp.year, quarter: sp.quarter }
                : {}),
            })}
            className={chip(effective === k.value)}
          >
            {k.label}
          </Link>
        ))}
          {q || year || quarter ? (
            <Link href="/browse" className="rounded-full px-3 py-1.5 text-sm text-fd-muted-foreground underline">
              Clear
            </Link>
          ) : null}
        </nav>
        <LanguageSwitch />
      </div>

      {datesIgnored ? (
        <p className="mt-4 text-sm text-fd-muted-foreground">
          Year and quarter describe releases, so they do not narrow{' '}
          {effective === 'shows' ? 'shows' : effective === 'characters' ? 'characters' : 'voice actors'} —
          they are ignored here.
        </p>
      ) : null}

      {error ? (
        <div className="mt-8">
          <ApiError
            detail={error instanceof ConnectError ? error.message : String(error)}
            badRequest={isBadRequest(error)}
          />
        </div>
      ) : null}

      {!error && totals === 0 ? (
        <p className="mt-8 rounded-xl border border-fd-border bg-fd-card p-6 text-fd-muted-foreground">
          Nothing matches these filters. Character and voice-actor names come from Wikidata, so
          anything the dataset has no name for cannot be found by name yet.
        </p>
      ) : null}

      {shows && shows.totalSize > 0 ? (
        <Section title="Shows" total={shows.totalSize} single={single}>
          <Grid>
            {showItems.map((e) => (
              <Card
                key={e.id}
                href={`/browse/${e.id}`}
                title={e.title || humanizeId(e.id)}
                badge={e.kind === EntryKind.FRANCHISE ? 'Franchise' : undefined}
                meta={
                  e.works === undefined
                    ? undefined
                    : [
                        yearsLabel(e.firstReleaseYear ?? 0, e.latestReleaseYear ?? 0, span.earliest),
                        plural(e.works, 'work'),
                        (e.episodes ?? 0) > 0 ? plural(e.episodes ?? 0, 'episode') : null,
                      ]
                        .filter(Boolean)
                        .join(' · ')
                }
              />
            ))}
          </Grid>
        </Section>
      ) : null}

      {releases && releases.totalSize > 0 ? (
        <Section title="Releases" total={releases.totalSize} single={single}>
          <Grid>
            {releases.works.map((w) => (
              <Card
                key={w.id}
                href={`/browse/${w.seriesId}`}
                title={w.seriesTitle || w.title || humanizeId(w.id)}
                meta={[
                  w.number > 0 ? `Season ${w.number}` : w.title,
                  w.releaseYear > 0 ? w.releaseYear : null,
                  w.episodeCount > 0 ? plural(w.episodeCount, 'episode') : null,
                ]
                  .filter(Boolean)
                  .join(' · ')}
              />
            ))}
          </Grid>
        </Section>
      ) : null}

      {characters && characters.totalSize > 0 ? (
        <Section title="Characters" total={characters.totalSize} single={single}>
          <Grid>
            {characters.characters.map((c) => (
              <Card
                key={c.id}
                href={`/characters/${c.id}`}
                title={c.name || humanizeId(c.id)}
                meta={
                  c.voiceActors.length
                    ? `Voiced by ${c.voiceActors.map((v) => v.staffName || humanizeId(v.staffId)).join(', ')}`
                    : plural(c.appearancesTotal, 'appearance')
                }
              />
            ))}
          </Grid>
        </Section>
      ) : null}

      {staff && staff.totalSize > 0 ? (
        <Section title="Voice actors" total={staff.totalSize} single={single}>
          <Grid>
            {staff.staff.map((s) => (
              <Card key={s.id} href={`/staff/${s.id}`} title={s.name || humanizeId(s.id)} />
            ))}
          </Grid>
        </Section>
      ) : null}

      {single ? (
        <Pager
          basePath="/browse"
          nextToken={
            shows?.nextPageToken ||
            releases?.nextPageToken ||
            characters?.nextPageToken ||
            staff?.nextPageToken ||
            ''
          }
          shown={
            showItems.length +
            (releases?.works.length ?? 0) +
            (characters?.characters.length ?? 0) +
            (staff?.staff.length ?? 0)
          }
          total={totals}
          query={{
            ...(q ? { q } : {}),
            ...(kind !== 'all' ? { kind } : {}),
            ...(sp.year ? { year: sp.year } : {}),
            ...(sp.quarter ? { quarter: sp.quarter } : {}),
          }}
        />
      ) : null}
    </main>
  );
}

function Section({
  title,
  total,
  single,
  children,
}: {
  title: string;
  total: number;
  single: boolean;
  children: React.ReactNode;
}) {
  return (
    <section className="mt-10">
      <div className="mb-3 flex items-baseline justify-between gap-4 border-b border-fd-border pb-2">
        <h2 className="text-lg font-semibold">{title}</h2>
        <span className="text-sm text-fd-muted-foreground">
          {plural(total, 'match', 'matches')}
        </span>
      </div>
      {children}
      {/* A mixed view shows the first few of each; the type chips are how you
          get the rest, since one cursor cannot page four result sets. */}
      {!single && total > 12 ? (
        <p className="mt-3 text-sm text-fd-muted-foreground">
          Showing the first 12 — pick <strong>{title}</strong> above to page through them all.
        </p>
      ) : null}
    </section>
  );
}
