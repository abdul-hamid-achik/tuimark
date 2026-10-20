import { DocsLayout } from 'fumadocs-ui/layouts/docs';
import { baseOptions } from '@/lib/layout.shared';
import { source } from '@/lib/source';

export default function Layout({ children }: LayoutProps<'/'>) {
  return (
    // Each top folder (guide, reference, examples) is its own sidebar; the
    // links in the nav move between them, so the sidebar needs no switcher.
    <DocsLayout tree={source.getPageTree()} tabs={false} {...baseOptions()}>
      {children}
    </DocsLayout>
  );
}
