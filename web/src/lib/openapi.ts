import { createOpenAPI } from 'fumadocs-openapi/server';
import { apiBaseUrl, appName } from './shared';
import spec from '../../openapi/anime/v1/anime.openapi.json';
import type { OpenAPIV3_2 } from 'fumadocs-openapi';

// The generator emits OpenAPI 3.1; the library types the loaded document as
// 3.2, which is a superset, so this is the type the input callback must return.
type Document = OpenAPIV3_2.Document;

// The API reference, loaded from the spec `buf generate` produces out of
// api/proto/anime/v1/anime.proto.
//
// Nothing here is authored: the method list, every field, every type and every
// description come from the proto, which is the same schema the Go server and
// this app's own client are built from. A method added to the service appears
// here without anyone remembering to write it down, and a renamed field cannot
// be described wrongly, because there is no second description to update.
//
// Two things the generator cannot know are supplied below.

// describe fills in the parts of `info` that belong to the deployment rather
// than to the schema. The generator sets `title` to the protobuf package name
// ("anime.v1"), which is accurate and useless as a page heading.
//
// The version is deliberately absent rather than invented. OpenAPI wants
// `info.version` and this service has no API version to give: it is v1 by
// package name, unversioned by deployment, and stamping a number here would be
// a fact this repository does not have.
function describe(document: Document): Document {
  return {
    ...document,
    info: {
      ...document.info,
      title: `${appName} API`,
      description:
        'Read-only Connect RPC over the committed dataset. Every method is an ' +
        'HTTP POST with a JSON body — the requests below are real, and run ' +
        'against the deployed service.',
      version: document.info?.version ?? '',
    },
    // Without a server the reference renders but cannot send anything, and the
    // interactive half is the reason it exists. The URL comes from the same
    // constant the app's own client uses, so the playground and the site can
    // never point at different deployments; `next dev` therefore aims it at a
    // local `make api` automatically, exactly like every other request here.
    //
    // This is read at build time, which is what makes it safe to use a value
    // with no NEXT_PUBLIC prefix: the resolved string is baked into the
    // rendered page, and process.env is never reached from the browser.
    servers: [{ url: apiBaseUrl, description: 'Deployed API' }],
  };
}

export const openapi = createOpenAPI({
  input: { 'anime.v1': () => describe(spec as Document) },
});
