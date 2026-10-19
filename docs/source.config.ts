import { defineConfig, defineDocs } from 'fumadocs-mdx/config';
import lastModified from 'fumadocs-mdx/plugins/last-modified';
import { metaSchema, pageSchema } from 'fumadocs-core/source/schema';
import type { LanguageRegistration } from 'shiki';
import { z } from 'zod';

// The whole site's content: content/{guide,reference,examples}. Each top
// folder is a sidebar of its own (`"root": true` in its meta.json), and
// the URL of a page is its path under content/, so /guide/layout is
// content/guide/layout.mdx, exactly as on the old site.
export const docs = defineDocs({
  dir: 'content',
  docs: {
    schema: pageSchema.extend({
      // Generated pages (docs/scripts/generate.mjs) have no "Edit this page" link.
      editLink: z.boolean().optional(),
    }),
    postprocess: {
      // The Markdown of each page, for /llms-full.txt and the .md routes.
      includeProcessedMarkdown: true,
    },
  },
  meta: {
    schema: metaSchema,
  },
});

// A frame the runtime painted is ```grid (plain text, drawn with touching
// rows), and a command with its output is ```term (the prompt line, the
// dump's section headers and the diagnostic codes are highlighted, the
// output is left alone). ```tui is XML and ```tcss is CSS.
const grid: LanguageRegistration = {
  name: 'grid',
  scopeName: 'text.tuimark.grid',
  patterns: [],
  repository: {},
};

const term: LanguageRegistration = {
  name: 'term',
  scopeName: 'text.tuimark.term',
  patterns: [
    {
      match: String.raw`^(\$) (\S+)(.*)$`,
      captures: {
        1: { name: 'comment' },
        2: { name: 'entity.name.function' },
        3: { name: 'variable.other' },
      },
    },
    { match: String.raw`^=== .* ===$`, name: 'comment' },
    {
      match: String.raw`^(error|warning) ([VLB]\d{3})`,
      captures: { 1: { name: 'keyword' }, 2: { name: 'keyword' } },
    },
    { match: String.raw`^ok$`, name: 'markup.inserted' },
  ],
  repository: {},
};

export default defineConfig({
  plugins: [lastModified()],
  mdxOptions: {
    rehypeCodeOptions: {
      themes: { light: 'github-light', dark: 'github-dark' },
      // xml and css are preloaded: an alias is only followed to a loaded language.
      langs: [grid, term, 'xml', 'css'],
      langAlias: { tui: 'xml', tcss: 'css' },
    },
  },
});
