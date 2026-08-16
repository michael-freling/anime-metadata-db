import { RootProvider } from 'fumadocs-ui/provider/next';
import './global.css';
import { Inter } from 'next/font/google';
import type { Metadata } from 'next';
import { appName, siteUrl } from '@/lib/shared';

const inter = Inter({
  subsets: ['latin'],
});

// metadataBase makes the relative OG image URLs emitted per page resolve to
// absolute ones; without it they point at localhost and no crawler can fetch
// them.
export const metadata: Metadata = {
  metadataBase: new URL(siteUrl),
  title: {
    default: `${appName} — an open dataset and API for anime`,
    template: `%s — ${appName}`,
  },
  description:
    'Openly licensed, redistributable anime metadata — franchises, series, seasons and episodes — served over a free, read-only API.',
};

export default function Layout({ children }: LayoutProps<'/'>) {
  return (
    <html lang="en" className={inter.className} suppressHydrationWarning>
      <body className="flex flex-col min-h-screen">
        <RootProvider>{children}</RootProvider>
      </body>
    </html>
  );
}
