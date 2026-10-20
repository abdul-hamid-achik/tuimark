import type { MetadataRoute } from 'next';
import { absoluteUrl } from '@/lib/site';

export default function robots(): MetadataRoute.Robots {
  return {
    // /md/… is the internal route behind the pages' .md URLs, which stay allowed.
    rules: [{ userAgent: '*', allow: '/', disallow: ['/api/', '/md/'] }],
    sitemap: absoluteUrl('/sitemap.xml'),
    host: absoluteUrl('/'),
  };
}
