import { createMDX } from 'fumadocs-mdx/next';

const withMDX = createMDX();

// Pages that have a Markdown twin (the .md routes): every docs page.
const DOC_PATHS = ['/guide/:path*', '/reference', '/reference/:path*', '/examples'];

/** @type {import('next').NextConfig} */
const config = {
  reactStrictMode: true,
  poweredByHeader: false,
  // Fail on a Link to a route that does not exist, at type-check time.
  typedRoutes: true,

  async redirects() {
    return [
      // The old Vercel domain moves to the canonical one, path and query
      // kept. Only that exact host matches, so preview deployments
      // (tuimark-git-….vercel.app) are left alone.
      {
        source: '/:path*',
        has: [{ type: 'host', value: 'tuimark.vercel.app' }],
        destination: 'https://tuimark.dev/:path*',
        permanent: true,
      },
      // The VitePress site served every page as NAME.html too.
      { source: '/index.html', destination: '/', permanent: true },
      { source: '/:path+.html', destination: '/:path+', permanent: true },
      // /guide has no page of its own; the guide starts here.
      { source: '/guide', destination: '/guide/getting-started', permanent: false },
    ];
  },

  async rewrites() {
    return {
      beforeFiles: [
        // /guide/layout.md is the Markdown of /guide/layout.
        { source: '/:path+.md', destination: '/md/:path+' },
        // A client that asks for Markdown (Accept: text/markdown) gets it
        // at the page's own URL.
        ...DOC_PATHS.map((source) => ({
          source,
          has: [{ type: 'header', key: 'accept', value: '(.*)text/markdown(.*)' }],
          destination: `/md${source}`,
        })),
      ],
    };
  },

  async headers() {
    return [
      // These URLs answer with HTML or Markdown depending on Accept.
      ...DOC_PATHS.map((source) => ({ source, headers: [{ key: 'Vary', value: 'Accept' }] })),
      {
        source: '/shots/:file*',
        headers: [{ key: 'Cache-Control', value: 'public, max-age=86400, stale-while-revalidate=604800' }],
      },
    ];
  },
};

export default withMDX(config);
