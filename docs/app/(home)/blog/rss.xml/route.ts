import { posts } from '@/lib/blog';
import { RSS } from '@/lib/metadata';
import { absoluteUrl } from '@/lib/site';

// The blog as RSS 2.0, built once with the site.
export const revalidate = false;

const esc = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&apos;');

export function GET() {
  const all = posts();
  const items = all.map((p) =>
    [
      '    <item>',
      `      <title>${esc(p.data.title)}</title>`,
      `      <link>${absoluteUrl(p.url)}</link>`,
      `      <guid isPermaLink="true">${absoluteUrl(p.url)}</guid>`,
      `      <pubDate>${p.data.date.toUTCString()}</pubDate>`,
      `      <dc:creator>${esc(p.data.author)}</dc:creator>`,
      `      <description>${esc(p.data.description)}</description>`,
      '    </item>',
    ].join('\n'),
  );
  const xml = [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<rss version="2.0" xmlns:atom="http://www.w3.org/2005/Atom" xmlns:dc="http://purl.org/dc/elements/1.1/">',
    '  <channel>',
    `    <title>${esc(RSS.title)}</title>`,
    `    <link>${absoluteUrl('/blog')}</link>`,
    `    <atom:link href="${absoluteUrl(RSS.url)}" rel="self" type="application/rss+xml"/>`,
    '    <description>News and notes from Tuimark, the view language for terminals.</description>',
    '    <language>en</language>',
    ...(all[0] ? [`    <lastBuildDate>${all[0].data.date.toUTCString()}</lastBuildDate>`] : []),
    ...items,
    '  </channel>',
    '</rss>',
    '',
  ].join('\n');
  return new Response(xml, { headers: { 'Content-Type': 'application/rss+xml; charset=utf-8' } });
}
