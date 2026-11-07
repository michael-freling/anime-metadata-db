import type { OpenAPIPageProps_Preloaded } from 'fumadocs-openapi/ui';
import { RenderedOpenAPIPage } from './openapi-page-client';
import { openapi } from './openapi';

// OpenAPIPage renders one generated reference page: the request and response
// schemas, the code samples, and the playground that sends a real request.
//
// A server component wrapping the library's client one. The generated MDX names
// a document and an operation but carries no schema, so the schema is loaded
// here — on the server, at build time — and handed across as a prop. Letting
// the browser fetch the spec instead would ship the whole 1,900-line document
// to every reader of every page, nearly all of it describing methods they are
// not looking at.
export async function OpenAPIPage(props: Omit<OpenAPIPageProps_Preloaded, 'preloaded'>) {
  const schemas = await openapi.getSchemas();
  const docs = Object.fromEntries(
    Object.entries(schemas).map(([name, loaded]) => [name, loaded.bundled]),
  );
  return <RenderedOpenAPIPage {...props} preloaded={{ docs }} />;
}
