import type { Metadata } from 'next';
import Link from 'next/link';
import { ConnectError } from '@connectrpc/connect';
import { api } from '@/lib/api';
import { EntryKind } from '@/lib/gen/anime/v1/anime_pb';
import { ApiError, Card, Grid, isBadRequest, PageHeader, plural } from '@/components/browse';

export const metadata: Metadata = {
  title: 'Search',
  description: 'Search the dataset by series, franchise, character or voice actor.',
};

const LIMIT = 12;

// Three separate calls rather than one combined endpoint. Each result type
// paginates independently — a query can match forty characters and two series —
// and they run concurrently on the server, so a single round trip's latency
// covers all three.
async function search(query: string) {
  const [catalog, characters, staff] = await Promise.all([
    api.search({ query, limit: LIMIT }),
    api.listCharacters({ query, limit: LIMIT }),
    api.listStaff({ query, limit: LIMIT }),
  ]);
  return { catalog, characters, staff };
}

function Section({
  title,
  total,
  shown,
  children,
}: {
  title: string;
  total: number;
  shown: number;
  children: React.ReactNode;
}) {
  return (
    <section className="mt-10">
      <div className="mb-3 flex items-baseline justify-between gap-4 border-b border-fd-border pb-2">
        <h2 className="text-lg font-semibold">{title}</h2>
        <span className="text-sm text-fd-muted-foreground">
          {total > shown ? `showing ${shown} of ${total.toLocaleString()}` : plural(total, 'match', 'matches')}
        </span>
      </div>
      {children}
    </section>
  );
}

export default async function SearchPage({
  searchParams,
}: {
  searchParams: Promise<{ q?: string }>;
}) {
  const { q = '' } = await searchParams;
  const query = q.trim();

  let results: Awaited<ReturnType<typeof search>> | null = null;
  let error: unknown = null;
  if (query) {
    try {
      results = await search(query);
    } catch (err) {
      error = err;
    }
  }

  const nothing =
    results &&
    results.catalog.totalSize === 0 &&
    results.characters.totalSize === 0 &&
    results.staff.totalSize === 0;

  return (
    <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-12">
      <PageHeader
        title="Search"
        subtitle="Across series and franchises, characters, and the voice actors who play them."
      />

      {/* A plain GET form: the query lives in the URL, so a search is
          linkable, shareable and survives a reload, and the page keeps
          working without JavaScript. */}
      <form action="/search" method="get" className="flex gap-2">
        <input
          type="search"
          name="q"
          defaultValue={query}
          autoFocus
          placeholder="Tanjirō, Demon Slayer, Natsuki Hanae…"
          aria-label="Search the dataset"
          className="flex-1 rounded-lg border border-fd-border bg-fd-card px-4 py-2.5 text-sm outline-none focus-visible:ring-2 focus-visible:ring-fd-primary"
        />
        <button
          type="submit"
          className="rounded-lg bg-fd-primary px-5 py-2.5 text-sm font-medium text-fd-primary-foreground transition-opacity hover:opacity-90"
        >
          Search
        </button>
      </form>

      {error ? (
        <div className="mt-8">
          <ApiError
            detail={error instanceof ConnectError ? error.message : String(error)}
            badRequest={isBadRequest(error)}
          />
        </div>
      ) : null}

      {!query ? (
        <p className="mt-8 text-fd-muted-foreground">
          Type a name to begin — or{' '}
          <Link href="/browse" className="underline">
            browse the whole catalogue
          </Link>
          .
        </p>
      ) : null}

      {nothing ? (
        <p className="mt-8 rounded-xl border border-fd-border bg-fd-card p-6 text-fd-muted-foreground">
          Nothing matches <strong className="text-fd-foreground">{query}</strong>. Names come from
          Wikidata, so a character the dataset has no name for cannot be found by name yet.
        </p>
      ) : null}

      {results && results.catalog.totalSize > 0 ? (
        <Section
          title="Series and franchises"
          total={results.catalog.totalSize}
          shown={results.catalog.results.length}
        >
          <Grid>
            {results.catalog.results.map((r) => (
              <Card
                key={r.id}
                href={`/browse/${r.id}`}
                title={r.title || r.id}
                badge={r.kind === EntryKind.FRANCHISE ? 'Franchise' : undefined}
              />
            ))}
          </Grid>
        </Section>
      ) : null}

      {results && results.characters.totalSize > 0 ? (
        <Section
          title="Characters"
          total={results.characters.totalSize}
          shown={results.characters.characters.length}
        >
          <Grid>
            {results.characters.characters.map((c) => (
              <Card
                key={c.id}
                href={`/characters/${c.id}`}
                title={c.name || c.id}
                meta={
                  c.voiceActors.length
                    ? `Voiced by ${c.voiceActors.map((v) => v.staffName || v.staffId).join(', ')}`
                    : plural(c.appearances.length, 'appearance')
                }
              />
            ))}
          </Grid>
        </Section>
      ) : null}

      {results && results.staff.totalSize > 0 ? (
        <Section
          title="Voice actors"
          total={results.staff.totalSize}
          shown={results.staff.staff.length}
        >
          <Grid>
            {results.staff.staff.map((s) => (
              <Card key={s.id} href={`/staff/${s.id}`} title={s.name || s.id} />
            ))}
          </Grid>
        </Section>
      ) : null}
    </main>
  );
}
