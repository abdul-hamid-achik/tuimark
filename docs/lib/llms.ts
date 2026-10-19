import type * as PageTree from 'fumadocs-core/page-tree';
import { pageMarkdown, source, type DocPage } from './source';
import { DESCRIPTION, INSTALL, NPM_PUBLISHED, REPO_URL, SITE_NAME, absoluteUrl } from './site';

/** The docs pages section by section, in sidebar order. */
export function pagesBySection(): { section: string; pages: DocPage[] }[] {
  const out: { section: string; pages: DocPage[] }[] = [];
  for (const node of source.getPageTree().children) {
    if (node.type !== 'folder') continue;
    const pages: DocPage[] = [];
    const seen = new Set<string>();
    const add = (url: string) => {
      if (seen.has(url)) return;
      const page = source.getPages().find((p) => p.url === url);
      if (page) {
        seen.add(url);
        pages.push(page);
      }
    };
    const walk = (n: PageTree.Node) => {
      if (n.type === 'page' && !n.external) add(n.url);
      else if (n.type === 'folder') {
        if (n.index) add(n.index.url);
        n.children.forEach(walk);
      }
    };
    walk(node);
    out.push({ section: String(node.name), pages });
  }
  return out;
}

/** /llms.txt, following the llms.txt convention (llmstxt.org). */
export function llmsIndex(): string {
  const lines = [
    `# ${SITE_NAME}`,
    '',
    `> ${DESCRIPTION}`,
    '',
    'Tuimark is a closed vocabulary: structure in `.tui` (XML), style in `.tcss` (a reduced CSS), data as JSON, behavior as named actions. Never invent a tag, attribute or property; the reference pages list every one. The authoring loop is `tuimark validate`, then `tuimark dump --format json` at 80 and 120 columns, then `tuimark play` for interactions. `tuimark agents` prints a full briefing (AGENTS.md) generated from the installed binary, and `tuimark mcp` serves the loop as MCP tools.',
    '',
    NPM_PUBLISHED
      ? `Install: \`${INSTALL.brew}\`, \`${INSTALL.go}\`, or \`${INSTALL.npmGlobal}\` (or run \`${INSTALL.npx}\`); for a TypeScript project, \`${INSTALL.npmNew}\`.`
      : `Install: \`${INSTALL.brew}\` or \`${INSTALL.go}\`.`,
    '',
    'Every page below is linked as Markdown; drop the `.md` for the HTML page.',
  ];
  for (const { section, pages } of pagesBySection()) {
    lines.push('', `## ${section}`, '');
    for (const p of pages) {
      const desc = p.data.description ? `: ${p.data.description}` : '';
      lines.push(`- [${p.data.title}](${absoluteUrl(`${p.url}.md`)})${desc}`);
    }
  }
  lines.push(
    '',
    '## Optional',
    '',
    `- [Everything above in one file](${absoluteUrl('/llms-full.txt')}): every page of the guide, the reference and the examples, as Markdown`,
    `- [Source code](${REPO_URL}): the Go runtime, the CLI, the kit, and the examples`,
    `- [Changelog](${REPO_URL}/blob/main/CHANGELOG.md): release history`,
    '',
  );
  return lines.join('\n');
}

/** /llms-full.txt: every page as Markdown, in sidebar order. */
export async function llmsFull(): Promise<string> {
  const parts = [
    `# ${SITE_NAME}: the complete documentation`,
    '',
    `> ${DESCRIPTION}`,
    '',
    `Source: ${absoluteUrl('/')}. The index of these pages is ${absoluteUrl('/llms.txt')}.`,
  ];
  for (const { pages } of pagesBySection()) {
    for (const page of pages) parts.push('', '---', '', await pageMarkdown(page));
  }
  return parts.join('\n');
}
