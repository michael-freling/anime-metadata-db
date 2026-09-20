import { permanentRedirect } from 'next/navigation';

// The seasonal chart is now /browse with the year and quarter filters set.
// Redirecting rather than deleting keeps every published link working, and
// carries the reader to the same result set.
export default async function SeasonalChart({
  params,
}: {
  params: Promise<{ year: string; season: string }>;
}): Promise<never> {
  const { year, season } = await params;
  permanentRedirect(`/browse?year=${encodeURIComponent(year)}&quarter=${encodeURIComponent(season)}`);
}
