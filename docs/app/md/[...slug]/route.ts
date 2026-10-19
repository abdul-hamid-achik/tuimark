import { notFound } from 'next/navigation';
import { pageMarkdown, source } from '@/lib/source';
import { absoluteUrl } from '@/lib/site';

// The Markdown twin of every docs page. next.config.mjs rewrites
// /guide/layout.md (and /guide/layout with Accept: text/markdown) here.
export const revalidate = false;
export const dynamicParams = false;

export async function GET(_req: Request, { params }: RouteContext<'/md/[...slug]'>) {
  const { slug } = await params;
  const page = source.getPage(slug);
  if (!page) notFound();

  return new Response(await pageMarkdown(page), {
    headers: {
      'Content-Type': 'text/markdown; charset=utf-8',
      // The HTML page is the one to index.
      Link: `<${absoluteUrl(page.url)}>; rel="canonical"`,
    },
  });
}

export function generateStaticParams() {
  return source.getPages().map((page) => ({ slug: page.slugs }));
}
