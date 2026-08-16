import Link from 'next/link';
import { Code, ConnectError } from '@connectrpc/connect';
import { plural, yearsLabel } from '@/lib/format';
import type { ReactNode } from 'react';

// Shared presentation for the browse routes. Kept apart from the pages so the
// catalogue and the seasonal chart cannot drift into looking like two products.

export function PageHeader({ title, subtitle }: { title: string; subtitle?: ReactNode }) {
  return (
    <header className="mb-8">
      <h1 className="text-3xl font-bold tracking-tight">{title}</h1>
      {subtitle ? <p className="mt-2 text-fd-muted-foreground">{subtitle}</p> : null}
    </header>
  );
}

export function Grid({ children }: { children: ReactNode }) {
  return <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">{children}</ul>;
}

export function Card({
  href,
  title,
  meta,
  badge,
}: {
  href: string;
  title: string;
  meta?: ReactNode;
  badge?: string;
}) {
  return (
    <li>
      <Link
        href={href}
        className="flex h-full flex-col rounded-xl border border-fd-border bg-fd-card p-4 transition-colors hover:bg-fd-accent"
      >
        <div className="flex items-start justify-between gap-3">
          <span className="font-medium leading-snug">{title}</span>
          {badge ? (
            <span className="shrink-0 rounded-full border border-fd-border px-2 py-0.5 text-[11px] uppercase tracking-wide text-fd-muted-foreground">
              {badge}
            </span>
          ) : null}
        </div>
        {meta ? <span className="mt-2 text-sm text-fd-muted-foreground">{meta}</span> : null}
      </Link>
    </li>
  );
}

// Pager renders cursor navigation. Page tokens are opaque and single-direction
// by design, so there is no "previous" — offering one would mean either
// decoding the token (which the API deliberately does not promise) or
// accumulating a token stack in the URL. "Start over" is the honest option.
export function Pager({
  basePath,
  nextToken,
  shown,
  total,
  query = {},
}: {
  basePath: string;
  nextToken: string;
  shown: number;
  total: number;
  query?: Record<string, string>;
}) {
  const withToken = (token: string) => {
    const params = new URLSearchParams(query);
    if (token) params.set('token', token);
    const qs = params.toString();
    return qs ? `${basePath}?${qs}` : basePath;
  };

  return (
    <nav className="mt-8 flex items-center justify-between gap-4 border-t border-fd-border pt-5">
      <p className="text-sm text-fd-muted-foreground">
        Showing {shown} of {total.toLocaleString()}
      </p>
      <div className="flex gap-2">
        <Link
          href={withToken('')}
          className="rounded-lg border border-fd-border px-3 py-1.5 text-sm transition-colors hover:bg-fd-accent"
        >
          Start over
        </Link>
        {nextToken ? (
          <Link
            href={withToken(nextToken)}
            className="rounded-lg bg-fd-primary px-3 py-1.5 text-sm text-fd-primary-foreground transition-opacity hover:opacity-90"
          >
            Next →
          </Link>
        ) : null}
      </div>
    </nav>
  );
}

// ApiError is what a browse page renders when a dataset query fails. The docs
// half of the site is static and unaffected, so the failure is deliberately
// scoped to the page rather than thrown into the root layout.
//
// A rejected request is NOT an outage and must not be reported as one: a stale
// or tampered page token comes back as InvalidArgument, and telling the reader
// the service is down when it is up and simply refused the link sends them to
// check the wrong thing.
export function ApiError({ detail, badRequest = false }: { detail: string; badRequest?: boolean }) {
  return (
    <div className="rounded-xl border border-fd-border bg-fd-card p-6">
      <h2 className="font-semibold">
        {badRequest ? 'That link is no longer valid' : 'The dataset API is unreachable'}
      </h2>
      <p className="mt-2 text-sm text-fd-muted-foreground">
        {badRequest ? (
          <>
            The page it points at could not be read. <Link href="/browse" className="underline">Start
            over from the beginning of the catalogue</Link>.
          </>
        ) : (
          'Browsing needs the read-only Connect service. The documentation is unaffected.'
        )}
      </p>
      <p className="mt-3 font-mono text-xs text-fd-muted-foreground">{detail}</p>
    </div>
  );
}

// isBadRequest reports whether a failure was the request's fault rather than
// the service being down.
export function isBadRequest(err: unknown): boolean {
  return err instanceof ConnectError && err.code === Code.InvalidArgument;
}

// Re-exported so the browse pages import their display helpers from one place.
export { plural, yearsLabel };
