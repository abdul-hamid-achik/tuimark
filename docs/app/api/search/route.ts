import { createFromSource } from 'fumadocs-core/search/server';
import { source } from '@/lib/source';

// The search index, exported once at build time and served as a static
// file; the search dialog downloads it and searches in the browser.
export const revalidate = false;

export const { staticGET: GET } = createFromSource(source, { language: 'english' });
