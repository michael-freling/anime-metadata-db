import { generateFiles } from 'fumadocs-openapi';
import { openapi } from '../src/lib/openapi.js';
import { operationName, slugsFor } from '../src/lib/openapi-slug.js';
import spec from '../openapi/anime/v1/anime.openapi.json' with { type: 'json' };

// Writes the API reference pages from the OpenAPI spec, which is itself
// generated from api/proto/anime/v1/anime.proto.
//
// The output is committed and checked in CI, the same way the Go and TypeScript
// clients are: a proto change that is not reflected here fails the build rather
// than leaving the docs describing an API that no longer exists.
//
// One page per operation, so each method gets its own URL, its own search
// entry and its own playground, instead of one page a reader has to scroll and
// cannot link into.
//
// The naming lives in src/lib/openapi-slug.ts so it can be tested; this file is
// the wiring around it.

const OUT = './content/docs/api';

// Every RPC the schema declares, in the order it declares them — which is an
// editorial order (the browse entry points first, the leaf lookups after) and
// worth preserving. Sorting the sidebar alphabetically instead would open on
// ListAppearances, which is nobody's starting point.
//
// slugsFor refuses a schema in which two RPCs would land on the same page, so a
// collision stops the build here rather than silently dropping a method from
// the docs.
const declared = Object.keys(spec.paths).map(operationName);
const slugs = slugsFor(declared);

// Paths handed to beforeWrite are relative to `output`, not to the working
// directory, so the rewrites below are bare filenames.
await generateFiles({
  input: openapi,
  output: OUT,
  per: 'operation',
  beforeWrite(files) {
    for (const file of files) {
      const name = operationName(file.path);
      const slug = slugs.get(name);
      // The generator produced a page for an operation the spec does not
      // declare, which should be impossible — both read the same document. If
      // it ever happens, an undefined slug would write a file called
      // "undefined.mdx" and overwrite the last one to hit it.
      if (slug === undefined) {
        throw new Error(`generated a page for ${name}, which is not in the spec's paths`);
      }
      file.path = `${slug}.mdx`;
    }
    // Generated rather than authored, so adding an RPC does not also mean
    // remembering to list it. `...` lets any page not named here still appear,
    // so a new method is never silently missing from the sidebar.
    files.push({
      path: 'meta.json',
      content:
        JSON.stringify(
          {
            title: 'API reference',
            description: 'Every method, generated from the schema',
            pages: [...declared.map((name) => slugs.get(name)!), '...'],
          },
          null,
          2,
        ) + '\n',
    });
  },
});
