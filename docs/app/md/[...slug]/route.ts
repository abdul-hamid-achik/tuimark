import { notFound } from 'next/navigation';
import { blogSource, postMarkdown } from '@/lib/blog';
import { pageMarkdown, source } from '@/lib/source';
import { absoluteUrl } from '@/lib/site';

// The Markdown twin of every docs page and post. next.config.mjs rewrites
// /guide/layout.md (and /guide/layout with Accept: text/markdown) here, and
// /blog/SLUG.md to /md/blog/SLUG.
export const revalidate = false;
export const dynamicParams = false;

export async function GET(_req: Request, { params }: RouteContext<'/md/[...slug]'>) {
  const { slug } = await params;
  const post = slug[0] === 'blog' ? blogSource.getPage(slug.slice(1)) : undefined;
  const page = post ? undefined : source.getPage(slug);
  if (!post && !page) notFound();

  const url = post ? post.url : page!.url;
  return new Response(post ? await postMarkdown(post) : await pageMarkdown(page!), {
    headers: {
      'Content-Type': 'text/markdown; charset=utf-8',
      // The HTML page is the one to index.
      Link: `<${absoluteUrl(url)}>; rel="canonical"`,
    },
  });
}

export function generateStaticParams() {
  return [
    ...source.getPages().map((page) => ({ slug: page.slugs })),
    ...blogSource.getPages().map((post) => ({ slug: ['blog', ...post.slugs] })),
  ];
}
