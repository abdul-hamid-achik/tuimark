// A store and a handler table in memory, standing in for @thelacanians/tuimark in the
// helpers' tests. It is test support, not a helper: do not copy it.
import { readFileSync } from "node:fs";
import type { Host, HostEvent } from "@thelacanians/tuimark";

type Handler = (ev: HostEvent) => unknown;

export class FakeStore {
  data: Record<string, unknown> = {};
  handlers = new Map<string, Handler>();
  /** Every batch, in order; a set() is a batch of one. */
  batches: { path: string; value: unknown }[][] = [];
  private seq = 0;

  constructor(initial: Record<string, unknown> = {}) {
    this.data = structuredClone(initial);
  }

  private write(path: string, value: unknown): void {
    const segs = path.split(".");
    let cur = this.data;
    for (const seg of segs.slice(0, -1)) {
      const next = cur[seg];
      if (typeof next !== "object" || next === null || Array.isArray(next)) cur[seg] = {};
      cur = cur[seg] as Record<string, unknown>;
    }
    cur[segs[segs.length - 1]!] = structuredClone(value ?? null);
  }

  async set(path: string, value: unknown) {
    return this.batch([{ path, value }]);
  }

  async batch(writes: readonly { path: string; value: unknown }[]) {
    this.batches.push(writes.map((w) => ({ ...w })));
    for (const w of writes) this.write(w.path, w.value);
    return { id: ++this.seq, ok: true };
  }

  async get(path: string) {
    let cur: unknown = this.data;
    for (const seg of path.split(".")) {
      if (typeof cur !== "object" || cur === null || !(seg in cur)) return { found: false };
      cur = (cur as Record<string, unknown>)[seg];
    }
    return { found: true, value: structuredClone(cur) };
  }

  on(action: string, handler: Handler) {
    this.handlers.set(action, handler);
    return this;
  }

  /** The value at a path, or undefined. */
  at(path: string): unknown {
    let cur: unknown = this.data;
    for (const seg of path.split(".")) {
      if (typeof cur !== "object" || cur === null) return undefined;
      cur = (cur as Record<string, unknown>)[seg];
    }
    return cur;
  }

  /** Fires an action as the host would: runs its handler and waits for it. */
  async emit(action: string, source = "", value: unknown = null): Promise<void> {
    const h = this.handlers.get(action);
    if (!h) throw new Error(`no handler registered for ${action}`);
    await h({ seq: 1, action, source, keys: {}, value });
  }

  /** This store as the Host the helpers are typed against. */
  get host(): Host {
    return this as unknown as Host;
  }
}

const stripComments = (s: string) => s.replace(/<!--[\s\S]*?-->/g, "");

/** A kit template: its props and its body, comments removed. */
interface Template {
  props: string[];
  body: string;
}

/** The kit's templates (kit/kit.tui): id to props and body, comments removed. */
function templates(): Map<string, Template> {
  const kit = stripComments(readFileSync(new URL("../kit.tui", import.meta.url), "utf8"));
  const out = new Map<string, Template>();
  for (const m of kit.matchAll(/<template\s+id="([^"]+)"(?:\s+props="([^"]*)")?\s*>([\s\S]*?)<\/template>/g)) {
    out.set(m[1]!, { props: (m[2] ?? "").split(/\s+/).filter(Boolean), body: m[3]! });
  }
  return out;
}

const SLOT = "\u0000slot\u0000";

/**
 * The markup of a kit component document with its comments removed, for
 * checking that the paths a helper writes are ones the markup reads (a header
 * comment lists paths too, and would make any such check pass). Each
 * `<use template="k-NAME" with="PATH as a">` (self-closing, or with content
 * before `</use>`) is replaced by the template's body from kit/kit.tui as its
 * copy reads: every `a.` is written `PATH.` and every id `x` (and `#x`) is the
 * use's `ID__x`; a prop the use passes replaces its `{$name}`, and a prop it
 * leaves out removes the attribute that reads it (or empties the text); the
 * use's content replaces `<slot/>` and keeps its own paths and ids. A use
 * inside another use's content is expanded first. A `</use>` nested in a use
 * that is not self-closing is not supported.
 */
export function component(name: string): string {
  const src = readFileSync(new URL(`../components/${name}.tui`, import.meta.url), "utf8");
  return expandUses(src, `${name}.tui`);
}

/** component() over markup you give it: the kit's uses in `source` expanded; `label` names it in errors. */
export function expandUses(source: string, label: string): string {
  const tpl = templates();
  const src = stripComments(source);
  const expand = (text: string): string =>
    text.replace(/<use\b([^>]*?)(?:\/>|>([\s\S]*?)<\/use>)/g, (_all, attrs: string, fill: string | undefined) => {
      const attr = (n: string) => new RegExp(`(?:^|\\s)${n}="([^"]*)"`).exec(attrs)?.[1];
      const t = tpl.get(attr("template") ?? "");
      if (t === undefined) throw new Error(`${label}: <use${attrs}> names no kit template`);
      let body = t.body;
      for (const prop of t.props) {
        const value = attr(prop);
        const ref = `\\{\\$${prop}\\}`;
        if (value === undefined) {
          body = body.replace(new RegExp(`\\s+[\\w:-]+="${ref}"`, "g"), "").replace(new RegExp(ref, "g"), "");
        } else {
          // A function, so $& and $1 in a value stay literal.
          body = body.replace(new RegExp(ref, "g"), () => value);
        }
      }
      body = body.replace(/<slot\s*\/>/g, SLOT);
      for (const pair of (attr("with") ?? "").split(",")) {
        const [path, alias] = pair.trim().split(" as ");
        if (!path || !alias) continue;
        body = body
          .replace(new RegExp(`(?<=[{"\\s!])${alias}\\.`, "g"), `${path}.`)
          .replace(new RegExp(`\\{${alias}\\}`, "g"), `{${path}}`);
      }
      const id = attr("id");
      if (id) body = body.replace(/\bid="([^"]+)"/g, `id="${id}__$1"`).replace(/="#([^"]+)"/g, `="#${id}__$1"`);
      return body.split(SLOT).join(fill === undefined ? "" : expand(fill));
    });
  return expand(src);
}
