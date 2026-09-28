import { defineConfig, type HeadConfig } from 'vitepress'

// The docs deploy at the site root on Vercel (see the repository's
// vercel.json). Set DOCS_BASE=/tuimark/ to build for a GitHub Pages
// project site instead.
const base = process.env.DOCS_BASE ?? '/'

// Canonical URLs, the sitemap and social cards point here (the Vercel
// project's domain; change it here and in public/robots.txt for a custom one).
const SITE_URL = 'https://tuimark.vercel.app'
const REPO_URL = 'https://github.com/abdul-hamid-achik/tuimark'
const VERSION = 'v0.3.1'
const DESCRIPTION =
  'Tuimark is a view language for terminals: UI as .tui markup, .tcss styles and JSON data, interpreted by a small Go runtime that lays out a cell grid and dumps every frame as JSON for coding agents.'

export default defineConfig({
  title: 'Tuimark',
  description: DESCRIPTION,
  lang: 'en-US',
  cleanUrls: true,
  lastUpdated: true,
  base,
  // Terminal tools live in dark terminals; the site opens the same way, and
  // the toggle offers Tuimark's built-in light theme.
  appearance: 'dark',
  // Fail the build on a link to a page that does not exist.
  ignoreDeadLinks: false,
  srcExclude: ['snippets/**', 'scripts/**'],

  markdown: {
    // Fenced blocks can say ```tui and ```tcss. A frame the runtime painted
    // is ```grid (plain text, drawn with touching rows), and a command with
    // its output is ```term (the prompt line and the dump's section headers
    // are highlighted, the output is left alone).
    languages: [
      { name: 'grid', scopeName: 'text.tuimark.grid', patterns: [], repository: {} },
      {
        name: 'term',
        scopeName: 'text.tuimark.term',
        patterns: [
          {
            match: String.raw`^(\$) (\S+)(.*)$`,
            captures: { 1: { name: 'comment' }, 2: { name: 'entity.name.function' }, 3: { name: 'variable.other' } },
          },
          { match: String.raw`^=== .* ===$`, name: 'comment' },
          { match: String.raw`^(error|warning) ([VLB]\d{3})`, captures: { 1: { name: 'keyword' }, 2: { name: 'keyword' } } },
          { match: String.raw`^ok$`, name: 'markup.inserted' },
        ],
        repository: {},
      },
    ],
    languageAlias: { tui: 'xml', tcss: 'css' },
  },

  head: [
    ['link', { rel: 'icon', type: 'image/svg+xml', href: `${base}favicon.svg` }],
    ['meta', { name: 'author', content: 'Abdul Hamid Achik' }],
    ['meta', { name: 'keywords', content: 'terminal UI, TUI, view language, Go TUI library, declarative terminal UI, XML UI, TCSS, coding agents, AGENTS.md, golden tests, Glyphrun' }],
    ['meta', { name: 'theme-color', content: '#ffffff', media: '(prefers-color-scheme: light)' }],
    ['meta', { name: 'theme-color', content: '#0d1117', media: '(prefers-color-scheme: dark)' }],
    ['meta', { name: 'twitter:card', content: 'summary' }],
    ['meta', { name: 'twitter:creator', content: '@abdulachik' }],
    [
      'script',
      { type: 'application/ld+json' },
      JSON.stringify({
        '@context': 'https://schema.org',
        '@type': 'SoftwareApplication',
        name: 'Tuimark',
        applicationCategory: 'DeveloperApplication',
        operatingSystem: 'macOS, Linux, Windows',
        softwareVersion: VERSION.slice(1),
        description: DESCRIPTION,
        url: SITE_URL,
        downloadUrl: `${REPO_URL}/releases`,
        codeRepository: REPO_URL,
        license: 'https://opensource.org/licenses/MIT',
        offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
        author: { '@type': 'Person', name: 'Abdul Hamid Achik' },
      }),
    ],
  ],

  sitemap: { hostname: SITE_URL },

  transformPageData(pageData) {
    const isHome = pageData.frontmatter.layout === 'home'
    const rel = pageData.relativePath.replace(/(^|\/)index\.md$/, '$1').replace(/\.md$/, '')
    const url = rel ? `${SITE_URL}/${rel}` : SITE_URL
    const title = isHome ? 'Tuimark — terminal UIs, written as text' : `${pageData.title} | Tuimark`
    const description = pageData.frontmatter.description || pageData.description || DESCRIPTION
    const head: HeadConfig[] = [
      ['link', { rel: 'canonical', href: url }],
      ['meta', { property: 'og:type', content: 'website' }],
      ['meta', { property: 'og:site_name', content: 'Tuimark' }],
      ['meta', { property: 'og:title', content: title }],
      ['meta', { property: 'og:description', content: description }],
      ['meta', { property: 'og:url', content: url }],
      ['meta', { name: 'twitter:title', content: title }],
      ['meta', { name: 'twitter:description', content: description }],
    ]
    pageData.frontmatter.head = [...(pageData.frontmatter.head ?? []), ...head]
  },

  themeConfig: {
    // No image logo: the nav draws the wordmark in CSS (custom.css).
    siteTitle: 'tuimark',

    nav: [
      { text: 'Guide', link: '/guide/getting-started', activeMatch: '/guide/' },
      { text: 'Reference', link: '/reference/', activeMatch: '/reference/' },
      { text: 'Examples', link: '/examples' },
      {
        text: VERSION,
        items: [
          { text: 'Release notes', link: `${REPO_URL}/releases/tag/${VERSION}` },
          { text: 'All releases', link: `${REPO_URL}/releases` },
          { text: 'Go package', link: 'https://pkg.go.dev/github.com/abdul-hamid-achik/tuimark' },
        ],
      },
    ],

    sidebar: {
      '/guide/': [
        {
          text: 'Start',
          items: [
            { text: 'Getting started', link: '/guide/getting-started' },
            { text: 'Working with agents', link: '/guide/agents' },
          ],
        },
        {
          text: 'The language',
          items: [
            { text: 'The .tui language', link: '/guide/language' },
            { text: 'TCSS', link: '/guide/tcss' },
            { text: 'Layout', link: '/guide/layout' },
            { text: 'Bindings and data', link: '/guide/bindings' },
            { text: 'Keymap, actions and focus', link: '/guide/interaction' },
            { text: 'Widgets', link: '/guide/widgets' },
          ],
        },
        {
          text: 'Tools and hosts',
          items: [
            { text: 'Dumps and tools', link: '/guide/tools' },
            { text: 'Go API', link: '/guide/go-api' },
            { text: 'Testing', link: '/guide/testing' },
            { text: 'Host protocol', link: '/guide/host-protocol' },
            { text: 'MCP server', link: '/guide/mcp' },
          ],
        },
        {
          text: 'More',
          items: [
            { text: 'Examples', link: '/examples' },
            { text: 'Reference', link: '/reference/' },
          ],
        },
      ],
      '/reference/': [
        {
          text: 'Reference',
          items: [
            { text: 'Overview', link: '/reference/' },
            { text: 'CLI', link: '/reference/cli' },
            { text: 'Tags and attributes', link: '/reference/tags' },
            { text: 'CSS properties', link: '/reference/css' },
            { text: 'Key tokens', link: '/reference/keys' },
            { text: 'Diagnostic codes', link: '/reference/diagnostics' },
          ],
        },
        {
          text: 'Guide',
          items: [
            { text: 'Getting started', link: '/guide/getting-started' },
            { text: 'Examples', link: '/examples' },
          ],
        },
      ],
    },

    outline: { level: [2, 3] },

    socialLinks: [{ icon: 'github', link: REPO_URL }],

    editLink: {
      pattern: `${REPO_URL}/edit/main/docs/:path`,
      text: 'Edit this page on GitHub',
    },

    search: { provider: 'local' },

    notFound: {
      code: '404',
      title: 'no such screen',
      quote: 'Nothing is laid out at this path. Switch back to a screen that exists.',
      linkText: '← back to tuimark',
    },

    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright © Abdul Hamid Achik',
    },
  },
})
