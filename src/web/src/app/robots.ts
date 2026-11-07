import type { MetadataRoute } from 'next';
import { resolveSiteUrl } from '@/lib/shared';

// Resolved per request so a preview advertises itself, not production.
export const dynamic = 'force-dynamic';

export default function robots(): MetadataRoute.Robots {
  const siteUrl = resolveSiteUrl();
  return {
    rules: { userAgent: '*', allow: '/' },
    sitemap: new URL('/sitemap.xml', siteUrl).toString(),
  };
}
