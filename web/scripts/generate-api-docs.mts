import { generateFiles } from 'fumadocs-openapi';
import { openapi } from '../src/lib/openapi.js';
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

const OUT = './content/docs/api';

// Paths handed to beforeWrite are relative to `output`, not to the working
// directory, so the rewrites below are bare filenames.

// operationName reduces "anime.v1.AnimeService.ListFranchises" to
// "ListFranchises". The generator names files after the fully-qualified
// operation id, which is unambiguous and unreadable in a URL.
function operationName(filePath: string): string {
  const base = filePath.split('/').pop()!.replace(/\.mdx$/, '');
  return base.split('.').pop()!;
}

// kebab turns ListFranchises into list-franchises, matching every other docs
// URL on the site. The page title stays PascalCase, because that is what the
// method is actually called.
function kebab(name: string): string {
  return name.replace(/([a-z0-9])([A-Z])/g, '$1-$2').toLowerCase();
}

// The order the proto declares its RPCs in, which is an editorial order — the
// browse entry points first, the leaf lookups after — and worth preserving.
// Sorting the sidebar alphabetically instead would put ListAppearances first,
// which is nobody's starting point.
const declaredOrder = Object.keys(spec.paths).map((p) => kebab(p.split('/').pop()!));

await generateFiles({
  input: openapi,
  output: OUT,
  per: 'operation',
  beforeWrite(files) {
    for (const file of files) {
      file.path = `${kebab(operationName(file.path))}.mdx`;
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
            pages: [...declaredOrder, '...'],
          },
          null,
          2,
        ) + '\n',
    });
  },
});
