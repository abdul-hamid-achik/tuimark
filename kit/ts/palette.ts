// palette: a command registry behind kit/components/palette.tui.
//
// Copy this file into your project. It depends on @tuimark/host (types) and on
// filter.ts (createFilter): copy that file with this one.
//
// The component reads palette.commands (the registry, [{id, label, hint}]),
// palette.query, found.items and found.any, and reads and writes palette.open
// and palette.cur. This helper writes the registry from your commands, keeps
// found with createFilter, and implements the actions:
//
//   palette_open   resets the query and the cursor, opens the palette
//   palette_close  closes it
//   palette_run    closes it and runs the command under the cursor (or the
//                  first match), and writes its title to palette.last
//   palette_query  the input's on:change: recomputes found
//
// The Go host keeps found with a Derive, so the list follows every keystroke
// with no handler. The TypeScript host has no Derive: the component's input
// fires palette_query on every change, and found is recomputed there. That is
// the one action a TypeScript parent needs that a Go host does not.
import type { Host, HostEvent } from "@tuimark/host";
import { createFilter, type FilterMode } from "./filter.ts";

type Ui = Pick<Host, "batch" | "get" | "set" | "on">;

/** One entry of the palette. */
export interface Command {
  /** Unique; the list's row key. */
  id: string;
  /** What the row shows and the query matches. */
  title: string;
  /** The shortcut shown at the right ("d", "^R"), or "". */
  key?: string;
  /** What the command does. */
  run?: () => unknown;
}

export interface PaletteOptions {
  /** The palette's store object. Default "palette". */
  path?: string;
  /** The filtered list the component reads. Default "found". */
  found?: string;
  /** "substring" (the default) or "fuzzy". */
  mode?: FilterMode;
}

/**
 * A command palette: registers the commands, writes palette.commands, keeps
 * found with createFilter, and offers the handlers of palette_open,
 * palette_close, palette_run and palette_query (register() installs them).
 * Async because it writes the initial state.
 */
export async function createPalette(ui: Ui, options: PaletteOptions = {}, ...commands: Command[]) {
  const path = options.path ?? "palette";
  const foundPath = options.found ?? "found";
  const cmds: Command[] = [];
  const filter = createFilter(ui, {
    source: `${path}.commands`,
    query: `${path}.query`,
    out: foundPath,
    fields: ["label", "id"],
    mode: options.mode,
  });

  /** Appends commands to the registry and rewrites palette.commands. A command whose id is already there replaces it. */
  async function add(...more: Command[]): Promise<void> {
    for (const c of more) {
      const i = cmds.findIndex((x) => x.id === c.id);
      if (i >= 0) cmds[i] = c;
      else cmds.push(c);
    }
    await ui.set(
      `${path}.commands`,
      cmds.map((c) => ({ id: c.id, label: c.title, hint: c.key ?? "" })),
    );
    await filter.recompute();
  }

  /** The handler of palette_open. */
  async function open(): Promise<void> {
    await ui.batch([
      { path: `${path}.query`, value: "" },
      { path: `${path}.cur`, value: cmds[0]?.id ?? "" },
      { path: `${path}.open`, value: true },
    ]);
    await filter.recompute("");
  }

  /** The handler of palette_close. */
  async function close(): Promise<void> {
    await ui.set(`${path}.open`, false);
  }

  /** The handler of palette_query: the query changed, so found is stale. */
  async function query(ev: HostEvent): Promise<void> {
    await filter.recompute(typeof ev.value === "string" ? ev.value : undefined);
  }

  /**
   * The handler of palette_run: runs the command under the cursor, or the
   * first match when the cursor's row was filtered away. With no match it
   * does nothing and the palette stays open, where its empty line says why.
   */
  async function run(): Promise<void> {
    const cur = (await ui.get(`${path}.cur`)).value;
    const items = (await ui.get(`${foundPath}.items`)).value;
    const rows = Array.isArray(items) ? (items as { id?: string }[]) : [];
    const id = rows.find((r) => r.id === cur)?.id ?? rows[0]?.id;
    if (id === undefined) return;
    const cmd = cmds.find((c) => c.id === id);
    await ui.batch([
      { path: `${path}.open`, value: false },
      { path: `${path}.last`, value: cmd?.title ?? "" },
    ]);
    await cmd?.run?.();
  }

  /** Registers the four handlers. */
  function register(): void {
    ui.on("palette_open", open);
    ui.on("palette_close", close);
    ui.on("palette_run", run);
    ui.on("palette_query", query);
  }

  await add(...commands);
  // The state the component reads, for a store that has none of it yet.
  const defaults: Record<string, unknown> = { open: false, query: "", cur: cmds[0]?.id ?? "", last: "" };
  for (const [name, value] of Object.entries(defaults)) {
    if (!(await ui.get(`${path}.${name}`)).found) await ui.set(`${path}.${name}`, value);
  }

  return { add, open, close, query, run, register };
}

export type Palette = Awaited<ReturnType<typeof createPalette>>;
