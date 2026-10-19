'use client';

import dynamic from 'next/dynamic';

// The search dialog and its index client load the first time a reader opens
// search (⌘K or the search button), not with every page.
export const SearchDialog = dynamic(() => import('./search').then((m) => m.SearchDialog), { ssr: false });
