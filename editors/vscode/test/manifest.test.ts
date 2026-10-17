// Checks that the manifest, the files it points to, and the snippets agree.
import { describe, expect, test } from "bun:test";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

const root = join(import.meta.dir, "..");
const read = (p: string) => JSON.parse(readFileSync(join(root, p), "utf8"));
const pkg = read("package.json");

describe("package.json", () => {
  test("identity", () => {
    expect(pkg.name).toBe("tuimark");
    expect(pkg.publisher).toBe("abdul-hamid-achik");
    expect(pkg.version).toMatch(/^0\.4\.\d+$/);
    expect(pkg.engines.vscode).toMatch(/^\^1\.\d+\.0$/);
  });

  test("@types/vscode is not newer than engines.vscode", () => {
    const want = String(pkg.engines.vscode).replace("^", "");
    expect(pkg.devDependencies["@types/vscode"]).toBe(want);
  });

  test("the only runtime dependency is the language client", () => {
    expect(Object.keys(pkg.dependencies)).toEqual(["vscode-languageclient"]);
  });

  test("main is the built client", () => {
    expect(pkg.main).toBe("./out/extension.js");
    expect(existsSync(join(root, "src/extension.ts"))).toBe(true);
  });

  test("it activates on both languages", () => {
    expect(pkg.activationEvents).toEqual(["onLanguage:tuimark", "onLanguage:tcss"]);
  });

  test("languages: .tui and .tcss, each with a configuration", () => {
    const byId = Object.fromEntries(pkg.contributes.languages.map((l: any) => [l.id, l]));
    expect(byId.tuimark.extensions).toEqual([".tui"]);
    expect(byId.tcss.extensions).toEqual([".tcss"]);
    for (const l of Object.values<any>(byId)) {
      const cfg = read(l.configuration);
      expect(cfg.comments.blockComment).toHaveLength(2);
      expect(cfg.brackets.length).toBeGreaterThan(0);
      expect(cfg.autoClosingPairs.length).toBeGreaterThan(0);
    }
  });

  test("grammars exist and carry the scope the manifest declares", () => {
    for (const g of pkg.contributes.grammars) {
      expect(read(g.path).scopeName).toBe(g.scopeName);
      expect(pkg.contributes.languages.some((l: any) => l.id === g.language)).toBe(true);
    }
  });

  test("snippets parse, have bodies, and use unique prefixes per language", () => {
    for (const s of pkg.contributes.snippets) {
      const defs = read(s.path) as Record<string, { prefix: string | string[]; body: string | string[] }>;
      const seen = new Set<string>();
      for (const [name, d] of Object.entries(defs)) {
        expect(d.body, name).toBeTruthy();
        for (const p of ([] as string[]).concat(d.prefix)) {
          expect(seen.has(p), `${s.language}: duplicate prefix ${p}`).toBe(false);
          seen.add(p);
        }
      }
      expect(seen.size).toBeGreaterThan(5);
    }
  });

  test("the snippets only use tags and attributes of the catalog", () => {
    const tags = new Set(
      "tui style keymap bind screen col row box scroll spacer text rule list item input button progress modal column table tabs tab sparkline hints".split(" "),
    );
    const defs = read("snippets/tui.code-snippets.json") as Record<string, { body: string | string[] }>;
    for (const [name, d] of Object.entries(defs)) {
      const body = ([] as string[]).concat(d.body).join("\n");
      for (const m of body.matchAll(/<\/?([a-z]+)/g)) {
        expect(tags.has(m[1]!), `${name}: <${m[1]}>`).toBe(true);
      }
    }
  });

  test("settings", () => {
    const props = pkg.contributes.configuration.properties;
    expect(props["tuimark.path"].default).toBe("tuimark");
    expect(props["tuimark.trace.server"].enum).toEqual(["off", "messages", "verbose"]);
    expect(props["tuimark.trace.server"].default).toBe("off");
  });

  test("the restart command is declared", () => {
    expect(pkg.contributes.commands.map((c: any) => c.command)).toContain("tuimark.restartServer");
  });
});
