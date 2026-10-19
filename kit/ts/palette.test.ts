import { describe, expect, test } from "bun:test";
import { component, FakeStore } from "./fake-store.ts";
import { createPalette } from "./palette.ts";

async function setup(mode: "substring" | "fuzzy" = "substring") {
  const store = new FakeStore();
  const ran: string[] = [];
  const run = (id: string) => () => void ran.push(id);
  const palette = await createPalette(
    store.host,
    { mode },
    { id: "deploy", title: "Deploy to production", key: "d", run: run("deploy") },
    { id: "rollback", title: "Roll back the last deploy", key: "r", run: run("rollback") },
    { id: "logs", title: "Open the logs", key: "l", run: run("logs") },
  );
  palette.register();
  return { store, palette, ran };
}

const titles = (store: FakeStore) => (store.at("found.items") as { label: string }[]).map((i) => i.label);

describe("palette", () => {
  test("writes the registry and found as palette.tui reads them", async () => {
    const { store } = await setup();
    expect(store.at("palette.commands")).toEqual([
      { id: "deploy", label: "Deploy to production", hint: "d" },
      { id: "rollback", label: "Roll back the last deploy", hint: "r" },
      { id: "logs", label: "Open the logs", hint: "l" },
    ]);
    expect(store.at("found.any")).toBe(true);
    const source = component("palette");
    for (const p of ["palette.query", "palette.cur", "palette.open", "palette.last", "found.items", "found.any"]) {
      expect(source).toContain(p);
    }
    // The TS host has no Derive, so the component must tell the parent the query changed.
    expect(source).toContain('on:change="palette_query"');
  });

  test("open, type, run", async () => {
    const { store, ran } = await setup();
    await store.emit("palette_open");
    expect(store.at("palette.open")).toBe(true);
    expect(store.at("palette.cur")).toBe("deploy");
    await store.set("palette.query", "roll"); // what the input's bind does
    await store.emit("palette_query", "palette__q", "roll");
    expect(titles(store)).toEqual(["Roll back the last deploy"]);
    await store.emit("palette_run", "palette__q");
    expect(ran).toEqual(["rollback"]); // the cursor's row was filtered away: the first match runs
    expect([store.at("palette.open"), store.at("palette.last")]).toEqual([false, "Roll back the last deploy"]);
  });

  test("the cursor's row runs when it is still in the list", async () => {
    const { store, ran } = await setup();
    await store.emit("palette_open");
    await store.set("palette.cur", "logs");
    await store.emit("palette_run");
    expect(ran).toEqual(["logs"]);
  });

  test("no match: run does nothing and the palette stays open", async () => {
    const { store, ran } = await setup();
    await store.emit("palette_open");
    await store.emit("palette_query", "palette__q", "zzz");
    expect(store.at("found.any")).toBe(false);
    await store.emit("palette_run");
    expect(ran).toEqual([]);
    expect(store.at("palette.open")).toBe(true);
    await store.emit("palette_close");
    expect(store.at("palette.open")).toBe(false);
  });

  test("opening again starts clean", async () => {
    const { store } = await setup();
    await store.emit("palette_open");
    await store.emit("palette_query", "palette__q", "logs");
    await store.emit("palette_close");
    await store.emit("palette_open");
    expect(store.at("palette.query")).toBe("");
    expect(titles(store).length).toBe(3);
  });

  test("fuzzy mode and add()", async () => {
    const { store, palette, ran } = await setup("fuzzy");
    await store.emit("palette_query", "palette__q", "dpl");
    expect(titles(store)).toEqual(["Deploy to production", "Roll back the last deploy"]);
    await palette.add({ id: "theme", title: "Toggle the theme", key: "t", run: () => void ran.push("theme") });
    await store.emit("palette_query", "palette__q", "tgl");
    expect(titles(store)).toEqual(["Toggle the theme"]);
    await palette.add({ id: "theme", title: "Switch the theme" }); // same id: replaces
    const rows = store.at("palette.commands") as { label: string }[];
    expect(rows.length).toBe(4);
    expect(rows[3]?.label).toBe("Switch the theme");
  });

  test("an existing palette.last is not overwritten", async () => {
    const store = new FakeStore({ palette: { last: "none" } });
    await createPalette(store.host, {}, { id: "a", title: "A" });
    expect(store.at("palette.last")).toBe("none");
  });
});
