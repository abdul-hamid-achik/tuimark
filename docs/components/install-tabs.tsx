'use client';

import { useId, useRef, useState, type KeyboardEvent, type ReactNode } from 'react';
import { CommandCopy } from './command-copy';
import { INSTALL, NPM_PUBLISHED, REPO_URL } from '@/lib/site';

type Method = 'brew' | 'go' | 'npm' | 'release' | 'library';

const LABELS: Record<Method, { label: string; note: string }> = {
  brew: { label: 'homebrew', note: 'macOS, Linux' },
  go: { label: 'go install', note: 'Go 1.22+' },
  npm: { label: 'npm', note: NPM_PUBLISHED ? 'Bun, Node' : 'coming soon' },
  release: { label: 'release archive', note: 'no toolchain' },
  library: { label: 'go library', note: 'embed it' },
};

const ARCHIVE = [
  'shasum -a 256 -c checksums.txt --ignore-missing',
  'tar -xzf tuimark_<version>_<Os>_<arch>.tar.gz tuimark',
  'mkdir -p "$HOME/.local/bin"',
  'install -m 0755 tuimark "$HOME/.local/bin/tuimark"',
].join('\n');

function Panel({ method, Heading }: { method: Method; Heading: 'h2' | 'h3' }) {
  switch (method) {
    case 'brew':
      return (
        <Body heading="Install a release with Homebrew" Heading={Heading}>
          <p>The cask picks the right build for macOS or Linux on Apple Silicon, Intel, or ARM64. The tap is added for you.</p>
          <CommandCopy command={INSTALL.brew} />
          <Steps />
        </Body>
      );
    case 'go':
      return (
        <Body heading="Install the CLI with Go" Heading={Heading}>
          <p>
            One static binary, no CGO. It lands in <code>$(go env GOPATH)/bin</code>; make sure that is on your{' '}
            <code>PATH</code>.
          </p>
          <CommandCopy command={INSTALL.go} />
          <Steps />
        </Body>
      );
    case 'npm':
      return NPM_PUBLISHED ? (
        <Body heading="Install with npm" Heading={Heading}>
          <p>
            <code>{INSTALL.npmPackage}</code> is the CLI and the TypeScript client in one package. Its{' '}
            <code>tuimark</code> command downloads the binary of its own release once, checks it against a checksum
            built into the package, and caches it. Install it globally, or run it with <code>npx</code>.
          </p>
          <CommandCopy command={INSTALL.npmGlobal} />
          <CommandCopy command={INSTALL.npx} />
          <p>
            For a TypeScript host, scaffold a project with a Bun parent, or add the package to your own and import{' '}
            <code>spawnHost</code> from <code>{INSTALL.npmPackage}</code>.
          </p>
          <CommandCopy command={INSTALL.npmNew} />
          <CommandCopy command={INSTALL.npmInstall} />
          <CommandCopy command={INSTALL.npmImport} prompt="" />
        </Body>
      ) : (
        <Body heading="npm: coming soon" Heading={Heading}>
          <p>
            <code>{INSTALL.npmPackage}</code> is the TypeScript client and the CLI in one package. Its first publish
            comes with the next release; until then, install the CLI with Homebrew or Go.
          </p>
          <CommandCopy command={INSTALL.npmGlobal} copyable={false} />
          <CommandCopy command={INSTALL.npmInstall} copyable={false} />
        </Body>
      );
    case 'release':
      return (
        <Body heading="Download a prebuilt binary" Heading={Heading}>
          <p>
            Every release ships archives for macOS, Linux, and Windows on x86_64 and arm64, named like{' '}
            <code>tuimark_0.5.0_Darwin_arm64.tar.gz</code> (<code>.zip</code> on Windows), with a{' '}
            <code>checksums.txt</code>. Check the download, unpack it, and put the binary on your <code>PATH</code>.
          </p>
          <CommandCopy command={ARCHIVE} prompt="" />
          <a className="install-link" href={`${REPO_URL}/releases/latest`}>
            Browse the latest release <span aria-hidden="true">→</span>
          </a>
        </Body>
      );
    case 'library':
      return (
        <Body heading="Add the runtime to a Go program" Heading={Heading}>
          <p>
            The package <code>github.com/abdul-hamid-achik/tuimark</code> loads a document, binds your data, and runs
            it in the terminal.
          </p>
          <CommandCopy command={INSTALL.goLibrary} />
        </Body>
      );
  }
}

function Body({ heading, Heading, children }: { heading: string; Heading: 'h2' | 'h3'; children: ReactNode }) {
  return (
    <div className="install-method">
      <Heading className="install-heading">{heading}</Heading>
      {children}
    </div>
  );
}

function Steps() {
  return (
    <ol className="install-steps">
      <li>
        <span>01</span>
        <div>
          <strong>Check it</strong>
          <code>tuimark version</code>
        </div>
      </li>
      <li>
        <span>02</span>
        <div>
          <strong>Start a project</strong>
          <code>tuimark new my-app</code>
        </div>
      </li>
      <li>
        <span>03</span>
        <div>
          <strong>Brief your agent</strong>
          <code>tuimark agents</code>
        </div>
      </li>
    </ol>
  );
}

/**
 * The ways to install Tuimark, as tabs. The landing page shows the three
 * package managers; the getting-started guide adds the release archive and
 * the Go library.
 */
export function InstallTabs({
  methods = ['brew', 'go', 'npm'],
  headingLevel = 'h3',
}: {
  methods?: Method[];
  headingLevel?: 'h2' | 'h3';
}) {
  const [active, setActive] = useState<Method>(methods[0]);
  const id = useId();
  const tabs = useRef<(HTMLButtonElement | null)[]>([]);

  function onKeyDown(e: KeyboardEvent, i: number) {
    const step = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0;
    if (!step) return;
    e.preventDefault();
    const next = (i + step + methods.length) % methods.length;
    setActive(methods[next]);
    tabs.current[next]?.focus();
  }

  return (
    <div className="install-tabs not-prose">
      <div className="install-tablist" role="tablist" aria-label="Ways to install Tuimark">
        {methods.map((m, i) => (
          <button
            key={m}
            ref={(el) => {
              tabs.current[i] = el;
            }}
            id={`${id}-tab-${m}`}
            type="button"
            role="tab"
            aria-selected={active === m}
            aria-controls={`${id}-panel`}
            tabIndex={active === m ? 0 : -1}
            className={active === m ? 'is-active' : undefined}
            onClick={() => setActive(m)}
            onKeyDown={(e) => onKeyDown(e, i)}
          >
            <span>
              {active === m ? '▸' : ' '}
              {LABELS[m].label}
            </span>
            <small>{LABELS[m].note}</small>
          </button>
        ))}
      </div>
      <div id={`${id}-panel`} className="install-panel" role="tabpanel" aria-labelledby={`${id}-tab-${active}`}>
        <Panel method={active} Heading={headingLevel} />
      </div>
    </div>
  );
}

/** The install panel of the getting-started guide: every method. */
export function InstallPanel({ headingLevel = 'h3' }: { headingLevel?: 'h2' | 'h3' }) {
  return <InstallTabs methods={['brew', 'go', 'npm', 'release', 'library']} headingLevel={headingLevel} />;
}
