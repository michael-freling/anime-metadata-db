import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { ConnectError } from '@connectrpc/connect';
import { api, datasetYearSpan } from '@/lib/api';
import { ReleaseSeason } from '@/lib/gen/anime/v1/anime_pb';
import { ApiError, Card, Grid, isBadRequest, PageHeader, Pager, plural } from '@/components/browse';

const QUARTERS: Record<string, { label: string; value: ReleaseSeason }> = {
  winter: { label: 'Winter', value: ReleaseSeason.WINTER },
  spring: { label: 'Spring', value: ReleaseSeason.SPRING },
  summer: { label: 'Summer', value: ReleaseSeason.SUMMER },
  fall: { label: 'Fall', value: ReleaseSeason.FALL },
};

type Params = Promise<{ year: string; season: string }>;

export async function generateMetadata({ params }: { params: Params }): Promise<Metadata> {
  const { year, season } = await params;
  const quarter = QUARTERS[season];
  if (!quarter) return { title: 'Not found' };
  return {
    title: `${quarter.label} ${year} anime`,
    description: `Every anime series that premiered in ${quarter.label} ${year}, from the anime-metadata-db open dataset.`,
  };
}

export default async function SeasonPage({
  params,
  searchParams,
}: {
  params: Params;
  searchParams: Promise<{ token?: string }>;
}) {
  const { year, season } = await params;
  const { token = '' } = await searchParams;

  const quarter = QUARTERS[season];
  const releaseYear = Number(year);
  // Reject a bad quarter or a non-numeric year outright rather than querying
  // for something that can never match.
  //
  // The year must also fall inside what the dataset covers. Year 0 in
  // particular has to be rejected here: proto3 gives scalars no presence, so
  // release_year: 0 reaches the API as "no year filter" and it would answer
  // with every winter release across every year, rendered under a "Winter 0"
  // heading. The bounds come from the dataset, so they widen as it grows.
  if (!quarter || !Number.isInteger(releaseYear)) notFound();
  const { earliest, latest } = await datasetYearSpan();
  if (earliest && (releaseYear < earliest || releaseYear > latest)) notFound();
  if (!earliest && releaseYear <= 0) notFound();

  let page;
  try {
    page = await api.listWorks({
      releaseYear,
      releaseSeason: quarter.value,
      pageToken: token,
      limit: 24,
    });
  } catch (err) {
    return (
      <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-12">
        <PageHeader title={`${quarter.label} ${year}`} />
        <ApiError
          detail={err instanceof ConnectError ? err.message : String(err)}
          badRequest={isBadRequest(err)}
        />
      </main>
    );
  }

  return (
    <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-12">
      <Link href="/seasons" className="text-sm text-fd-muted-foreground hover:underline">
        ← Seasons
      </Link>
      <div className="mt-4">
        <PageHeader
          title={`${quarter.label} ${year}`}
          subtitle={`${plural(page.totalSize, 'release')} premiered this quarter.`}
        />
      </div>

      {page.works.length === 0 ? (
        <p className="rounded-xl border border-fd-border bg-fd-card p-6 text-fd-muted-foreground">
          Nothing from this quarter is in the dataset yet.
        </p>
      ) : (
        <Grid>
          {page.works.map((w) => (
            <Card
              key={w.id}
              href={`/browse/${w.seriesId}`}
              title={w.seriesTitle || w.title || w.id}
              meta={[
                w.number > 0 ? `Season ${w.number}` : w.title,
                w.episodeCount > 0 ? plural(w.episodeCount, 'episode') : null,
              ]
                .filter(Boolean)
                .join(' · ')}
            />
          ))}
        </Grid>
      )}

      <Pager
        basePath={`/seasons/${year}/${season}`}
        nextToken={page.nextPageToken}
        shown={page.works.length}
        total={page.totalSize}
      />
    </main>
  );
}
