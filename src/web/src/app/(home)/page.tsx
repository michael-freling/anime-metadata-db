import Link from 'next/link';
import { Code, Database, Terminal } from 'lucide-react';
import type { LucideIcon } from 'lucide-react';

const features: { title: string; description: string; href: string; icon: LucideIcon }[] = [
  {
    title: 'Using the API',
    description:
      'Query the hosted, read-only service with a plain HTTP POST + JSON — no client library or codegen.',
    href: '/docs/using-the-api',
    icon: Code,
  },
  {
    title: 'Using the dataset',
    description:
      'Read the committed YAML directly and learn the Franchise → Series → Season → Episode model.',
    href: '/docs/using-the-dataset',
    icon: Database,
  },
  {
    title: 'Building the dataset',
    description:
      'Compile the dataset and author your own entries with the deterministic builder CLI.',
    href: '/docs/building-the-dataset',
    icon: Terminal,
  },
];

export default function HomePage() {
  return (
    <main className="mx-auto w-full max-w-5xl flex-1 px-6 py-20">
      <span className="inline-flex items-center rounded-full border border-fd-border px-3 py-1 text-xs font-medium text-fd-muted-foreground">
        Experimental · open &amp; free
      </span>

      <h1 className="mt-8 max-w-3xl text-4xl font-bold tracking-tight sm:text-5xl">
        An{' '}
        <span className="bg-linear-to-r from-fd-primary to-purple-500 bg-clip-text text-transparent">
          open
        </span>{' '}
        dataset and API for anime
      </h1>

      <p className="mt-6 max-w-2xl text-lg text-fd-muted-foreground">
        Openly licensed, redistributable anime metadata — franchises, series, seasons and episodes —
        served over a free, read-only API. Early days: the dataset is small and growing.
      </p>

      <div className="mt-10 flex flex-wrap gap-4">
        <Link
          href="/docs/using-the-api"
          className="rounded-lg bg-fd-primary px-5 py-2.5 text-sm font-medium text-fd-primary-foreground transition-opacity hover:opacity-90"
        >
          Get started
        </Link>
        <Link
          href="https://github.com/michael-freling/anime-metadata-db"
          className="rounded-lg border border-fd-border px-5 py-2.5 text-sm font-medium transition-colors hover:bg-fd-accent"
        >
          View on GitHub →
        </Link>
      </div>

      <div className="mt-20 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {features.map(({ title, description, href, icon: Icon }) => (
          <Link
            key={href}
            href={href}
            className="rounded-xl border border-fd-border bg-fd-card p-5 transition-colors hover:bg-fd-accent"
          >
            <Icon className="size-5 text-fd-muted-foreground" />
            <h2 className="mt-3 font-semibold">{title}</h2>
            <p className="mt-2 text-sm text-fd-muted-foreground">{description}</p>
          </Link>
        ))}
      </div>
    </main>
  );
}
