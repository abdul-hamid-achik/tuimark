import type { Route } from 'next';
import Link from 'next/link';
import { AUTHOR, REPO_URL } from '@/lib/site';

const columns: { title: string; links: { text: string; href: string; external?: boolean }[] }[] = [
  {
    title: 'learn',
    links: [
      { text: 'Getting started', href: '/guide/getting-started' },
      { text: 'Use with AI agents', href: '/guide/agents' },
      { text: 'The .tui language', href: '/guide/language' },
      { text: 'Widgets', href: '/guide/widgets' },
      { text: 'The kit', href: '/guide/kit' },
    ],
  },
  {
    title: 'hosts',
    links: [
      { text: 'Go API', href: '/guide/go-api' },
      { text: 'TypeScript client', href: '/guide/typescript' },
      { text: 'Host protocol', href: '/guide/host-protocol' },
      { text: 'MCP server', href: '/guide/mcp' },
      { text: 'Editors', href: '/guide/editors' },
    ],
  },
  {
    title: 'reference',
    links: [
      { text: 'CLI', href: '/reference/cli' },
      { text: 'Tags and attributes', href: '/reference/tags' },
      { text: 'CSS properties', href: '/reference/css' },
      { text: 'Diagnostic codes', href: '/reference/diagnostics' },
      { text: 'Examples', href: '/examples' },
    ],
  },
  {
    title: 'project',
    links: [
      { text: 'GitHub', href: REPO_URL, external: true },
      { text: 'Releases', href: `${REPO_URL}/releases`, external: true },
      { text: 'Changelog', href: `${REPO_URL}/blob/main/CHANGELOG.md`, external: true },
      { text: 'llms.txt', href: '/llms.txt', external: true },
    ],
  },
];

export function SiteFooter() {
  return (
    <footer className="site-footer">
      <div className="site-footer-inner">
        <nav className="site-footer-cols" aria-label="Footer">
          {columns.map((c) => (
            <div key={c.title}>
              <p className="site-footer-title">{c.title}</p>
              <ul>
                {c.links.map((l) => (
                  <li key={l.href}>
                    {l.external ? (
                      <a href={l.href}>{l.text}</a>
                    ) : (
                      <Link href={l.href as Route}>{l.text}</Link>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </nav>
        <p className="site-footer-legal">
          Released under the MIT License. Copyright © {AUTHOR}.
        </p>
      </div>
    </footer>
  );
}
