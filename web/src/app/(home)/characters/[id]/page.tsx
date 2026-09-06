import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { cache } from 'react';
import { Code, ConnectError } from '@connectrpc/connect';
import { localizedBrowse } from '@/lib/api';
import { ApiError, isBadRequest, PageHeader, plural } from '@/components/browse';
import { humanizeId, languageLabel } from '@/lib/format';
import type { ScopeRef, VoiceActor } from '@/lib/gen/anime/v1/anime_pb';

// Shared by generateMetadata and the page body, which would otherwise each
// issue their own RPC for the same character.
const load = cache(async (id: string) => {
  try {
    const browse = await localizedBrowse();
    const { character } = await browse.getCharacter({ id });
    return character ?? null;
  } catch (err) {
    // Only a genuine not-found is swallowed; an outage must not be reported as
    // "this character does not exist".
    if (err instanceof ConnectError && err.code === Code.NotFound) return null;
    throw err;
  }
});

// A scope narrows an appearance to particular installments of a series — Saber
// is in every Fate/stay night adaptation, but Kate Higgins dubbed only the 2006
// one. Exactly one id is set per entry.
function scopeId(s: ScopeRef): string {
  return s.seasonId || s.movieId || s.specialId;
}

// The cast this appearance adds: everyone in its resolved list who is not
// already there because they voice the character throughout.
function addedCast(a: { voiceActors: VoiceActor[] }): VoiceActor[] {
  return a.voiceActors.filter((v) => !v.throughout);
}

// The API resolves each installment's own title, which a numbered season
// usually lacks — hence the number as a fallback. An entry the index could not
// resolve is dropped rather than printed as an id.
function scopeLabel(a: { scope: ScopeRef[] }): string {
  return a.scope
    .map((s) => s.title || (s.number ? `Season ${s.number}` : ''))
    .filter(Boolean)
    .join(', ');
}

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

  // One call. GetCharacter embeds every appearance, each with its cast already
  // resolved, so there is no second request and no pager to thread.
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

  const appearances = character.appearances;

  return (
    <main className="mx-auto w-full max-w-3xl flex-1 px-6 py-12">
      <Link href="/browse" className="text-sm text-fd-muted-foreground hover:underline">
        ← Browse
      </Link>
      <div className="mt-4">
        <PageHeader
          title={character.name || humanizeId(character.id)}
          // No voice-actor count here: cast that varies by series lives on the
          // appearances, so a single total would conflate the two.
          subtitle={plural(appearances.length, 'appearance')}
        />
      </div>

      {character.voiceActors.length > 0 ? (
        <section className="mt-10">
          {/* The cast that holds throughout. Anyone cast for a single series
              is listed under that series below, not here. */}
          <h2 className="mb-2 text-lg font-semibold">Voiced by</h2>
          <ul>
            {character.voiceActors.map((v) => (
              <li
                key={`${v.staffId}-${v.language}`}
                className="flex items-baseline justify-between gap-4 border-b border-fd-border py-3 last:border-0"
              >
                <Link href={`/staff/${v.staffId}`} className="font-medium hover:underline">
                  {v.staffName || humanizeId(v.staffId)}
                </Link>
                <span className="text-sm text-fd-muted-foreground">{languageLabel(v.language)}</span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {appearances.length > 0 ? (
        <section className="mt-10">
          <h2 className="mb-2 text-lg font-semibold">Appears in</h2>
          <ul>
            {appearances.map((a) => (
              <li
                // A character can hold more than one appearance in a series
                // when the cast changes between its installments, so the series
                // id alone is not a key.
                key={`${a.seriesId}:${a.scope.map(scopeId).join('+')}`}
                className="flex flex-wrap items-baseline justify-between gap-4 border-b border-fd-border py-3 last:border-0"
              >
                <div>
                  <Link href={`/browse/${a.seriesId}`} className="font-medium hover:underline">
                    {a.seriesTitle || humanizeId(a.seriesId)}
                  </Link>
                  {/* Which installments this appearance covers. Only a series
                      whose cast changed part-way through has one, and without
                      it two such rows would read as duplicates. */}
                  {scopeLabel(a) ? (
                    <p className="text-sm text-fd-muted-foreground">{scopeLabel(a)}</p>
                  ) : null}
                </div>
                {/* Only who is specific to this appearance. The API sends the
                    resolved cast — the actors above plus these — but repeating
                    "Ayako Kawasumi" on every row says nothing; the rows exist
                    to show where the cast differs. */}
                {addedCast(a).length > 0 ? (
                  <span className="text-sm text-fd-muted-foreground">
                    {character.voiceActors.length > 0 ? 'also ' : null}
                    {addedCast(a)
                      .map((v) => v.staffName || humanizeId(v.staffId))
                      .join(', ')}
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
