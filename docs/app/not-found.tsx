import type { Metadata } from 'next';
import Link from 'next/link';
import { HomeLayout } from 'fumadocs-ui/layouts/home';
import { baseOptions } from '@/lib/layout.shared';

export const metadata: Metadata = {
  title: 'Page not found',
  robots: { index: false },
};

export default function NotFound() {
  return (
    <HomeLayout {...baseOptions()}>
      <main className="not-found">
        <div className="tf not-found-frame">
          <span className="tf-title">
            <b>404</b> no such screen
          </span>
          <p className="not-found-code" aria-hidden="true">
            error B005 #page: no node at this path
          </p>
          <h1>Nothing is laid out here.</h1>
          <p>The page moved or never existed. Switch to a screen that does:</p>
          <ul className="not-found-links">
            <li>
              <Link href="/">▸ home</Link>
            </li>
            <li>
              <Link href="/guide/getting-started">▸ getting started</Link>
            </li>
            <li>
              <Link href="/reference">▸ reference</Link>
            </li>
            <li>
              <Link href="/examples">▸ examples</Link>
            </li>
          </ul>
          <p className="not-found-hint">
            <kbd>⌘</kbd> <kbd>K</kbd> searches the docs.
          </p>
        </div>
      </main>
    </HomeLayout>
  );
}
