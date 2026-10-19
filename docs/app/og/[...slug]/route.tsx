import { notFound } from 'next/navigation';
import { blogSource, formatDate } from '@/lib/blog';
import { KIT_PAGE } from '@/lib/kit';
import { ogImage } from '@/lib/og';
import { ogImageSegments, sectionOf, source } from '@/lib/source';

// One social card per page: /og/guide/layout/image.png for a docs page,
// /og/blog/SLUG/image.png for a post, and /og/blog and /og/kit for those
// two pages.
export const revalidate = false;
export const dynamicParams = false;

function card(path: string[]) {
  const [head, ...rest] = path;
  if (head === 'kit' && rest.length === 0) {
    return { eyebrow: 'the kit', title: KIT_PAGE.title, description: KIT_PAGE.description };
  }
  if (head === 'blog') {
    if (rest.length === 0) {
      return { eyebrow: 'blog', title: 'Notes from the cell grid', description: 'Releases, design notes, and how Tuimark is built.' };
    }
    const post = blogSource.getPage(rest);
    if (post) {
      return { eyebrow: `blog · ${formatDate(post.data.date).toLowerCase()}`, title: post.data.title, description: post.data.description };
    }
  }
  const page = source.getPage(path);
  if (!page) return undefined;
  return { eyebrow: sectionOf(page).name.toLowerCase(), title: page.data.title, description: page.data.description };
}

export async function GET(_req: Request, { params }: RouteContext<'/og/[...slug]'>) {
  const { slug } = await params;
  const c = card(slug.slice(0, -1));
  if (!c) notFound();
  return ogImage(c);
}

export function generateStaticParams() {
  return [
    ...source.getPages().map((page) => ({ slug: ogImageSegments(page) })),
    ...blogSource.getPages().map((post) => ({ slug: ['blog', ...post.slugs, 'image.png'] })),
    { slug: ['blog', 'image.png'] },
    { slug: ['kit', 'image.png'] },
  ];
}
