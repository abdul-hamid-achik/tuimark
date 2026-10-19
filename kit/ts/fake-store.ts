// A store and a handler table in memory, standing in for @tuimark/host in the
// helpers' tests. It is test support, not a helper: do not copy it.
import { readFileSync } from "node:fs";
import type { Host, HostEvent } from "@tuimark/host";

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

/**
 * The markup of a kit component document with its comments removed, for
 * checking that the paths a helper writes are ones the markup reads (a header
 * comment lists paths too, and would make any such check pass).
 */
export function component(name: string): string {
  return readFileSync(new URL(`../components/${name}.tui`, import.meta.url), "utf8").replace(/<!--[\s\S]*?-->/g, "");
}
