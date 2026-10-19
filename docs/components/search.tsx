'use client';

import { useMemo } from 'react';
import { useDocsSearch } from 'fumadocs-core/search/client';
import { staticClient } from 'fumadocs-core/search/client/orama-static';
import {
  SearchDialog as Dialog,
  SearchDialogClose,
  SearchDialogContent,
  SearchDialogHeader,
  SearchDialogIcon,
  SearchDialogInput,
  SearchDialogList,
  SearchDialogOverlay,
} from 'fumadocs-ui/components/dialog/search';
import type { SearchLink, SharedProps } from 'fumadocs-ui/contexts/search';

// Search runs in the browser: /api/search is a static file (the exported
// index, built with the site), downloaded the first time the dialog opens.
const client = staticClient({ from: '/api/search' });

export function SearchDialog({ links = [], ...props }: SharedProps & { links?: SearchLink[] }) {
  const search = useDocsSearch({ client });
  const defaultItems = useMemo(
    () =>
      links.length === 0
        ? null
        : links.map(([name, url]) => ({ type: 'page' as const, id: name, content: name, url })),
    [links],
  );

  return (
    <Dialog {...search} {...props}>
      <SearchDialogOverlay />
      <SearchDialogContent>
        <SearchDialogHeader>
          <SearchDialogIcon />
          <SearchDialogInput />
          <SearchDialogClose />
        </SearchDialogHeader>
        <SearchDialogList defaultItems={defaultItems} />
      </SearchDialogContent>
    </Dialog>
  );
}
