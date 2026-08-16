import type { BaseLayoutProps } from 'fumadocs-ui/layouts/shared';
import { appName, gitConfig } from './shared';

export function baseOptions(): BaseLayoutProps {
  return {
    nav: {
      // JSX supported
      title: appName,
    },
    links: [
      { text: 'Search', url: '/search' },
      { text: 'Browse', url: '/browse' },
      { text: 'Seasons', url: '/seasons' },
      { text: 'Docs', url: '/docs' },
    ],
    githubUrl: `https://github.com/${gitConfig.user}/${gitConfig.repo}`,
  };
}
