import type { Metadata } from 'next';
import { notFound } from 'next/navigation';
import {
  DocsBody,
  DocsDescription,
  DocsPage,
  DocsTitle,
  EditOnGitHub,
  MarkdownCopyButton,
  PageLastUpdate,
  ViewOptionsPopover,
} from 'fumadocs-ui/layouts/docs/page';
import { createRelativeLink } from 'fumadocs-ui/mdx';
import { getMDXComponents } from '@/components/mdx';
import { JsonLd } from '@/components/json-ld';
import { markdownUrl, ogImageUrl, sectionOf, source } from '@/lib/source';
import { AUTHOR, REPO_URL, SITE_NAME, SITE_URL, absoluteUrl } from '@/lib/site';

export const dynamicParams = false;

export default async function Page(props: PageProps<'/[...slug]'>) {
  const { slug } = await props.params;
  const page = source.getPage(slug);
  if (!page) notFound();

  const MDX = page.data.body;
  const md = markdownUrl(page);
  const githubUrl = `${REPO_URL}/blob/main/docs/content/${page.path}`;
  const section = sectionOf(page);
  const lastModified = page.data.lastModified ? new Date(page.data.lastModified) : undefined;

  return (
    <DocsPage
      toc={page.data.toc}
      full={page.data.full}
      tableOfContent={{ style: 'clerk' }}
      footer={{ enabled: true }}
    >
      <JsonLd
        data={[
          {
            '@context': 'https://schema.org',
            '@type': 'TechArticle',
            headline: page.data.title,
            description: page.data.description,
            url: absoluteUrl(page.url),
            image: absoluteUrl(ogImageUrl(page)),
            inLanguage: 'en',
            ...(lastModified ? { dateModified: lastModified.toISOString() } : {}),
            author: { '@type': 'Person', name: AUTHOR },
            publisher: { '@type': 'Organization', name: SITE_NAME, url: SITE_URL },
            isPartOf: { '@type': 'WebSite', name: SITE_NAME, url: SITE_URL },
            about: { '@type': 'SoftwareApplication', name: SITE_NAME, url: SITE_URL },
          },
          {
            '@context': 'https://schema.org',
            '@type': 'BreadcrumbList',
            itemListElement: [
              { '@type': 'ListItem', position: 1, name: SITE_NAME, item: SITE_URL },
              { '@type': 'ListItem', position: 2, name: section.name, item: absoluteUrl(section.url) },
              ...(page.url === section.url
                ? []
                : [{ '@type': 'ListItem', position: 3, name: page.data.title, item: absoluteUrl(page.url) }]),
            ],
          },
        ]}
      />
      <DocsTitle>{page.data.title}</DocsTitle>
      <DocsDescription className="mb-0">{page.data.description}</DocsDescription>
      <div className="page-actions flex flex-row flex-wrap items-center gap-2 border-b pb-6">
        <MarkdownCopyButton markdownUrl={md} />
        <ViewOptionsPopover markdownUrl={md} githubUrl={githubUrl} pageUrl={absoluteUrl(page.url)} />
      </div>
      <DocsBody>
        <MDX components={getMDXComponents({ a: createRelativeLink(source, page) })} />
      </DocsBody>
      <div className="page-meta flex flex-row flex-wrap items-center justify-between gap-4">
        {page.data.editLink !== false ? <EditOnGitHub href={githubUrl} /> : <span />}
        {lastModified && <PageLastUpdate date={lastModified} />}
      </div>
    </DocsPage>
  );
}

export function generateStaticParams() {
  return source.generateParams();
}

export async function generateMetadata(props: PageProps<'/[...slug]'>): Promise<Metadata> {
  const { slug } = await props.params;
  const page = source.getPage(slug);
  if (!page) notFound();

  const image = { url: ogImageUrl(page), width: 1200, height: 630, alt: page.data.title };
  return {
    title: page.data.title,
    description: page.data.description,
    alternates: {
      canonical: page.url,
      types: { 'text/markdown': markdownUrl(page) },
    },
    openGraph: {
      type: 'article',
      url: page.url,
      title: `${page.data.title} | ${SITE_NAME}`,
      description: page.data.description,
      images: [image],
    },
    twitter: {
      card: 'summary_large_image',
      title: `${page.data.title} | ${SITE_NAME}`,
      description: page.data.description,
      images: [image.url],
    },
  };
}
