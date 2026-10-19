import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import type { Metadata, Route } from 'next';
import Link from 'next/link';
import catalog from '@/lib/generated/catalog.json';
import { code } from '@/lib/highlight';
import { DESCRIPTION, HOME_TITLE, INSTALL, REPO_URL, SITE_NAME, SITE_URL, VERSION, AUTHOR, absoluteUrl } from '@/lib/site';
import { pageMetadata } from '@/lib/metadata';
import { CodeTabs } from '@/components/landing/code-tabs';
import { CommandCopy } from '@/components/command-copy';
import { InstallTabs } from '@/components/install-tabs';
import { JsonLd } from '@/components/json-ld';
import { TermShot } from '@/components/term-shot';
import './landing.css';

export const metadata: Metadata = pageMetadata({ title: HOME_TITLE, absoluteTitle: true, description: DESCRIPTION, path: '/' });

// Everything on this page is read at build time: the hero is the files in
// snippets/hero and the output docs/scripts/generate.mjs recorded for them.
const snippet = (path: string) => readFileSync(join(process.cwd(), 'snippets', path), 'utf8');

const hero = {
  tui: snippet('hero/app.tui'),
  tcss: snippet('hero/theme.tcss'),
  json: snippet('hero/sample.json'),
  events: snippet('out/hero-events.txt').trimEnd().split('\n'),
  dump: JSON.parse(snippet('out/hero-dump.json')) as {
    cols: number;
    rows: number;
    ok: boolean;
    focus: string;
    errors: unknown[];
    nodes: Record<string, unknown>[];
    grid: string[];
  },
};

// One node per line, the named ones and the selected row: the geometry an
// agent reads back.
function dumpExcerpt() {
  const d = hero.dump;
  const node = (n: Record<string, unknown>) => JSON.stringify(n).replace(/,"/g, ', "').replace(/":/g, '": ');
  const shown = d.nodes.filter((n) => n.id || n.selected);
  return [
    '{',
    `  "cols": ${d.cols}, "rows": ${d.rows}, "ok": ${d.ok}, "focus": ${JSON.stringify(d.focus)}, "errors": [],`,
    '  "nodes": [',
    ...shown.map((n) => `    ${node(n)},`),
    `    … ${d.nodes.length - shown.length} more`,
    '  ],',
    `  "grid": ["${d.grid[0].slice(0, 26)}…", … ${d.grid.length - 1} more rows]`,
    '}',
  ].join('\n');
}

const tagCount = catalog.tags.length + catalog.tagsV2.length;

// The closed vocabulary, grouped by role.
const roles: Record<string, string[]> = {
  document: ['tui', 'style', 'keymap', 'bind', 'screen'],
  layout: ['col', 'row', 'box', 'scroll', 'spacer', 'tabs', 'tab'],
  content: ['text', 'rule', 'sparkline', 'hints'],
  interactive: ['list', 'item', 'input', 'button', 'progress', 'table', 'column'],
  overlay: ['modal'],
};
const allTags = [...catalog.tags, ...catalog.tagsV2];
const grouped = new Set(Object.values(roles).flat());
const vocabulary = [
  ...Object.entries(roles).map(([group, tags]) => ({ group, tags: tags.filter((t) => allTags.includes(t)) })),
  { group: 'more', tags: allTags.filter((t) => !grouped.has(t)) },
].filter((g) => g.tags.length > 0);

const goHost = `ui, err := tuimark.Load("app.tui")
if err != nil {
	log.Fatal(err)
}
ui.Bind("", fleet.Snapshot())

ui.On("show", func(ev tuimark.Event) error {
	return ui.Set("current", fleet.Detail(ev.Keys["s"]))
})
ui.On("deploy", func(ev tuimark.Event) error {
	go fleet.Deploy(ev.Keys["s"], func(pct int) {
		ui.Set("current.rollout", pct)
	})
	return nil
})

log.Fatal(ui.Run(os.Stdout))`;

const tsHost = `import { spawnHost } from "@thelacanians/tuimark";

const ui = await spawnHost("app.tui", { data: fleet.snapshot() });

ui.on("show", async (ev) => {
  await ui.set("current", await fleet.detail(ev.keys.s));
});
ui.on("deploy", (ev) => {
  fleet.deploy(ev.keys.s, (pct) => ui.set("current.rollout", pct));
});

process.exit((await ui.exited).code);`;

const protocol = `# tuimark host app.tui    fd 4: tuimark -> you, fd 3: you -> tuimark
<- {"type":"ready","protocol":1,"cols":72,"rows":16,"theme":"dark",…}
<- {"type":"event","seq":1,"action":"show","source":"services","keys":{"s":"auth"},"value":null}
-> {"type":"set","path":"current","value":{"name":"auth","status":"● healthy",…}}
-> {"type":"reply","seq":1}
<- {"type":"event","seq":2,"action":"deploy","source":"services","keys":{"s":"billing"},"value":null}
-> {"type":"reply","seq":2}
-> {"type":"set","path":"current.rollout","value":41}`;

const mcpConfig = `{
  "mcpServers": {
    "tuimark": { "command": "tuimark", "args": ["mcp"] }
  }
}`;

const loop = [
  {
    verb: 'validate',
    cmd: 'tuimark validate app.tui --data sample.json',
    note: 'Every problem is a code with a position: V for parse, L for layout, B for bindings.',
  },
  {
    verb: 'dump',
    cmd: 'tuimark dump app.tui --data sample.json --cols 72 --format json',
    note: 'The frame as data: the text grid, every node’s rectangle, focus, and the errors.',
  },
  {
    verb: 'play',
    cmd: 'tuimark play app.tui --data sample.json --input "down enter"',
    note: 'Replay keys, typing, and clicks with no terminal, and read which actions fired.',
  },
];

// The gallery: the example apps, most visual first. Every tile follows the
// site's light or dark mode, so inbox, dashboard and unicode, whose documents
// paint their own dark palette, are left to /examples; the kit gallery shows
// its data tab here (its basics tab is in the kit section). Eighteen keep the
// grid even at three columns and at two. Animated demos start paused, so the
// page loads only their still frames.
const gallery: { shot: string; title: string; anchor: string; text: string }[] = [
  { shot: 'monitor-processes', title: 'monitor studio', anchor: 'monitor-studio', text: 'Nine tabs, a sortable process table with multi-select, gauges and sparklines.' },
  { shot: 'containers-demo', title: 'containers', anchor: 'containers', text: 'A container dashboard built on the kit: a filterable table, a detail pane, dialogs, and a palette.' },
  { shot: 'agent', title: 'agent', anchor: 'agent', text: 'A coding-agent chat: transcript, composer, tool activity, and approvals.' },
  { shot: 'gitui-status', title: 'gitui', anchor: 'gitui', text: 'Staged and unstaged files beside a colored diff, a log, and commit details.' },
  { shot: 'http-client', title: 'http', anchor: 'http', text: 'A request collection, an editor, and the response with its headers.' },
  { shot: 'logs-live', title: 'logs', anchor: 'logs', text: 'A live tail with per-level counts and a lines-per-second sparkline.' },
  { shot: 'tasks-board', title: 'tasks', anchor: 'tasks', text: 'A kanban board with lanes, priorities, tags, and cards you move together.' },
  { shot: 'pipeline-demo', title: 'pipeline', anchor: 'pipeline', text: 'CI jobs in parallel with spinners and progress, a failure, and a retry.' },
  { shot: 'music-demo', title: 'music', anchor: 'music', text: 'A player with a spectrum visualizer, a position bar, and a queue.' },
  { shot: 'kit-gallery-data', title: 'kit gallery', anchor: 'kit-gallery', text: 'Every kit component on five tabs; here a focused data table over a build log.' },
  { shot: 'snake-demo', title: 'snake', anchor: 'snake', text: 'The classic, on an autopilot, with the score and speed climbing.' },
  { shot: 'tetris-demo', title: 'tetris', anchor: 'tetris', text: 'Falling pieces, line clears, and a next-piece preview.' },
  { shot: 'chess-demo', title: 'chess', anchor: 'chess', text: 'A famous game replayed move by move, with the move list.' },
  { shot: '2048-demo', title: '2048', anchor: '2048', text: 'Sliding tiles that merge and pop, and a climbing score.' },
  { shot: 'minesweeper-demo', title: 'minesweeper', anchor: 'minesweeper', text: 'A solver flagging mines and opening safe cells.' },
  { shot: 'sudoku-demo', title: 'sudoku', anchor: 'sudoku', text: 'A logic solver filling a puzzle digit by digit.' },
  { shot: 'pomodoro-demo', title: 'pomodoro', anchor: 'pomodoro', text: 'Big block digits counting down a work session.' },
  { shot: 'spike', title: 'spike', anchor: 'spike', text: 'Where it started: five tags, two bordered boxes, and a status line.' },
];

function Section({
  id,
  kicker,
  title,
  children,
  lede,
}: {
  id: string;
  kicker: string;
  title: string;
  lede: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section id={id} className="l-shell l-section" aria-labelledby={`${id}-title`}>
      <header className="l-head">
        <p className="l-kicker">{kicker}</p>
        <h2 id={`${id}-title`}>{title}</h2>
        <p className="l-lede">{lede}</p>
      </header>
      {children}
    </section>
  );
}

function More({ links }: { links: [string, string][] }) {
  return (
    <p className="l-more">
      {links.map(([href, label]) => (
        <Link key={href} href={href as Route}>
          {label} <span aria-hidden="true">→</span>
        </Link>
      ))}
    </p>
  );
}

export default async function HomePage() {
  const [tuiHtml, tcssHtml, jsonHtml, goHtml, tsHtml, protocolHtml, mcpHtml, dumpHtml] = await Promise.all([
    code(hero.tui, 'xml'),
    code(hero.tcss, 'css'),
    code(hero.json, 'json'),
    code(goHost, 'go'),
    code(tsHost, 'ts'),
    code(protocol, 'sh'),
    code(mcpConfig, 'json'),
    code(dumpExcerpt(), 'json'),
  ]);

  const jsonLd = {
    '@context': 'https://schema.org',
    '@type': 'SoftwareApplication',
    name: SITE_NAME,
    alternateName: 'tuimark',
    description: DESCRIPTION,
    url: SITE_URL,
    image: absoluteUrl('/opengraph-image'),
    applicationCategory: 'DeveloperApplication',
    applicationSubCategory: 'Terminal UI framework',
    operatingSystem: 'macOS, Linux, Windows',
    softwareVersion: VERSION.replace(/^v/, ''),
    license: 'https://opensource.org/licenses/MIT',
    isAccessibleForFree: true,
    offers: { '@type': 'Offer', price: '0', priceCurrency: 'USD' },
    author: { '@type': 'Person', name: AUTHOR, url: 'https://github.com/abdul-hamid-achik' },
    codeRepository: REPO_URL,
    downloadUrl: `${REPO_URL}/releases/latest`,
    installUrl: absoluteUrl('/guide/getting-started'),
    programmingLanguage: ['Go', 'TypeScript'],
  };

  return (
    <main className="landing">
      <JsonLd data={jsonLd} />

      {/* ── Hero ───────────────────────────────────────────────────────── */}
      <section className="l-shell l-hero" aria-labelledby="hero-title">
        <div className="l-hero-copy">
          <p className="l-eyebrow">
            <span aria-hidden="true">▌</span> {VERSION} · open source · MIT
          </p>
          <h1 id="hero-title">Terminal UIs as documents.</h1>
          <p className="l-pitch">
            Write <code>.tui</code> and <code>.tcss</code>, let any language drive it, let agents design it.
          </p>
          <p className="l-sub">
            Structure is markup, style is a small CSS, data is JSON, and behavior is named actions. One runtime lays it
            out on a cell grid, paints it, and prints any frame as JSON, so a coding agent can check what it built
            without seeing a screen.
          </p>
          <div className="l-actions">
            <Link className="l-btn is-primary" href="/guide/getting-started">
              Get started <span aria-hidden="true">→</span>
            </Link>
            <Link className="l-btn" href="/guide/agents">
              Use it with AI agents
            </Link>
            <a className="l-btn" href={REPO_URL}>
              GitHub
            </a>
          </div>
        </div>

        <div className="l-demo">
          <CodeTabs
            className="l-editor"
            label="The three files of the demo"
            tabs={[
              { name: 'app.tui', note: 'structure' },
              { name: 'theme.tcss', note: 'style' },
              { name: 'sample.json', note: 'data' },
            ]}
          >
            {tuiHtml}
            {tcssHtml}
            {jsonHtml}
          </CodeTabs>
          <div className="l-run">
            <TermShot name="hero" eager className="l-hero-shot" />
            <div className="tf l-events">
              <span className="tf-title">
                events <small>what your host receives</small>
              </span>
              <ol>
                {hero.events.map((line) => {
                  const [step, action, source, ...rest] = line.split(' ');
                  return (
                    <li key={line}>
                      <span className="ev-step">{step}</span> <b>{action}</b> <span className="ev-src">{source}</span>{' '}
                      <code>{rest.join(' ')}</code>
                    </li>
                  );
                })}
              </ol>
            </div>
          </div>
        </div>
        <p className="l-demo-note">
          The terminal is <code>tuimark play</code> replaying <kbd>↓</kbd> <kbd>↓</kbd> <kbd>enter</kbd> against these
          three files. The list fires <code>show</code>, the keymap fires <code>deploy</code>, and the script answers
          them the way a host would, with <code>set</code>.
        </p>
      </section>

      {/* ── Install ───────────────────────────────────────────────────── */}
      <section id="install" className="l-shell l-install" aria-labelledby="install-title">
        <h2 id="install-title" className="l-install-title">
          Install
        </h2>
        <InstallTabs headingLevel="h3" />
      </section>

      {/* ── Closed vocabulary ─────────────────────────────────────────── */}
      <Section
        id="vocabulary"
        kicker="01 · a closed vocabulary"
        title="Small enough for an agent to hold."
        lede={
          <>
            {tagCount} tags, {catalog.properties} CSS properties, and {catalog.diagnostics} diagnostic codes, and nothing
            else. An invented tag is <code>V001</code>, not a new widget; an invented property is <code>V003</code>.
            There are no expressions in markup: computed values come from the data.
          </>
        }
      >
        <div className="l-split">
          <div className="l-vocab">
            {vocabulary.map((g) => (
              <div key={g.group} className="tf">
                <span className="tf-title">{g.group}</span>
                <ul>
                  {g.tags.map((t) => (
                    <li key={t}>&lt;{t}&gt;</li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
          <div className="tf l-briefing">
            <span className="tf-title">tuimark agents &gt; AGENTS.md</span>
            <p>
              The whole language, as a briefing your agent reads before it writes a line: what to edit, the loop, every
              tag and the attributes it takes, every property and its values, the key tokens, every diagnostic.
            </p>
            <p>
              It is generated from the runtime’s own catalogs, so it always matches the binary that printed it. Editors
              and tools read the same vocabulary as JSON from <code>tuimark catalog</code>.
            </p>
            <CommandCopy command="tuimark agents > AGENTS.md" />
          </div>
        </div>
        <More
          links={[
            ['/reference/tags', 'Tags and attributes'],
            ['/reference/css', 'CSS properties'],
            ['/reference/diagnostics', 'Diagnostics'],
          ]}
        />
      </Section>

      {/* ── The loop ──────────────────────────────────────────────────── */}
      <Section
        id="loop"
        kicker="02 · the loop"
        title="Validate, dump, play. Repeat."
        lede={
          <>
            An agent cannot look at a terminal, so Tuimark gives it the frame as data. It edits the files, renders them
            at 80 and 120 columns, reads back exactly what is on screen, and replays the keys a person would press.
          </>
        }
      >
        <div className="l-loop">
          <ol className="l-steps">
            {loop.map((s, i) => (
              <li key={s.verb} className="tf">
                <span className="tf-title">
                  <b>{i + 1}</b> {s.verb}
                </span>
                <code className="l-step-cmd">
                  <span aria-hidden="true">$ </span>
                  {s.cmd}
                </code>
                <p>{s.note}</p>
              </li>
            ))}
          </ol>
          <div className="l-loop-out">
            <div className="tf l-code">
              <span className="tf-title">
                dump --format json <small>excerpt</small>
              </span>
              {dumpHtml}
            </div>
            <div className="tf l-code">
              <span className="tf-title">
                play: events <small>step · action · source · keys · value</small>
              </span>
              <pre className="l-plain">{hero.events.join('\n')}</pre>
            </div>
          </div>
        </div>
        <More
          links={[
            ['/guide/agents#the-loop', 'The authoring loop'],
            ['/guide/tools', 'Dumps and tools'],
            ['/guide/testing', 'Golden tests'],
          ]}
        />
      </Section>

      {/* ── Hosts ─────────────────────────────────────────────────────── */}
      <Section
        id="hosts"
        kicker="03 · any language"
        title="Your program answers actions. Tuimark does the rest."
        lede={
          <>
            The host binds data and handles the action names the document fires. Layout, focus, keys, the mouse, colors,
            and the terminal stay in the runtime, whether the host is Go, TypeScript, or anything that writes JSON lines.
          </>
        }
      >
        <div className="l-split l-hosts">
          <CodeTabs
            label="Host languages"
            tabs={[
              { name: 'main.go', note: 'Go' },
              { name: 'parent.ts', note: 'TypeScript' },
              { name: 'tuimark host', note: 'any language' },
            ]}
          >
            {goHtml}
            {tsHtml}
            {protocolHtml}
          </CodeTabs>
          <div className="tf l-division">
            <span className="tf-title">who does what</span>
            <h3>The runtime</h3>
            <ul className="l-checks">
              <li>layout, sizes, wrapping, and truncation</li>
              <li>focus, the keymap, and the mouse</li>
              <li>colors, light and dark themes</li>
              <li>terminal modes, resizes, and redraws</li>
            </ul>
            <h3>Your host</h3>
            <ul className="l-checks is-host">
              <li>binds the data</li>
              <li>answers the actions the document names</li>
              <li>writes changes back with set</li>
            </ul>
            <p>
              In Go it is a library; from Bun, <code>@thelacanians/tuimark</code>; from anything else,{' '}
              <code>tuimark host</code> speaks JSON lines on two file descriptors.
            </p>
          </div>
        </div>
        <More
          links={[
            ['/guide/go-api', 'Go API'],
            ['/guide/typescript', 'TypeScript client'],
            ['/guide/host-protocol', 'Host protocol'],
          ]}
        />
      </Section>

      {/* ── Kit ───────────────────────────────────────────────────────── */}
      <Section
        id="kit"
        kicker="04 · the kit"
        title="Components you copy and own."
        lede={
          <>
            Buttons, cards, alerts, a data table, a command palette, dialogs, toasts, and more, as small documents and two
            stylesheets. Like shadcn, you copy what you need into your project: no package, no new tags, every color a
            token you can rebrand. Small helpers in Go and TypeScript do the host’s part: toasts that expire, form
            validation, dialog routing, the palette’s filter.
          </>
        }
      >
        <TermShot name="kit-gallery">
          <code>examples/kit-gallery</code>: every component on five tabs, in the kit’s dark and light themes.
        </TermShot>
        <More
          links={[
            ['/kit', 'Every component, pictured'],
            ['/guide/kit', 'The kit guide'],
            ['/guide/kit#helpers', 'Helpers'],
            [`/examples#kit-gallery`, 'The gallery app'],
          ]}
        />
      </Section>

      {/* ── Agents and editors ────────────────────────────────────────── */}
      <Section
        id="tools"
        kicker="05 · agents and editors"
        title="The same loop over MCP, the same checks as you type."
        lede={
          <>
            One binary carries an MCP server for agents without a shell and a language server for your editor. Both run
            the code the CLI runs, so they never disagree with it.
          </>
        }
      >
        <div className="l-split l-tools">
          <div className="tf l-tool">
            <span className="tf-title">tuimark mcp</span>
            <p>
              Five read-only tools: <code>tuimark_validate</code>, <code>tuimark_dump</code>, <code>tuimark_play</code>,{' '}
              <code>tuimark_inspect</code>, and <code>tuimark_agents</code>. Each returns exactly what its command prints.
            </p>
            <CommandCopy command="claude mcp add tuimark -- tuimark mcp" />
            <div className="l-code l-code-inline">{mcpHtml}</div>
            <More links={[['/guide/mcp', 'MCP server']]} />
          </div>
          <div className="tf l-tool">
            <span className="tf-title">tuimark lsp · VS Code</span>
            <p>
              <code>tuimark lsp</code> shows <code>validate</code>’s diagnostics as you type, with a <code>sample.json</code>{' '}
              beside the document as data, and adds completion, hover, go to definition, and formatting from the catalog.
            </p>
            <ul className="l-checks">
              <li>VS Code extension: highlighting and snippets for .tui and .tcss</li>
              <li>an unknown tag or property is marked while you type</li>
              <li>Neovim, Helix, or any LSP client: run tuimark lsp</li>
            </ul>
            <More links={[['/guide/editors', 'Editors']]} />
          </div>
        </div>
      </Section>

      {/* ── Gallery ───────────────────────────────────────────────────── */}
      <Section
        id="examples"
        kicker="06 · examples"
        title="Real apps, the same few files."
        lede={
          <>
            Every example in the repository is a <code>.tui</code>, a <code>.tcss</code>, and sample data, with a small
            host. Every picture here is rendered from <code>tuimark play</code> output; press play on the demos.
          </>
        }
      >
        <ul className="l-gallery">
          {gallery.map((g) => (
            <li key={g.shot}>
              <TermShot name={g.shot} bare paused />
              <h3>
                <Link href={`/examples#${g.anchor}` as Route}>
                  {g.title} <span aria-hidden="true">→</span>
                </Link>
              </h3>
              <p>{g.text}</p>
            </li>
          ))}
        </ul>
        <More links={[['/examples', 'All examples']]} />
      </Section>

      {/* ── Final call to action ──────────────────────────────────────── */}
      <section className="l-shell l-cta" aria-labelledby="cta-title">
        <div className="tf l-cta-frame">
          <span className="tf-title">
            <b>▸</b> next
          </span>
          <h2 id="cta-title">Write your first screen.</h2>
          <p>
            <code>tuimark new</code> writes a project with a document, a stylesheet, sample data, and an{' '}
            <code>AGENTS.md</code>. Hand it to your agent, or open it yourself.
          </p>
          <div className="l-cta-cmds">
            <CommandCopy command={INSTALL.brew} />
            <CommandCopy command="tuimark new my-app && cd my-app && tuimark run app.tui --data sample.json" />
          </div>
          <div className="l-actions">
            <Link className="l-btn is-primary" href="/guide/getting-started">
              Read the guide <span aria-hidden="true">→</span>
            </Link>
            <Link className="l-btn" href="/reference">
              Reference
            </Link>
            <Link className="l-btn" href="/examples">
              Examples
            </Link>
          </div>
        </div>
      </section>
    </main>
  );
}
