import type { Metadata, Route } from 'next';
import Link from 'next/link';
import { notFound } from 'next/navigation';
import { getMDXComponents } from '@/components/mdx';
import { JsonLd } from '@/components/json-ld';
import { blogSource, formatDate, postOgImageUrl, posts } from '@/lib/blog';
import { RSS, pageMetadata } from '@/lib/metadata';
import { AUTHOR, SITE_NAME, SITE_URL, absoluteUrl } from '@/lib/site';
import '../../pages.css';

export const dynamicParams = false;

export default async function Post(props: PageProps<'/blog/[slug]'>) {
  const { slug } = await props.params;
  const post = blogSource.getPage([slug]);
  if (!post) notFound();

  const MDX = post.data.body;
  const date = post.data.date;
  const modified = post.data.lastModified ? new Date(post.data.lastModified) : undefined;
  const all = posts();
  const i = all.findIndex((p) => p.url === post.url);
  const newer = all[i - 1];
  const older = all[i + 1];

  return (
    <main className="pg">
      <JsonLd
        data={[
          {
            '@context': 'https://schema.org',
            '@type': 'BlogPosting',
            headline: post.data.title,
            description: post.data.description,
            url: absoluteUrl(post.url),
            mainEntityOfPage: absoluteUrl(post.url),
            image: absoluteUrl(postOgImageUrl(post)),
            datePublished: date.toISOString(),
            dateModified: (modified && modified > date ? modified : date).toISOString(),
            inLanguage: 'en',
            author: { '@type': 'Person', name: post.data.author, url: 'https://github.com/abdul-hamid-achik' },
            publisher: { '@type': 'Organization', name: SITE_NAME, url: SITE_URL },
            isPartOf: { '@type': 'Blog', name: `${SITE_NAME} blog`, url: absoluteUrl('/blog') },
          },
          {
            '@context': 'https://schema.org',
            '@type': 'BreadcrumbList',
            itemListElement: [
              { '@type': 'ListItem', position: 1, name: SITE_NAME, item: SITE_URL },
              { '@type': 'ListItem', position: 2, name: 'Blog', item: absoluteUrl('/blog') },
              { '@type': 'ListItem', position: 3, name: post.data.title, item: absoluteUrl(post.url) },
            ],
          },
        ]}
      />
      <article className="pg-shell pg-narrow">
        <nav className="pg-crumbs" aria-label="Breadcrumb">
          <ol>
            <li>
              <Link href="/">tuimark</Link>
            </li>
            <li>
              <Link href="/blog">blog</Link>
            </li>
          </ol>
        </nav>
        <header className="pg-head">
          <p className="pg-kicker">
            <time dateTime={date.toISOString().slice(0, 10)}>{formatDate(date)}</time> · {post.data.author}
          </p>
          <h1>{post.data.title}</h1>
          <p className="pg-lede">{post.data.description}</p>
        </header>
        <div className="prose pg-post-body">
          <MDX components={getMDXComponents()} />
        </div>
        <footer className="pg-post-foot">
          {newer && (
            <Link className="pg-link" href={newer.url as Route}>
              <span aria-hidden="true">←</span> {newer.data.title}
            </Link>
          )}
          {older && (
            <Link className="pg-link" href={older.url as Route}>
              {older.data.title} <span aria-hidden="true">→</span>
            </Link>
          )}
          <Link className="pg-link" href="/blog">
            All posts
          </Link>
          <a className="pg-link" href={RSS.url}>
            RSS
          </a>
        </footer>
      </article>
    </main>
  );
}

export function generateStaticParams() {
  return blogSource.getPages().map((p) => ({ slug: p.slugs[0] }));
}

export async function generateMetadata(props: PageProps<'/blog/[slug]'>): Promise<Metadata> {
  const { slug } = await props.params;
  const post = blogSource.getPage([slug]);
  if (!post) notFound();
  const modified = post.data.lastModified ? new Date(post.data.lastModified) : undefined;
  return pageMetadata({
    title: post.data.title,
    description: post.data.description,
    path: post.url,
    image: { url: postOgImageUrl(post), alt: post.data.title },
    type: 'article',
    types: { 'text/markdown': `${post.url}.md` },
    published: post.data.date,
    modified: modified && modified > post.data.date ? modified : undefined,
    authors: [post.data.author ?? AUTHOR],
  });
}
