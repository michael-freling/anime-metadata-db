import type { Metadata } from 'next';
import Link from 'next/link';
import { ConnectError } from '@connectrpc/connect';
import { api } from '@/lib/api';
import { ApiError, PageHeader } from '@/components/browse';

export const metadata: Metadata = {
  title: 'Seasons',
  description: 'Anime by release year and quarter, counted from the committed dataset.',
};

const QUARTERS = [
  { slug: 'winter', label: 'Winter' },
  { slug: 'spring', label: 'Spring' },
  { slug: 'summer', label: 'Summer' },
  { slug: 'fall', label: 'Fall' },
] as const;

// Without this the year list is prerendered once and frozen at build time. The
// API is a separate deployment, so its data can change without this app being
// rebuilt, and a frozen index would quietly stop listing new years.
export const revalidate = 3600;

// The index is built from the works themselves rather than a hardcoded year
// range, so it grows with the dataset instead of going stale. One unpaginated
// call is fine while the catalogue is small; when it is not, this becomes a
// counts-by-year endpoint rather than a client-side tally.
export default async function SeasonsPage() {
  let years: number[];
  try {
    const { works } = await api.listWorks({ limit: 10_000 });
    years = [...new Set(works.filter((w) => w.releaseYear > 0).map((w) => w.releaseYear))].sort(
      (a, b) => b - a,
    );
  } catch (err) {
    return (
      <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-12">
        <PageHeader title="Seasons" />
        <ApiError detail={err instanceof ConnectError ? err.message : String(err)} />
      </main>
    );
  }

  return (
    <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-12">
      <PageHeader
        title="Seasons"
        subtitle="Anime by the quarter they premiered in. Films and specials carry a year without a quarter, so they are listed under the year itself."
      />

      <div className="space-y-6">
        {years.map((year) => (
          <div key={year} className="flex flex-wrap items-center gap-3">
            <span className="w-16 shrink-0 font-mono text-lg font-semibold">{year}</span>
            {QUARTERS.map((q) => (
              <Link
                key={q.slug}
                href={`/seasons/${year}/${q.slug}`}
                className="rounded-lg border border-fd-border px-3 py-1.5 text-sm transition-colors hover:bg-fd-accent"
              >
                {q.label}
              </Link>
            ))}
          </div>
        ))}
      </div>
    </main>
  );
}
