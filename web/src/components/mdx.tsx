import defaultMdxComponents from 'fumadocs-ui/mdx';
import type { MDXComponents } from 'mdx/types';
import { OpenAPIPage } from '@/lib/openapi-page';

export function getMDXComponents(components?: MDXComponents) {
  return {
    ...defaultMdxComponents,
    // The generated API reference pages render themselves through this: each
    // one is a thin MDX wrapper that names an operation and defers to the
    // component for the schema tables and the request playground.
    OpenAPIPage,
    // v10 of the generator emitted <APIPage>; the pages accept either name, so
    // both are provided and a regeneration cannot break on the rename.
    APIPage: OpenAPIPage,
    ...components,
  } satisfies MDXComponents;
}

export const useMDXComponents = getMDXComponents;

declare global {
  type MDXProvidedComponents = ReturnType<typeof getMDXComponents>;
}
