import { blog } from '@/.source/server';
import { loader } from 'fumadocs-core/source';
import { toFumadocsSource } from 'fumadocs-mdx/runtime/server';
import { absoluteUrl } from './site';

// The blog: blog/SLUG.mdx is /blog/SLUG, with its Markdown twin at
// /blog/SLUG.md and its social card at /og/blog/SLUG/image.png.
export const blogSource = loader({
  baseUrl: '/blog',
  source: toFumadocsSource(blog, []),
});

export type Post = NonNullable<ReturnType<typeof blogSource.getPage>>;

/** Every post, newest first. */
export function posts(): Post[] {
  return [...blogSource.getPages()].sort((a, b) => b.data.date.getTime() - a.data.date.getTime());
}

export function postSlug(post: Post): string {
  return post.slugs[0];
}

export function postOgImageUrl(post: Post): string {
  return `/og/blog/${postSlug(post)}/image.png`;
}

/** A date as the posts print it: October 19, 2026 (the date is a UTC day). */
export function formatDate(date: Date): string {
  return date.toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC' });
}

/** A post as Markdown, for its .md twin and /llms-full.txt. */
export async function postMarkdown(post: Post): Promise<string> {
  const body = await post.data.getText('processed');
  const lines = [
    `# ${post.data.title}`,
    '',
    `URL: ${absoluteUrl(post.url)}`,
    `Date: ${post.data.date.toISOString().slice(0, 10)}`,
    `Author: ${post.data.author}`,
    '',
    `> ${post.data.description}`,
    '',
    body.trim(),
    '',
  ];
  return lines.join('\n');
}
