import type { Metadata } from 'next';
import Link from 'next/link';
import { ServerCodeBlock } from 'fumadocs-ui/components/codeblock.rsc';
import { JsonLd } from '@/components/json-ld';
import { TermShot } from '@/components/term-shot';
import { KIT_GROUPS, KIT_PAGE, groupId, type KitComponent } from '@/lib/kit';
import { pageMetadata } from '@/lib/metadata';
import { REPO_URL, SITE_NAME, SITE_URL, absoluteUrl } from '@/lib/site';
import '../pages.css';

export const metadata: Metadata = pageMetadata({
  title: KIT_PAGE.title,
  description: KIT_PAGE.description,
  path: '/kit',
  image: { url: '/og/kit/image.png', alt: `${SITE_NAME} kit components` },
});

const THEMES = { light: 'github-light-default', dark: 'github-dark-default' } as const;

const HEAD = `<tui theme="auto">
  <include src="kit/kit.tui"/>  <!-- the kit: sheets, keys, templates -->
  <style src="app.tcss"/>       <!-- yours, after it: token overrides -->
  <screen id="main">
    …
    <use template="k-alert" class="warn" title="Quota" icon="!" text="92% of the build minutes used."/>
  </screen>
</tui>`;

const GET = `git clone --depth 1 https://github.com/abdul-hamid-achik/tuimark
mkdir -p my-app/kit
cp tuimark/kit/kit.tui tuimark/kit/theme.tcss tuimark/kit/kit.tcss my-app/kit/`;

/** `code` spans in a one-line summary, as the kit guide writes them. */
function inline(text: string) {
  return text.split('`').map((part, i) => (i % 2 ? <code key={i}>{part}</code> : part));
}

/** The contract comment's parts, leaving out the ones that are "none". */
function Contract({ c }: { c: KitComponent }) {
  const rows: [string, string][] = [
    ['takes', c.contract],
    ['actions', c.actions],
    ['keys', c.keys],
    ['needs', c.requires],
  ];
  return (
    <dl className="kit-contract">
      {rows
        .filter(([, v]) => v && v !== 'none')
        .map(([k, v]) => (
          <div key={k}>
            <dt>{k}</dt>
            <dd>{v}</dd>
          </div>
        ))}
    </dl>
  );
}

function Component({ c }: { c: KitComponent }) {
  return (
    <section className="kit-item" id={c.name} aria-labelledby={`${c.name}-title`}>
      <h3 id={`${c.name}-title`}>
        <a href={`#${c.name}`}>{c.title}</a>
        <code>{c.cls}</code>
      </h3>
      <p className="pg-meta kit-line">
        {inline(c.line)}
      </p>
      <TermShot name={c.shot} />
      <Contract c={c} />
      {c.template && (
        <details className="kit-markup">
          <summary>Template: {c.cls} in kit.tui</summary>
          <ServerCodeBlock
            code={c.template.replace(/\n+$/, '')}
            lang="xml"
            themes={THEMES}
            defaultColor={false}
            codeblock={{ title: 'kit/kit.tui' }}
          />
        </details>
      )}
      <details className="kit-markup">
        <summary>
          {c.template ? 'In use' : 'Markup'}: {c.file.replace(/^kit\//, '')}
        </summary>
        <ServerCodeBlock
          code={c.source.replace(/\n+$/, '')}
          lang="xml"
          themes={THEMES}
          defaultColor={false}
          codeblock={{ title: c.file }}
        />
      </details>
    </section>
  );
}

export default function KitPage() {
  const count = KIT_GROUPS.reduce((n, g) => n + g.components.length, 0);
  return (
    <div className="pg">
      <JsonLd
        data={[
          {
            '@context': 'https://schema.org',
            '@type': 'CollectionPage',
            name: KIT_PAGE.title,
            description: KIT_PAGE.description,
            url: absoluteUrl('/kit'),
            inLanguage: 'en',
            isPartOf: { '@type': 'WebSite', name: SITE_NAME, url: SITE_URL },
            hasPart: KIT_GROUPS.flatMap((g) =>
              g.components.map((c) => ({
                '@type': 'SoftwareSourceCode',
                name: `${c.title} (${c.cls})`,
                description: c.line.replace(/`/g, ''),
                codeRepository: REPO_URL,
                url: `${REPO_URL}/blob/main/${c.file}`,
                programmingLanguage: 'Tuimark',
              })),
            ),
          },
          {
            '@context': 'https://schema.org',
            '@type': 'BreadcrumbList',
            itemListElement: [
              { '@type': 'ListItem', position: 1, name: SITE_NAME, item: SITE_URL },
              { '@type': 'ListItem', position: 2, name: 'Kit', item: absoluteUrl('/kit') },
            ],
          },
        ]}
      />
      <div className="pg-shell">
        <header className="pg-head">
          <p className="pg-kicker">the kit</p>
          <h1>{KIT_PAGE.title}</h1>
          <p className="pg-lede">
            {count} components you copy into your project and own, in the spirit of shadcn: no package, no new tags,
            every color a token. Each picture is <code>tuimark play</code> on the component’s own document in{' '}
            <code>kit/components</code>, with its sample data, in the kit’s dark or light theme to match this site. The{' '}
            <Link href="/guide/kit">kit guide</Link> explains the conventions and the host helpers.
          </p>
        </header>

        <div className="kit-start">
          <div className="tf">
            <span className="tf-title">
              <b>1</b> copy the kit
            </span>
            <p>
              <code>kit.tui</code> holds a template per component and the overlays’ keys, and loads{' '}
              <code>theme.tcss</code> (the dark and light tokens) and <code>kit.tcss</code> (the <code>k-</code>{' '}
              classes) from its own folder. Keep the three files together.
            </p>
            <ServerCodeBlock code={GET} lang="sh" themes={THEMES} defaultColor={false} />
          </div>
          <div className="tf">
            <span className="tf-title">
              <b>2</b> include it, then use a component
            </span>
            <p>
              Include the kit first and your own sheet after it, so a token you re-declare rebrands the whole kit. Use a
              template with <code>&lt;use&gt;</code>, passing literal text as props, your data with <code>with</code>, a
              body as the content between <code>&lt;use&gt;</code> and <code>&lt;/use&gt;</code>, and a variant with{' '}
              <code>class</code>; a class component’s markup is below, to copy. Each one’s contract says which props,
              store paths, actions, and keys it takes.
            </p>
            <ServerCodeBlock code={HEAD} lang="xml" themes={THEMES} defaultColor={false} />
          </div>
        </div>

        <nav aria-label="Component groups">
          <ul className="kit-toc">
            {KIT_GROUPS.map((g) => (
              <li key={g.group}>
                <a href={`#${groupId(g.group)}`}>
                  {g.group} <span aria-hidden="true">·</span> {g.components.length}
                </a>
              </li>
            ))}
            <li>
              <Link href="/examples#kit-gallery">The gallery app →</Link>
            </li>
          </ul>
        </nav>

        {KIT_GROUPS.map((g) => (
          <section key={g.group} className="kit-group" id={groupId(g.group)} aria-labelledby={`${groupId(g.group)}-title`}>
            <h2 id={`${groupId(g.group)}-title`}>{g.group}</h2>
            <div className="kit-grid">
              {g.components.map((c) => (
                <Component key={c.name} c={c} />
              ))}
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}
