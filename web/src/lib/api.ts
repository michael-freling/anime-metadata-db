import { cache } from 'react';
import { cookies } from 'next/headers';
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

// The languages the dataset can actually answer in. Titles carry an `en`
// translation and a native original (Japanese); asking for anything else would
// resolve back to one of these, so offering more would be a lie.
export const LANGUAGES = [
  { code: 'en', label: 'English' },
  { code: 'ja', label: '日本語' },
] as const;

export type LanguageCode = (typeof LANGUAGES)[number]['code'];

export const LANGUAGE_COOKIE = 'lang';

// Language is a viewer preference, not a view — like the theme toggle — so it
// lives in a cookie rather than the URL. Putting it in the URL would mean
// threading it through every link on every page, and would make two links to
// the same series look like different pages.
export const currentLanguage = cache(async (): Promise<LanguageCode> => {
  const store = await cookies();
  const value = store.get(LANGUAGE_COOKIE)?.value;
  return LANGUAGES.some((l) => l.code === value) ? (value as LanguageCode) : 'en';
});

// The API resolves every title and name from Accept-Language, so the client has
// to be built per request rather than once at module load.
//
// cache()d for the same reason as currentLanguage above: a detail page can ask
// for it more than once — /browse/[id] tries getSeries and then getFranchise —
// and without this each call builds a transport and a client of its own.
export const localizedApi = cache(async (): Promise<Client<typeof AnimeService>> => {
  const lang = await currentLanguage();
  return createClient(
    AnimeService,
    createConnectTransport({
      baseUrl: apiBaseUrl,
      interceptors: [
        (next) => async (req) => {
          req.header.set('Accept-Language', lang);
          return next(req);
        },
      ],
    }),
  );
});

// The earliest release year the dataset covers, used as the floor below which a
// year cannot be real data (see lib/format.ts). Wrapped in cache() so a page
// that needs it alongside its own query still makes one call per request.
//
// A failure here must not take a page down: the floor is a display refinement,
// and 0 simply disables the check, so the page still renders whatever years the
// API gave it.
export const datasetYearSpan = cache(async (): Promise<{ earliest: number; latest: number }> => {
  try {
    const { stats } = await api.getStats({});
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
