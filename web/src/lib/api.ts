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
