import type { Metadata } from 'next';
import { HOME_TITLE, SITE_NAME } from './site';

/** The blog's feed, announced on every page. */
export const RSS = { url: '/blog/rss.xml', title: `${SITE_NAME} blog` };

/**
 * The metadata of one page: its title, description, canonical URL, feed,
 * Open Graph and Twitter card. Next merges metadata shallowly (a page's
 * `alternates` or `openGraph` replaces the layout's whole object), so every
 * page builds all of it here instead of inheriting parts.
 */
export function pageMetadata({
  title,
  absoluteTitle = false,
  description,
  path,
  image,
  type = 'website',
  types = {},
  published,
  modified,
  authors,
}: {
  /** The page's own title; the layout's template adds " | Tuimark". */
  title: string;
  /** Use `title` as the whole title, with no template. */
  absoluteTitle?: boolean;
  description: string;
  /** The page's path, which is also its canonical URL. */
  path: string;
  /** A social card for the page; the site's default card (app/opengraph-image.tsx) otherwise. */
  image?: { url: string; alt: string };
  type?: 'website' | 'article';
  /** More alternate representations, by media type (the Markdown twin). */
  types?: Record<string, string>;
  published?: Date;
  modified?: Date;
  authors?: string[];
}): Metadata {
  const fullTitle = absoluteTitle ? title : `${title} | ${SITE_NAME}`;
  // Next adds the default card only to pages that set no openGraph at all,
  // so a page that sets one names its image itself.
  const card = image ?? { url: '/opengraph-image', alt: HOME_TITLE };
  const images = [{ url: card.url, width: 1200, height: 630, alt: card.alt }];
  return {
    title: absoluteTitle ? { absolute: title } : title,
    description,
    alternates: {
      canonical: path,
      types: { ...types, 'application/rss+xml': [RSS] },
    },
    openGraph: {
      type,
      siteName: SITE_NAME,
      locale: 'en_US',
      url: path,
      title: fullTitle,
      description,
      images,
      ...(type === 'article'
        ? {
            ...(published ? { publishedTime: published.toISOString() } : {}),
            ...(modified ? { modifiedTime: modified.toISOString() } : {}),
            ...(authors ? { authors } : {}),
          }
        : {}),
    },
    twitter: {
      card: 'summary_large_image',
      title: fullTitle,
      description,
      images: images.map((i) => ({ url: i.url, alt: i.alt })),
    },
  };
}
