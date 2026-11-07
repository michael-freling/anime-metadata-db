import { permanentRedirect } from 'next/navigation';

// Search is not a separate destination: it is the query box on /browse.
export default async function SearchRedirect({
  searchParams,
}: {
  searchParams: Promise<{ q?: string }>;
}): Promise<never> {
  const { q } = await searchParams;
  permanentRedirect(q ? `/browse?q=${encodeURIComponent(q)}` : '/browse');
}
