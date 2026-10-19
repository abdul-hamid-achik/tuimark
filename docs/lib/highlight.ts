import { highlight } from 'fumadocs-core/highlight';

/** The languages the landing page highlights; .tui reads as XML and .tcss as CSS, as in the docs. */
export type Lang = 'xml' | 'css' | 'json' | 'go' | 'ts' | 'sh';

/**
 * Highlights code at build time with the docs' Shiki themes, both at once:
 * the site's theme picks the colors with CSS, so no highlighter ships to
 * the browser.
 */
export function code(source: string, lang: Lang) {
  return highlight(source.replace(/\n+$/, ''), {
    lang,
    themes: { light: 'github-light', dark: 'github-dark' },
    // Only CSS variables, no inline color: Fumadocs' stylesheet picks
    // --shiki-light or --shiki-dark from the site's theme.
    defaultColor: false,
  });
}
