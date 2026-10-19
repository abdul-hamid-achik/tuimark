import { notFound } from 'next/navigation';
import { ogImage } from '@/lib/og';
import { ogImageSegments, sectionOf, source } from '@/lib/source';

// One social card per docs page: /og/guide/layout/image.png.
export const revalidate = false;
export const dynamicParams = false;

export async function GET(_req: Request, { params }: RouteContext<'/og/[...slug]'>) {
  const { slug } = await params;
  const page = source.getPage(slug.slice(0, -1));
  if (!page) notFound();

  return ogImage({
    eyebrow: sectionOf(page).name.toLowerCase(),
    title: page.data.title,
    description: page.data.description,
  });
}

export function generateStaticParams() {
  return source.getPages().map((page) => ({ slug: ogImageSegments(page) }));
}
