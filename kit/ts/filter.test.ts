import { describe, expect, test } from "bun:test";
import { FakeStore } from "./fake-store.ts";
import { createFilter, filterItems, match } from "./filter.ts";

const rows = [
  { id: "deploy", label: "Deploy to production", meta: { team: "ops" }, n: 3 },
  { id: "rollback", label: "Roll back the last deploy", meta: { team: "ops" }, n: 12 },
  { id: "logs", label: "Open the logs", meta: { team: "dev" }, n: 7 },
  { id: "theme", label: "Toggle the theme", meta: { team: "ui" }, n: 1 },
];
const ids = (items: unknown[]) => items.map((i) => (i as { id: string }).id);

describe("filterItems", () => {
  test("substring: every word, any order, ignoring case", () => {
    const f = ["label", "id"];
    const cases: [string, string[]][] = [
      ["", ["deploy", "rollback", "logs", "theme"]],
      ["DEPLOY", ["deploy", "rollback"]],
      ["the", ["rollback", "logs", "theme"]],
      ["back deploy", ["rollback"]],
      ["deploy back", ["rollback"]],
      ["zzz", []],
      ["  ", ["deploy", "rollback", "logs", "theme"]],
    ];
    for (const [q, want] of cases) expect(ids(filterItems(rows, q, f))).toEqual(want);
    expect(ids(filterItems(rows, "dev", ["meta.team"]))).toEqual(["logs"]);
    expect(ids(filterItems(rows, "12", ["n"]))).toEqual(["rollback"]);
    expect(filterItems([], "x", f)).toEqual([]);
  });

  test("fuzzy: letters in order, tighter matches first", () => {
    expect(ids(filterItems(rows, "dpl", ["label"], "fuzzy"))).toEqual(["deploy", "rollback"]);
    expect(ids(filterItems(rows, "log", ["label"], "fuzzy"))[0]).toBe("logs");
    expect(filterItems(rows, "zq", ["label"], "fuzzy")).toEqual([]);
    expect(match("fuzzy", "ba", "abc")).toBe(false);
    expect(match("fuzzy", "ac", "abc")).toBe(true);
    expect(match("substring", "BC", "abc")).toBe(true);
  });
});

describe("createFilter", () => {
  test("writes {items, any, count, total} when asked to recompute", async () => {
    const store = new FakeStore({ q: "", items: rows });
    const filter = createFilter(store.host, { source: "items", query: "q", out: "found", fields: ["label"] });
    await filter.recompute();
    expect(store.at("found.count")).toBe(4);
    await store.set("q", "deploy");
    // The TS host has no Derive: the store is stale until the parent recomputes.
    expect(store.at("found.count")).toBe(4);
    await filter.recompute();
    expect(store.at("found.count")).toBe(2);
    expect(store.at("found.total")).toBe(4);
    await filter.recompute("nope"); // an event's own value wins over the store
    expect(store.at("found.any")).toBe(false);
    expect(store.at("found.items")).toEqual([]);
  });
});
