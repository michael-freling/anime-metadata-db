import type { Metadata } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { cache } from 'react';
import { Code, ConnectError } from '@connectrpc/connect';
import { localizedApi } from '@/lib/api';
import { ApiError, isBadRequest, PageHeader, Pager, plural } from '@/components/browse';
import { humanizeId, languageLabel } from '@/lib/format';

const load = cache(async (id: string) => {
  try {
    const api = await localizedApi();
    const res = await api.getStaff({ id });
    return res.staff ? res : null;
  } catch (err) {
    if (err instanceof ConnectError && err.code === Code.NotFound) return null;
    throw err;
  }
});

// How many roles one page shows.
const ROLE_LIMIT = 24;

// A working voice actor accumulates roles for a career, so the credits GetStaff
// embeds are a capped page. Asking ListCredits is what makes the rest
// reachable, and the count honest.
const loadCredits = cache(async (id: string, token: string) => {
  const api = await localizedApi();
  return api.listCredits({ staffId: id, pageToken: token, limit: ROLE_LIMIT });
});

export async function generateMetadata({
  params,
}: {
  params: Promise<{ id: string }>;
}): Promise<Metadata> {
  const { id } = await params;
  try {
    const res = await load(id);
    if (res?.staff?.name) {
      return {
        title: res.staff.name,
        description: `${res.staff.name} — roles in anime-metadata-db.`,
      };
    }
  } catch {
    // Metadata must never take the page down.
  }
  return { title: humanizeId(id) };
}

export default async function StaffPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ token?: string }>;
}) {
  const { id } = await params;
  const { token = '' } = await searchParams;

  let res;
  let page;
  try {
    res = await load(id);
    if (res?.staff) page = await loadCredits(id, token);
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
  if (!res?.staff || !page) notFound();

  const { staff } = res;
  const credits = page.credits;

  return (
    <main className="mx-auto w-full max-w-3xl flex-1 px-6 py-12">
      <Link href="/browse" className="text-sm text-fd-muted-foreground hover:underline">
        ← Browse
      </Link>
      <div className="mt-4">
        <PageHeader
          title={staff.name || humanizeId(staff.id)}
          subtitle={plural(page.totalSize, 'role')}
        />
      </div>

      {page.totalSize > 0 ? (
        <section className="mt-10">
          <h2 className="mb-2 text-lg font-semibold">Roles</h2>
          <ul>
            {credits.map((c) => (
              <li
                key={`${c.characterId}-${c.language}`}
                className="flex flex-wrap items-baseline justify-between gap-4 border-b border-fd-border py-3 last:border-0"
              >
                <Link href={`/characters/${c.characterId}`} className="font-medium hover:underline">
                  {c.characterName || humanizeId(c.characterId)}
                </Link>
                <span className="text-sm text-fd-muted-foreground">
                  {[
                    languageLabel(c.language),
                    // Titles come denormalized on the credit; the id is only a
                    // fallback for a series the dataset cannot name.
                    ...c.seriesIds.map((id, i) => c.seriesTitles[i] || humanizeId(id)),
                  ].join(' · ')}
                </span>
              </li>
            ))}
          </ul>
          <Pager
            basePath={`/staff/${staff.id}`}
            nextToken={page.nextPageToken}
            shown={credits.length}
            total={page.totalSize}
          />
        </section>
      ) : (
        <p className="mt-10 text-fd-muted-foreground">
          The dataset records no roles for this person yet.
        </p>
      )}
    </main>
  );
}
