// Facts about the site and the project, in one place.

/** The canonical origin: every canonical URL, the sitemap and the social cards point here. */
export const SITE_URL = 'https://tuimark.dev';
export const SITE_NAME = 'Tuimark';
export const REPO_URL = 'https://github.com/abdul-hamid-achik/tuimark';
export const VERSION = 'v0.4.2';
export const AUTHOR = 'Abdul Hamid Achik';

export const TAGLINE = 'Terminal UIs as documents';
/** The landing page's title, and the default for a page without one. */
export const HOME_TITLE = `${SITE_NAME}: terminal UIs as documents`;
export const PITCH =
  'Write .tui and .tcss, let any language drive it, let agents design it.';
export const DESCRIPTION =
  'Tuimark is a view language for terminals: UI as .tui markup, .tcss styles and JSON data, interpreted by a small Go runtime that lays out a cell grid and dumps every frame as JSON for coding agents. Drive it from Go, TypeScript, or any language.';

export const KEYWORDS = [
  'terminal UI',
  'TUI',
  'TUI framework',
  'view language',
  'Go TUI library',
  'declarative terminal UI',
  'TypeScript TUI',
  'XML UI',
  'TCSS',
  'coding agents',
  'AGENTS.md',
  'MCP server',
  'golden tests',
];

/** The ways to install Tuimark, as the install tabs show them. */
export const INSTALL = {
  brew: 'brew install --cask abdul-hamid-achik/tap/tuimark',
  go: 'go install github.com/abdul-hamid-achik/tuimark/cmd/tuimark@latest',
  goLibrary: 'go get github.com/abdul-hamid-achik/tuimark@latest',
  npmPackage: '@thelacanians/tuimark',
  npmGlobal: 'npm i -g @thelacanians/tuimark',
  npx: 'npx @thelacanians/tuimark version',
  npmNew: 'bunx @thelacanians/tuimark new my-app --ts',
  npmInstall: 'npm i @thelacanians/tuimark',
};

/**
 * Whether @thelacanians/tuimark is on npm (it is, since 0.4.2). While it is
 * false, the npm install tab says "coming soon" and shows its commands
 * without a copy button, and /llms.txt lists only Homebrew and Go.
 */
export const NPM_PUBLISHED = true;

/** An absolute URL on the canonical origin. */
export function absoluteUrl(path: string): string {
  return path === '/' ? SITE_URL : `${SITE_URL}${path.startsWith('/') ? path : `/${path}`}`;
}
