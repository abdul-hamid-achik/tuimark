import type { BaseLayoutProps } from 'fumadocs-ui/layouts/shared';
import { REPO_URL, VERSION } from './site';

/** The wordmark: a block cursor sitting in a cell, then the name. */
export function Wordmark() {
  return (
    <span className="wordmark">
      <span className="wordmark-cursor" aria-hidden="true" />
      tuimark
    </span>
  );
}

export function baseOptions(): BaseLayoutProps {
  return {
    nav: {
      title: <Wordmark />,
      url: '/',
    },
    githubUrl: REPO_URL,
    links: [
      { text: 'Guide', url: '/guide/getting-started', active: 'none' },
      { text: 'Reference', url: '/reference', active: 'nested-url' },
      { text: 'Examples', url: '/examples' },
      { text: 'Kit', url: '/kit' },
      { text: 'Blog', url: '/blog', active: 'nested-url' },
      {
        type: 'menu',
        text: VERSION,
        items: [
          { text: 'Release notes', url: `${REPO_URL}/releases/tag/${VERSION}`, external: true },
          { text: 'All releases', url: `${REPO_URL}/releases`, external: true },
          { text: 'Changelog', url: `${REPO_URL}/blob/main/CHANGELOG.md`, external: true },
          { text: 'Go package', url: 'https://pkg.go.dev/github.com/abdul-hamid-achik/tuimark', external: true },
        ],
      },
    ],
  };
}
