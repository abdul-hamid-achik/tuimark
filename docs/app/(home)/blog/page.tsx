import type { Metadata, Route } from 'next';
import Link from 'next/link';
import { JsonLd } from '@/components/json-ld';
import { formatDate, posts } from '@/lib/blog';
import { RSS, pageMetadata } from '@/lib/metadata';
import { SITE_NAME, SITE_URL, absoluteUrl } from '@/lib/site';
import '../pages.css';

const TITLE = 'Blog';
const DESCRIPTION =
  'News and notes from Tuimark, the view language for terminals: releases, the npm package, the component kit, and why terminal UIs work well as documents.';

export const metadata: Metadata = pageMetadata({
  title: TITLE,
  description: DESCRIPTION,
  path: '/blog',
  image: { url: '/og/blog/image.png', alt: `${SITE_NAME} blog` },
});

export default function BlogIndex() {
  const all = posts();
  return (
    <main className="pg">
      <JsonLd
        data={[
          {
            '@context': 'https://schema.org',
            '@type': 'Blog',
            name: `${SITE_NAME} blog`,
            description: DESCRIPTION,
            url: absoluteUrl('/blog'),
            inLanguage: 'en',
            publisher: { '@type': 'Organization', name: SITE_NAME, url: SITE_URL },
            blogPost: all.map((p) => ({
              '@type': 'BlogPosting',
              headline: p.data.title,
              url: absoluteUrl(p.url),
              datePublished: p.data.date.toISOString(),
              author: { '@type': 'Person', name: p.data.author },
            })),
          },
          {
            '@context': 'https://schema.org',
            '@type': 'BreadcrumbList',
            itemListElement: [
              { '@type': 'ListItem', position: 1, name: SITE_NAME, item: SITE_URL },
              { '@type': 'ListItem', position: 2, name: TITLE, item: absoluteUrl('/blog') },
            ],
          },
        ]}
      />
      <div className="pg-shell pg-narrow">
        <header className="pg-head">
          <p className="pg-kicker">blog</p>
          <h1>Notes from the cell grid.</h1>
          <p className="pg-lede">
            Releases, design notes, and how Tuimark is built. Follow along with the{' '}
            <a href={RSS.url}>RSS feed</a>.
          </p>
        </header>
        <ol className="pg-posts">
          {all.map((p) => (
            <li key={p.url}>
              <article className="tf">
                <span className="tf-title">
                  <b>▸</b> {p.slugs[0]}
                </span>
                <p className="pg-meta">
                  <time dateTime={p.data.date.toISOString().slice(0, 10)}>{formatDate(p.data.date)}</time> ·{' '}
                  {p.data.author}
                </p>
                <h2>
                  <Link href={p.url as Route}>{p.data.title}</Link>
                </h2>
                <p className="pg-summary">{p.data.description}</p>
                <Link className="pg-link" href={p.url as Route} aria-label={`Read “${p.data.title}”`}>
                  Read the post <span aria-hidden="true">→</span>
                </Link>
              </article>
            </li>
          ))}
        </ol>
      </div>
    </main>
  );
}
