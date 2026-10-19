import type { MetadataRoute } from 'next';
import { source } from '@/lib/source';
import { absoluteUrl } from '@/lib/site';

export const revalidate = false;

export default function sitemap(): MetadataRoute.Sitemap {
  const pages = source.getPages().map((page) => ({
    url: absoluteUrl(page.url),
    ...(page.data.lastModified ? { lastModified: new Date(page.data.lastModified) } : {}),
    changeFrequency: 'weekly' as const,
    priority: page.url === '/guide/getting-started' ? 0.9 : page.url.startsWith('/reference') ? 0.6 : 0.7,
  }));
  return [{ url: absoluteUrl('/'), changeFrequency: 'weekly', priority: 1 }, ...pages];
}
