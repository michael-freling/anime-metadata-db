import type { Metadata } from 'next';
import { ConnectError } from '@connectrpc/connect';
import { api, earliestReleaseYear } from '@/lib/api';
import { EntryKind } from '@/lib/gen/anime/v1/anime_pb';
import { ApiError, Card, Grid, PageHeader, Pager, plural, yearsLabel } from '@/components/browse';

export const metadata: Metadata = {
  title: 'Browse the catalogue',
  description: 'Every franchise and series in the dataset, with its release span and episode count.',
};

export default async function BrowsePage({
  searchParams,
}: {
  searchParams: Promise<{ token?: string }>;
}) {
  const { token = '' } = await searchParams;

  let page;
  let floor = 0;
  try {
    // The floor comes from the dataset itself, so it never goes stale as older
    // works are added.
    [page, floor] = await Promise.all([
      api.listCatalog({ pageToken: token, limit: 24 }),
      earliestReleaseYear(),
    ]);
  } catch (err) {
    return (
      <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-12">
        <PageHeader title="Browse the catalogue" />
        <ApiError detail={err instanceof ConnectError ? err.message : String(err)} />
      </main>
    );
  }

  return (
    <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-12">
      <PageHeader
        title="Browse the catalogue"
        subtitle="Every franchise and standalone series in the dataset. A franchise groups several storylines; a series is one continuity."
      />

      <Grid>
        {page.entries.map((entry) => (
          <Card
            key={entry.id}
            href={`/browse/${entry.id}`}
            title={entry.title || entry.id}
            badge={entry.kind === EntryKind.FRANCHISE ? 'Franchise' : undefined}
            meta={
              [
                yearsLabel(entry.firstReleaseYear, entry.latestReleaseYear, floor),
                plural(entry.works, 'work'),
                entry.episodes > 0 ? plural(entry.episodes, 'episode') : null,
              ]
                .filter(Boolean)
                .join(' · ')
            }
          />
        ))}
      </Grid>

      <Pager
        basePath="/browse"
        nextToken={page.nextPageToken}
        shown={page.entries.length}
        total={page.totalSize}
      />
    </main>
  );
}
