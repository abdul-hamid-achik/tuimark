import type { Metadata, Viewport } from 'next';
import { Geist, JetBrains_Mono } from 'next/font/google';
import { RootProvider } from 'fumadocs-ui/provider/next';
import { SearchDialog } from '@/components/search-lazy';
import { AUTHOR, DESCRIPTION, KEYWORDS, SITE_NAME, SITE_URL, TAGLINE } from '@/lib/site';
import './global.css';

const sans = Geist({ subsets: ['latin'], variable: '--font-geist', display: 'swap' });
const mono = JetBrains_Mono({ subsets: ['latin'], variable: '--font-jetbrains', display: 'swap' });

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    default: `${SITE_NAME}: ${TAGLINE.toLowerCase()}`,
    template: `%s | ${SITE_NAME}`,
  },
  description: DESCRIPTION,
  applicationName: SITE_NAME,
  authors: [{ name: AUTHOR, url: 'https://github.com/abdul-hamid-achik' }],
  creator: AUTHOR,
  keywords: KEYWORDS,
  alternates: { canonical: '/' },
  openGraph: {
    type: 'website',
    siteName: SITE_NAME,
    locale: 'en_US',
    url: '/',
  },
  twitter: {
    card: 'summary_large_image',
  },
  icons: { icon: [{ url: '/favicon.svg', type: 'image/svg+xml' }] },
  formatDetection: { telephone: false, email: false, address: false },
};

export const viewport: Viewport = {
  themeColor: [
    { media: '(prefers-color-scheme: light)', color: '#ffffff' },
    { media: '(prefers-color-scheme: dark)', color: '#0d1117' },
  ],
};

export default function RootLayout({ children }: LayoutProps<'/'>) {
  return (
    <html lang="en" className={`${sans.variable} ${mono.variable}`} suppressHydrationWarning>
      <body className="flex min-h-screen flex-col">
        <RootProvider
          // Terminal tools live in dark terminals; the site opens the same
          // way, and the toggle offers Tuimark's built-in light theme.
          theme={{ defaultTheme: 'dark', enableSystem: false }}
          search={{
            SearchDialog,
            links: [
              ['Getting started', '/guide/getting-started'],
              ['Use Tuimark with AI agents', '/guide/agents'],
              ['Widgets', '/guide/widgets'],
              ['Tags and attributes', '/reference/tags'],
              ['Examples', '/examples'],
            ],
          }}
        >
          {children}
        </RootProvider>
      </body>
    </html>
  );
}
