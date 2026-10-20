import { describe, expect, test } from "bun:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
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
    const store = new FakeStore({ palette: { last: "none", title: "Run", placeholder: "what to run" } });
    await createPalette(store.host, {}, { id: "a", title: "A" });
    expect(store.at("palette.last")).toBe("none");
    expect([store.at("palette.title"), store.at("palette.placeholder")]).toEqual(["Run", "what to run"]);
  });

  test("what it writes is every path palette.tui reads", async () => {
    const { store } = await setup();
    expect([store.at("palette.title"), store.at("palette.placeholder")]).toEqual([
      "Command palette",
      "type a command…",
    ]);
    // The paths of the component, as its copy reads them: {path} templates and
    // the attributes that read the store (an each alias, cmd., is per row).
    const source = component("palette");
    const paths = new Set<string>();
    for (const m of source.matchAll(/\{([^}\s]+)\}/g)) paths.add(m[1]!);
    for (const m of source.matchAll(/\s(?:bind|open|if|hidden|each|class:[a-z_][a-z0-9_-]*)="!?([^"\s]+)/g)) paths.add(m[1]!);
    const read = [...paths].filter((p) => p.startsWith("palette.") || p.startsWith("found."));
    expect(read).toEqual(expect.arrayContaining(["palette.title", "palette.placeholder", "palette.query", "found.items"]));
    for (const p of read) expect({ path: p, found: (await store.get(p)).found }).toEqual({ path: p, found: true });
  });

  // The real runtime over exactly what the helper wrote: no diagnostic, and
  // the open palette shows its title and placeholder. It builds the CLI, so
  // it needs Go; the Go helper's test checks the same with the runtime.
  test.skipIf(!Bun.which("go"))(
    "palette.tui over the helper's store: no diagnostic, title and placeholder shown",
    async () => {
      const { store } = await setup();
      await store.emit("palette_open");
      const root = fileURLToPath(new URL("../..", import.meta.url));
      const dir = mkdtempSync(join(tmpdir(), "kit-palette-"));
      try {
        const bin = join(dir, "tuimark");
        const build = Bun.spawnSync(["go", "build", "-o", bin, "./cmd/tuimark"], { cwd: root });
        if (build.exitCode !== 0) throw new Error(`go build ./cmd/tuimark: ${build.stderr.toString()}`);
        const data = join(dir, "store.json");
        writeFileSync(data, JSON.stringify(store.data));
        const doc = fileURLToPath(new URL("../components/palette.tui", import.meta.url));
        const validate = JSON.parse(Bun.spawnSync([bin, "validate", "--json", "--data", data, doc]).stdout.toString());
        expect(validate.diagnostics).toEqual([]);
        const dump = JSON.parse(
          Bun.spawnSync([bin, "dump", doc, "--data", data, "--cols", "100", "--rows", "30", "--format", "json"]).stdout.toString(),
        );
        expect(dump.errors).toEqual([]);
        const grid = (dump.grid as string[]).join("\n");
        expect(grid).toContain("Command palette");
        expect(grid).toContain("type a command…");
      } finally {
        rmSync(dir, { recursive: true, force: true });
      }
    },
    180_000,
  );
});
