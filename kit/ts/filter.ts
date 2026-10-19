// filter: keep a filtered copy of an array in the store, with counts.
//
// Copy this file into your project. It depends only on @tuimark/host (types).
// palette.ts imports it, so copy this file with that one.
//
// Markup has no expressions, so it cannot filter a list. The Go host's Derive
// recomputes the result inside every store write; the TypeScript host has no
// Derive, so the parent recomputes: call recompute() after a write to the
// source, and from the event that tells you the query changed (an input's
// on:change). A filter is as live as the events you wire to it.
import type { Host } from "@tuimark/host";

type Ui = Pick<Host, "set" | "get">;

/** "substring": every word of the query appears, in any order. "fuzzy": the query's letters appear in order. */
export type FilterMode = "substring" | "fuzzy";

export interface FilterOptions {
  /** Path of the array of objects, e.g. "palette.commands". */
  source: string;
  /** Path of the query string, e.g. "palette.query". */
  query: string;
  /** Path to write, e.g. "found". */
  out: string;
  /** Fields to match, each a key or a dotted path ("owner.name"). */
  fields: string[];
  mode?: FilterMode;
}

export interface FilterResult {
  items: unknown[];
  any: boolean;
  count: number;
  total: number;
}

/** The pure part: the items of `rows` that match `query` in `fields`. Never undefined. */
export function filterItems(rows: unknown[], query: string, fields: string[], mode: FilterMode = "substring"): unknown[] {
  const words = query.toLowerCase().split(/\s+/).filter((w) => w !== "");
  const hits: { item: unknown; score: number; idx: number }[] = [];
  rows.forEach((row, idx) => {
    let ok = true;
    let score = 0;
    if (words.length > 0) {
      if (mode === "fuzzy") {
        const r = fuzzyScore(row, words.join(""), fields);
        ok = r !== undefined;
        score = r ?? 0;
      } else {
        ok = containsAll(row, words, fields);
      }
    }
    if (ok) hits.push({ item: row, score, idx });
  });
  if (mode === "fuzzy") hits.sort((a, b) => b.score - a.score || a.idx - b.idx);
  return hits.map((h) => h.item);
}

/** Whether `query` matches the single string `hay`. */
export function match(mode: FilterMode, query: string, hay: string): boolean {
  return filterItems([{ v: hay }], query, ["v"], mode).length === 1;
}

function containsAll(row: unknown, words: string[], fields: string[]): boolean {
  const hay = fields.map((f) => fieldText(row, f).toLowerCase()).join(" ");
  return words.every((w) => hay.includes(w));
}

function fuzzyScore(row: unknown, query: string, fields: string[]): number | undefined {
  let best: number | undefined;
  for (const f of fields) {
    const s = subsequenceScore(fieldText(row, f).toLowerCase(), [...query]);
    if (s !== undefined && (best === undefined || s > best)) best = s;
  }
  return best;
}

// Each matched letter scores 10, +8 when it follows the previous one directly,
// +6 at the start of a word, and the whole is reduced by the position of the
// first match.
function subsequenceScore(hay: string, q: string[]): number | undefined {
  const chars = [...hay];
  let score = 0;
  let qi = 0;
  let prev = -2;
  let first = -1;
  for (let i = 0; i < chars.length && qi < q.length; i++) {
    if (chars[i] !== q[qi]) continue;
    score += 10;
    if (i === prev + 1) score += 8;
    const before = chars[i - 1];
    if (i === 0 || before === undefined || !/[\p{L}\p{N}]/u.test(before)) score += 6;
    if (first < 0) first = i;
    prev = i;
    qi++;
  }
  return qi < q.length ? undefined : score - first;
}

/** The text of a field of an object: a string as it is, a number or boolean formatted, anything else "". */
export function fieldText(row: unknown, path: string): string {
  let cur: unknown = row;
  for (const seg of path.split(".")) {
    if (typeof cur !== "object" || cur === null || Array.isArray(cur)) return "";
    cur = (cur as Record<string, unknown>)[seg];
  }
  return typeof cur === "string" || typeof cur === "number" || typeof cur === "boolean" ? String(cur) : "";
}

/**
 * Writes `out` as `{items, any, count, total}` from the store's source and
 * query. Call `recompute()` after the source or the query changed; pass the
 * query when an event carries it, to not depend on the order of the write.
 */
export function createFilter(ui: Ui, o: FilterOptions) {
  async function recompute(query?: string): Promise<FilterResult> {
    const q = query ?? (await ui.get(o.query)).value;
    const src = (await ui.get(o.source)).value;
    const rows = Array.isArray(src) ? src : [];
    const items = filterItems(rows, typeof q === "string" ? q : "", o.fields, o.mode ?? "substring");
    const result: FilterResult = { items, any: items.length > 0, count: items.length, total: rows.length };
    await ui.set(o.out, result);
    return result;
  }
  return { recompute };
}

export type Filter = ReturnType<typeof createFilter>;
