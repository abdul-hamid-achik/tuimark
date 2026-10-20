import { docs } from '@/.source/server';
import { loader } from 'fumadocs-core/source';
import { absoluteUrl } from './site';

// Pages live at their path under content/: /guide/…, /reference/…, and
// /examples. The landing page (/) is a route of its own.
export const source = loader({
  baseUrl: '/',
  source: docs.toFumadocsSource(),
});

export type DocPage = NonNullable<ReturnType<typeof source.getPage>>;

/** The URL of a page's Markdown twin: /guide/layout → /guide/layout.md. */
export function markdownUrl(page: DocPage): string {
  return `${page.url}.md`;
}

/** The route segments of a page's social card: /og/guide/layout/image.png. */
export function ogImageSegments(page: DocPage): string[] {
  return [...page.slugs, 'image.png'];
}

export function ogImageUrl(page: DocPage): string {
  return `/og/${ogImageSegments(page).join('/')}`;
}

/** The section a page belongs to, for breadcrumbs and social cards. */
export function sectionOf(page: DocPage): { name: string; url: string } {
  switch (page.slugs[0]) {
    case 'reference':
      return { name: 'Reference', url: '/reference' };
    case 'examples':
      return { name: 'Examples', url: '/examples' };
    default:
      return { name: 'Guide', url: '/guide/getting-started' };
  }
}

/**
 * A page as Markdown: its title, its canonical URL, and its body as the
 * MDX processor left it (snippets inlined, components as JSX).
 */
export async function pageMarkdown(page: DocPage): Promise<string> {
  const body = await page.data.getText('processed');
  const lines = [`# ${page.data.title}`, '', `URL: ${absoluteUrl(page.url)}`];
  if (page.data.description) lines.push('', `> ${page.data.description}`);
  lines.push('', body.trim(), '');
  return lines.join('\n');
}
