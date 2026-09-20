'use client';

import { createOpenAPIPage } from 'fumadocs-openapi/ui';
import { createCodeUsageGeneratorRegistry } from 'fumadocs-openapi/requests/generators';
import { curl } from 'fumadocs-openapi/requests/generators/curl';
import { javascript } from 'fumadocs-openapi/requests/generators/javascript';
import { go } from 'fumadocs-openapi/requests/generators/go';
import { python } from 'fumadocs-openapi/requests/generators/python';

// The playground is interactive, so the page it lives on is a client component
// and the factory has to be called in a client module — this file exists for
// that boundary alone. The schema it renders is loaded on the server and passed
// in as a prop; see openapi-page.tsx.

// The code samples shown beside each method.
//
// Four languages, not the full set the library offers. Every extra tab is one
// more thing a reader scans past, and a sample in a language nobody calls this
// from is a maintenance claim without a maintainer. curl is first on purpose:
// the whole point of Connect over HTTP/JSON is that no client library is
// needed, and a reader who copies that one line has the entire contract.
const codeUsages = createCodeUsageGeneratorRegistry();
codeUsages.add('curl', curl);
codeUsages.add('javascript', javascript);
codeUsages.add('go', go);
codeUsages.add('python', python);

export const RenderedOpenAPIPage = createOpenAPIPage({ codeUsages });
