// sortpage: sort by a column (click to cycle) and paginate, for a table.
//
// Copy this file into your project. It depends only on @tuimark/host (types).
//
// A table shows the array it is given. A sort-page controller computes two
// paths from the full array and a small state object, so the table's `each`
// reads the sorted rows of the current page and its column titles read the
// arrows:
//
//   <table each="svc.rows as s" ...>
//     <column id="svc__name" title="{svc.meta.titles.name}" on:click="table_sort">{s.name}</column>
//   <text>{svc.meta.label}</text>          1–50 of 340
//
// `out` gets the rows; `meta` gets {page, pages, size, total, label, has_prev,
// has_next, titles}. The state ({col, desc, page, size}) is a store object.
// The Go host derives both paths on every write; the TypeScript host has no
// Derive, so cycle(), setPage() and the others recompute for you, and you call
// recompute() after the source array changes.
import type { Host } from "@tuimark/host";

type Ui = Pick<Host, "batch" | "get">;

export interface SortColumn {
  /** The field of each row it sorts by. */
  key: string;
  /** The title the header shows; the sorted column shows "Title ▲" or "Title ▼". */
  title: string;
}

export interface SortPageOptions {
  /** Path of the full array of objects. */
  source: string;
  /** Path of the state object. Default `${out}_state`. */
  state?: string;
  /** Path to write: the sorted rows of the current page. */
  out: string;
  /** Path to write: the page's numbers, label and column titles. */
  meta: string;
  columns: SortColumn[];
  /** Page size when the state has none. Default 50. */
  size?: number;
}

interface State {
  col: string;
  desc: boolean;
  page: number;
  size: number;
}

/** "first–last of total" (an en dash), or "0 of 0" when empty. */
export function pageLabel(page: number, size: number, total: number): string {
  if (total === 0) return "0 of 0";
  return `${(page - 1) * size + 1}–${Math.min(page * size, total)} of ${total}`;
}

/**
 * A copy of `rows` sorted by the field `key` (stable). Numbers sort before
 * text and compare numerically, text compares ignoring case, booleans put
 * false first, and a row without the field goes last in either direction. An
 * empty key keeps the order.
 */
export function sortRows(rows: unknown[], key: string, desc: boolean): unknown[] {
  const out = [...rows];
  if (key === "") return out;
  const keyed = out.map((row, idx) => ({ row, idx, v: sortValue(row, key) }));
  keyed.sort((a, b) => {
    if (a.v === undefined || b.v === undefined) {
      if (a.v === b.v) return a.idx - b.idx;
      return a.v === undefined ? 1 : -1; // missing goes last
    }
    const c = compare(a.v, b.v);
    return (desc ? -c : c) || a.idx - b.idx;
  });
  return keyed.map((k) => k.row);
}

type SortValue = number | boolean | string;

function sortValue(row: unknown, key: string): SortValue | undefined {
  let cur: unknown = row;
  for (const seg of key.split(".")) {
    if (typeof cur !== "object" || cur === null || Array.isArray(cur)) return undefined;
    cur = (cur as Record<string, unknown>)[seg];
  }
  if (typeof cur === "number" || typeof cur === "boolean") return cur;
  if (typeof cur === "string") return cur.toLowerCase();
  return undefined;
}

const rank = (v: SortValue): number => (typeof v === "number" ? 0 : typeof v === "boolean" ? 1 : 2);

function compare(a: SortValue, b: SortValue): number {
  if (rank(a) !== rank(b)) return rank(a) - rank(b);
  if (typeof a === "number" && typeof b === "number") return a < b ? -1 : a > b ? 1 : 0;
  if (typeof a === "boolean" && typeof b === "boolean") return Number(a) - Number(b);
  return a < b ? -1 : a > b ? 1 : 0;
}

export function createSortPage(ui: Ui, o: SortPageOptions) {
  const statePath = o.state ?? `${o.out}_state`;
  const defaultSize = o.size && o.size > 0 ? o.size : 50;

  async function readState(): Promise<State> {
    const raw = (await ui.get(statePath)).value;
    const m = typeof raw === "object" && raw !== null ? (raw as Record<string, unknown>) : {};
    const page = m.page;
    const size = m.size;
    return {
      col: typeof m.col === "string" ? m.col : "",
      desc: m.desc === true,
      page: typeof page === "number" && page >= 1 ? Math.trunc(page) : 1,
      size: typeof size === "number" && size >= 1 ? Math.trunc(size) : defaultSize,
    };
  }

  async function writeState(st: State): Promise<void> {
    await ui.batch([{ path: statePath, value: { ...st } }]);
    await recompute();
  }

  /** Writes `out` and `meta` from the store's source and state. Call it after the source changes. */
  async function recompute(): Promise<void> {
    const st = await readState();
    const src = (await ui.get(o.source)).value;
    const rows = sortRows(Array.isArray(src) ? src : [], st.col, st.desc);
    const total = rows.length;
    const pages = Math.max(1, Math.ceil(total / st.size));
    const page = Math.min(Math.max(st.page, 1), pages);
    const lo = (page - 1) * st.size;
    const titles: Record<string, string> = {};
    for (const c of o.columns) titles[c.key] = c.key === st.col ? `${c.title} ${st.desc ? "▼" : "▲"}` : c.title;
    await ui.batch([
      { path: o.out, value: rows.slice(lo, Math.min(page * st.size, total)) },
      {
        path: o.meta,
        value: {
          page, pages, size: st.size, total, label: pageLabel(page, st.size, total),
          has_prev: page > 1, has_next: page < pages, titles,
        },
      },
    ]);
  }

  /**
   * The click on a column header: a new column sorts ascending, the same
   * column again descending, a third time back to the source order. The page
   * goes back to 1.
   */
  async function cycle(col: string): Promise<void> {
    if (!o.columns.some((c) => c.key === col)) throw new Error(`kit: "${col}" is not a column of ${o.out}`);
    const st = await readState();
    if (st.col !== col) [st.col, st.desc] = [col, false];
    else if (!st.desc) st.desc = true;
    else [st.col, st.desc] = ["", false];
    st.page = 1;
    await writeState(st);
  }

  /** Goes to a page; the view clamps it to the pages that exist. */
  async function setPage(n: number): Promise<void> {
    await writeState({ ...(await readState()), page: Math.max(1, Math.trunc(n)) });
  }

  async function step(d: number): Promise<void> {
    const meta = (await ui.get(o.meta)).value as { page?: number; pages?: number } | undefined;
    const page = meta?.page ?? 1;
    const pages = meta?.pages ?? 1;
    await setPage(Math.min(Math.max(page + d, 1), Math.max(pages, 1)));
  }

  /** Changes the page size and goes back to page 1. */
  async function setSize(n: number): Promise<void> {
    await writeState({ ...(await readState()), size: Math.max(1, Math.trunc(n)), page: 1 });
  }

  return { cycle, setPage, nextPage: () => step(1), prevPage: () => step(-1), setSize, recompute };
}

export type SortPage = ReturnType<typeof createSortPage>;
