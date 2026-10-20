'use client';

import type { ImgHTMLAttributes } from 'react';

/**
 * A page's largest image, fetched first. It is a client component on
 * purpose: React preloads every eager <img> a server component renders, and
 * that preload travels in the page's RSC payload, so every page that
 * prefetches this one (each docs page links home) would fetch the image too.
 * Rendered here, the preload is only in this page's own HTML.
 */
export function EagerImg({ alt, ...props }: ImgHTMLAttributes<HTMLImageElement>) {
  // eslint-disable-next-line @next/next/no-img-element -- SVGs: nothing for next/image to optimize
  return <img alt={alt} {...props} loading="eager" fetchPriority="high" decoding="async" />;
}
