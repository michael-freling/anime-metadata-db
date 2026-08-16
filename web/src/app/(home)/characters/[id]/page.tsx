import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { cache } from 'react';
import { Code, ConnectError } from '@connectrpc/connect';
import { localizedApi } from '@/lib/api';
import { ApiError, isBadRequest, PageHeader, plural } from '@/components/browse';
import { humanizeId } from '@/lib/format';

// Shared by generateMetadata and the page body, which would otherwise each
// issue their own RPC for the same character.
const load = cache(async (id: string) => {
  try {
    const api = await localizedApi();
    const { character } = await api.getCharacter({ id });
    return character ?? null;
  } catch (err) {
    // Only a genuine not-found is swallowed; an outage must not be reported as
    // "this character does not exist".
    if (err instanceof ConnectError && err.code === Code.NotFound) return null;
    throw err;
  }
});

export async function generateMetadata({
  params,
}: {
  params: Promise<{ id: string }>;
}): Promise<Metadata> {
  const { id } = await params;
  try {
    const character = await load(id);
    if (character?.name) {
      return {
        title: character.name,
        description: `${character.name} — appearances and voice actors in anime-metadata-db.`,
      };
    }
  } catch {
    // Metadata must never take the page down.
  }
  return { title: humanizeId(id) };
}

export default async function CharacterPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;

  let character;
  try {
    character = await load(id);
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
  if (!character) notFound();

  return (
    <main className="mx-auto w-full max-w-3xl flex-1 px-6 py-12">
      <Link href="/search" className="text-sm text-fd-muted-foreground hover:underline">
        ← Search
      </Link>
      <div className="mt-4">
        <PageHeader
          title={character.name || humanizeId(character.id)}
          subtitle={[
            plural(character.appearances.length, 'appearance'),
            character.voiceActors.length
              ? plural(character.voiceActors.length, 'voice actor')
              : null,
          ]
            .filter(Boolean)
            .join(' · ')}
        />
      </div>

      {character.voiceActors.length > 0 ? (
        <section className="mt-10">
          <h2 className="mb-2 text-lg font-semibold">Voiced by</h2>
          <ul>
            {character.voiceActors.map((v) => (
              <li
                key={`${v.staffId}-${v.language}`}
                className="flex items-baseline justify-between gap-4 border-b border-fd-border py-3 last:border-0"
              >
                <Link href={`/staff/${v.staffId}`} className="font-medium hover:underline">
                  {v.staffName || v.staffId}
                </Link>
                <span className="text-sm text-fd-muted-foreground">{v.language}</span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {character.appearances.length > 0 ? (
        <section className="mt-10">
          <h2 className="mb-2 text-lg font-semibold">Appears in</h2>
          <ul>
            {character.appearances.map((a) => (
              <li
                key={a.seriesId}
                className="flex flex-wrap items-baseline justify-between gap-4 border-b border-fd-border py-3 last:border-0"
              >
                <Link href={`/browse/${a.seriesId}`} className="font-medium hover:underline">
                  {a.seriesTitle || a.seriesId}
                </Link>
                {/* An appearance may override the default cast for that series. */}
                {a.voiceActors.length > 0 ? (
                  <span className="text-sm text-fd-muted-foreground">
                    {a.voiceActors.map((v) => v.staffName || v.staffId).join(', ')}
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </main>
  );
}
