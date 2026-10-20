#!/usr/bin/env node
// Checks every internal link of the built site (run `next build` first):
// each href to a page of this site must name a page that was built, and its
// #fragment, if any, an element id on that page; each /shots/… or other
// static file must exist in public/. Links to tuimark.dev in the
// repository's README.md are held to the same rule, so the front page of
// the repository cannot point at a page or an anchor the site lost.
//
//   npm run build && npm run check-links
//
// Exits 1 and lists every broken link. Stricter than the VitePress build it
// replaces, which checked pages but not anchors.

import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const DOCS = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const APP = join(DOCS, '.next', 'server', 'app');
const PUBLIC = join(DOCS, 'public');
const README = resolve(DOCS, '..', 'README.md');
const ORIGIN = 'https://tuimark.dev';

if (!existsSync(APP)) {
  console.error('check-links: .next/server/app not found; run `npm run build` first');
  process.exit(1);
}

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, out);
    else out.push(p);
  }
  return out;
}

const files = walk(APP);

/** URL path → the set of ids on that page. */
const pages = new Map();
for (const f of files.filter((f) => f.endsWith('.html'))) {
  const rel = relative(APP, f).replace(/\\/g, '/').replace(/\.html$/, '');
  if (rel.startsWith('_')) continue; // _not-found, _global-error
  // Route groups, (docs) and (home), are not part of the URL.
  const path = '/' + rel.split('/').filter((s) => !/^\(.*\)$/.test(s)).join('/');
  const url = path === '/index' ? '/' : path;
  const html = readFileSync(f, 'utf8');
  const ids = new Set([...html.matchAll(/\sid="([^"]+)"/g)].map((m) => decodeEntities(m[1])));
  pages.set(url, { ids, html, file: f });
}

/** Paths served by route handlers (llms.txt, sitemap.xml, the .md twins, …). */
const routes = new Set();
for (const f of files.filter((f) => f.endsWith('.body'))) {
  const rel = relative(APP, f).replace(/\\/g, '/').replace(/\.body$/, '');
  // A route group, such as (home) for /blog/rss.xml, is not part of the URL.
  routes.add('/' + rel.split('/').filter((s) => !/^\(.*\)$/.test(s)).join('/'));
}
// /guide/x.md is rewritten to the /md/guide/x route.
for (const url of pages.keys()) if (routes.has(`/md${url}`)) routes.add(`${url}.md`);

function decodeEntities(s) {
  return s
    .replace(/&amp;/g, '&')
    .replace(/&quot;/g, '"')
    .replace(/&#x27;|&#39;/g, "'")
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>');
}

const broken = [];

function check(href, where) {
  let url;
  try {
    url = new URL(href, `${ORIGIN}${where.page ?? '/'}`);
  } catch {
    broken.push({ where: where.label, href, why: 'not a URL' });
    return;
  }
  if (url.origin !== ORIGIN) return;
  const path = decodeURIComponent(url.pathname).replace(/\/$/, '') || '/';
  const frag = decodeURIComponent(url.hash.slice(1));
  if (pages.has(path)) {
    if (frag && !pages.get(path).ids.has(frag)) broken.push({ where: where.label, href, why: `no id="${frag}" on ${path}` });
    return;
  }
  if (routes.has(path)) return;
  if (existsSync(join(PUBLIC, path)) && statSync(join(PUBLIC, path)).isFile()) return;
  // /guide redirects to the first guide page (next.config.mjs).
  if (path === '/guide') return;
  broken.push({ where: where.label, href, why: 'no such page or file' });
}

let count = 0;
for (const [url, { html }] of pages) {
  // Only what the page links to itself: skip <link rel="preload"> and the
  // RSC payload, which repeat hrefs for the router.
  const body = html.replace(/<script[\s\S]*?<\/script>/g, '');
  for (const m of body.matchAll(/<(a|img|link)\s[^>]*?(href|src)="([^"]*)"/g)) {
    const [, tag, , raw] = m;
    if (tag === 'link' && !/rel="(canonical|alternate|icon)"/.test(m[0])) continue;
    const href = decodeEntities(raw);
    if (!href || href.startsWith('mailto:') || href.startsWith('data:') || href.startsWith('/_next/')) continue;
    count++;
    check(href, { page: url, label: url });
  }
}

// README.md: links into the site.
let readmeLinks = 0;
if (existsSync(README)) {
  const md = readFileSync(README, 'utf8');
  for (const m of md.matchAll(/https:\/\/tuimark\.dev[^\s)>\]"'`]*/g)) {
    readmeLinks++;
    check(m[0], { page: '/', label: 'README.md' });
  }
}

if (broken.length) {
  console.error(`check-links: ${broken.length} broken link(s):`);
  for (const b of broken) console.error(`  ${b.where}: ${b.href}  (${b.why})`);
  process.exit(1);
}
console.log(`check-links: ${count} links on ${pages.size} pages and ${readmeLinks} in README.md are fine`);
