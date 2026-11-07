import type { MetadataRoute } from 'next';
import { source } from '@/lib/source';
import { resolveSiteUrl } from '@/lib/shared';

// Built from the same filtered source the pages are, so internal pages under
// development/ can never reach the sitemap: there is nothing here to keep in
// sync by hand.
// Resolved per request so a preview advertises itself, not production.
export const dynamic = 'force-dynamic';

export default function sitemap(): MetadataRoute.Sitemap {
  const siteUrl = resolveSiteUrl();
  return [
    { url: siteUrl, changeFrequency: 'weekly', priority: 1 },
    ...source.getPages().map((page) => ({
      url: new URL(page.url, siteUrl).toString(),
      changeFrequency: 'weekly' as const,
      priority: 0.8,
    })),
  ];
}
