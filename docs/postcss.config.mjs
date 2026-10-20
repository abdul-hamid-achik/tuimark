const config = {
  plugins: {
    // Next.js minifies the CSS itself. Tailwind's own optimizer (Lightning
    // CSS) would only repeat that, and it warns on the ::highlight() rules
    // that fumadocs-ui's search dialog uses for its matches.
    '@tailwindcss/postcss': { optimize: false },
  },
};

export default config;
