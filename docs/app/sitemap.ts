import type { MetadataRoute } from 'next';
import { posts } from '@/lib/blog';
import { gitLastModified } from '@/lib/git';
import { source } from '@/lib/source';
import { absoluteUrl } from '@/lib/site';

export const revalidate = false;

const lastModified = (date: Date | undefined) => (date ? { lastModified: date } : {});

export default function sitemap(): MetadataRoute.Sitemap {
  // Every date comes from git: the last commit that touched what the page
  // is built from (fumadocs-mdx records it for the MDX pages and posts).
  const docs = source.getPages().map((page) => ({
    url: absoluteUrl(page.url),
    ...lastModified(page.data.lastModified ? new Date(page.data.lastModified) : undefined),
    changeFrequency: 'weekly' as const,
    priority: page.url === '/guide/getting-started' ? 0.9 : page.url.startsWith('/reference') ? 0.6 : 0.7,
  }));
  const blog = posts().map((post) => ({
    url: absoluteUrl(post.url),
    ...lastModified(post.data.lastModified ? new Date(post.data.lastModified) : post.data.date),
    changeFrequency: 'monthly' as const,
    priority: 0.6,
  }));
  return [
    {
      url: absoluteUrl('/'),
      ...lastModified(gitLastModified('app/(home)/page.tsx', 'app/(home)/landing.css', 'snippets/hero', 'lib/site.ts')),
      changeFrequency: 'weekly',
      priority: 1,
    },
    {
      url: absoluteUrl('/kit'),
      ...lastModified(gitLastModified('app/(home)/kit', 'lib/generated/kit.json')),
      changeFrequency: 'weekly',
      priority: 0.8,
    },
    {
      url: absoluteUrl('/blog'),
      ...lastModified(gitLastModified('app/(home)/blog', 'blog')),
      changeFrequency: 'weekly',
      priority: 0.6,
    },
    ...docs,
    ...blog,
  ];
}
