import { cache } from 'react';
import { createClient, type Client } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-web';
import { AnimeService } from './gen/anime/v1/anime_pb';
import { apiBaseUrl } from './shared';

// The Connect client for the read-only dataset API.
//
// This is imported by server components only. Fetching on the server rather
// than in the browser is what lets the API stay exactly as it is: the request
// is same-origin from Node's point of view, so no CORS headers are needed on
// the Go service, and series pages render server-side and stay indexable.
//
// The types come from the same proto the Go server is built from (see
// buf.gen.yaml), so a field renamed there fails this build rather than
// surfacing as undefined at runtime.
export const api: Client<typeof AnimeService> = createClient(
  AnimeService,
  createConnectTransport({ baseUrl: apiBaseUrl }),
);

// The earliest release year the dataset covers, used as the floor below which a
// year cannot be real data (see lib/format.ts). Wrapped in cache() so a page
// that needs it alongside its own query still makes one call per request.
//
// A failure here must not take a page down: the floor is a display refinement,
// and 0 simply disables the check, so the page still renders whatever years the
// API gave it.
export const datasetYearSpan = cache(async (): Promise<{ earliest: number; latest: number }> => {
  try {
    const { stats } = await api.getHealth({});
    return {
      earliest: stats?.earliestReleaseYear ?? 0,
      latest: stats?.latestReleaseYear ?? 0,
    };
  } catch {
    // 0/0 disables both the label floor and the route's range check, so an
    // outage degrades to "no refinement" rather than 404ing valid URLs.
    return { earliest: 0, latest: 0 };
  }
});

// The earliest year the dataset covers, as the floor for year labels.
export const earliestReleaseYear = async (): Promise<number> =>
  (await datasetYearSpan()).earliest;
