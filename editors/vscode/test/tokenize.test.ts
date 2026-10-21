// Tokenizes every real .tui and .tcss document of the repository with the two
// grammars and fails on any `invalid.illegal` scope: a document the runtime
// accepts must not light up as an error in the editor. It also pins the
// opposite direction, on small inputs the runtime rejects.
import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { createOnigScanner, createOnigString, loadWASM } from "vscode-oniguruma";
import { INITIAL, Registry, parseRawGrammar, type IGrammar } from "vscode-textmate";

const root = join(import.meta.dir, "..");
const repo = join(root, "..", "..");

await loadWASM(readFileSync(join(root, "node_modules/vscode-oniguruma/release/onig.wasm")).buffer as ArrayBuffer);

const files: Record<string, string> = {
  "text.tui": join(root, "syntaxes/tui.tmLanguage.json"),
  "source.tcss": join(root, "syntaxes/tcss.tmLanguage.json"),
};
const registry = new Registry({
  onigLib: Promise.resolve({ createOnigScanner, createOnigString }),
  loadGrammar: async (scope) => {
    const path = files[scope];
    return path ? parseRawGrammar(readFileSync(path, "utf8"), path) : null;
  },
});
const tui = (await registry.loadGrammar("text.tui")) as IGrammar;
const tcss = (await registry.loadGrammar("source.tcss")) as IGrammar;

function illegal(grammar: IGrammar, text: string): string[] {
  const found: string[] = [];
  let stack = INITIAL;
  text.split("\n").forEach((line, i) => {
    const r = grammar.tokenizeLine(line, stack);
    for (const t of r.tokens) {
      if (t.scopes.some((s) => s.includes("invalid.illegal"))) {
        found.push(`${i + 1}:${t.startIndex + 1} ${JSON.stringify(line.slice(t.startIndex, t.endIndex))}`);
      }
    }
    stack = r.ruleStack;
  });
  return found;
}

function scopesAt(grammar: IGrammar, line: string, needle: string): string[] {
  const r = grammar.tokenizeLine(line, INITIAL);
  const at = line.indexOf(needle);
  const tok = r.tokens.find((t) => t.startIndex <= at && at < t.endIndex);
  return tok ? tok.scopes : [];
}

function collect(dir: string, out: string[] = []): string[] {
  for (const name of readdirSync(dir)) {
    if (name === "node_modules" || name === ".git") continue;
    const p = join(dir, name);
    if (statSync(p).isDirectory()) collect(p, out);
    else if (/\.(tui|tcss)$/.test(name)) out.push(p);
  }
  return out;
}

// Documents that are wrong on purpose.
const rejected = /(^|\/)(bad_[^/]*|broken)\.tui$/;

describe("repository documents have no illegal tokens", () => {
  const docs = [...collect(join(repo, "examples")), ...collect(join(repo, "specs/fixtures")), ...collect(join(repo, "kit"))]
    .filter((p) => !rejected.test(p))
    // Frozen inputs of the runtime's own tests, copied verbatim from older releases.
    .filter((p) => !p.includes("/testdata/"));
  test("there are documents to check", () => {
    expect(docs.length).toBeGreaterThan(10);
  });
  for (const p of docs) {
    test(relative(repo, p), () => {
      const g = p.endsWith(".tcss") ? tcss : tui;
      expect(illegal(g, readFileSync(p, "utf8"))).toEqual([]);
    });
  }
});

// Forms `tuimark validate` accepts and that must not light up as errors: the
// condition and the brace of an @media on later lines (Allman style), and a
// <style> whose body is a CDATA section.
describe("valid forms spread over several lines have no illegal tokens", () => {
  const allman = "@media (min-cols: 80)\n{\n  .b { width: 50%; }\n}\n";
  const split = "@media\n  (theme: light) {\n  .b { color: red; }\n}\n";
  const comment = "@media /* c */ (theme: dark)\n/* d */\n{\n  .b { color: red; }\n}\n.after { gap: 1; }\n";
  test(".tcss: @media with the brace on the next line", () => {
    expect(illegal(tcss, allman)).toEqual([]);
  });
  test(".tcss: @media with the condition on the next line", () => {
    expect(illegal(tcss, split)).toEqual([]);
  });
  test(".tcss: comments inside the @media head", () => {
    expect(illegal(tcss, comment)).toEqual([]);
  });
  test(".tcss: a rule after a multi-line @media is a rule, not part of it", () => {
    const r = tcss.tokenizeLine(".after { gap: 1; }", tcss.tokenizeLine("}", tcss.tokenizeLine("  .b { gap: 0; }", tcss.tokenizeLine("{", tcss.tokenizeLine("@media (theme: dark)", INITIAL).ruleStack).ruleStack).ruleStack).ruleStack);
    expect(r.tokens.some((t) => t.scopes.some((s) => s.includes("meta.at-rule.media")))).toBe(false);
  });
  test(".tcss: a bad feature is still marked when the brace is on the next line", () => {
    expect(illegal(tcss, "@media (min-width: 80)\n{\n  a { gap: 1; }\n}\n").length).toBeGreaterThan(0);
  });
  test(".tui: @media over several lines in an inline <style>", () => {
    expect(illegal(tui, `<style>\n  ${allman.replaceAll("\n", "\n  ")}\n</style>`)).toEqual([]);
    expect(illegal(tui, `<style>\n  ${split.replaceAll("\n", "\n  ")}\n</style>`)).toEqual([]);
  });
  test(".tui: <style><![CDATA[ ... ]]></style>", () => {
    expect(illegal(tui, "<style><![CDATA[\n  #t { color: red; }\n]]></style>\n<text>{x}</text>")).toEqual([]);
    expect(illegal(tui, `<style><![CDATA[\n${allman}]]></style>`)).toEqual([]);
  });
  test(".tui: an unknown property in a CDATA <style> is still marked", () => {
    expect(illegal(tui, "<style><![CDATA[\n  #t { colour: red; }\n]]></style>").length).toBeGreaterThan(0);
  });
  for (const f of ["test/media-allman.tcss", "test/style-forms.tui"]) {
    test(f, () => {
      // Drop the assertion lines, as vscode-tmgrammar-test does before tokenizing.
      const isAssertion = f.endsWith(".tcss") ? /^\/\*\s*(<|\^|SYNTAX)/ : /^<!--/;
      const body = readFileSync(join(root, f), "utf8")
        .split("\n")
        .filter((l) => !isAssertion.test(l))
        .join("\n");
      expect(body).toContain("@media");
      expect(illegal(f.endsWith(".tcss") ? tcss : tui, body)).toEqual([]);
    });
  }
});

describe(".tui: what the runtime rejects is marked", () => {
  test("an unknown tag", () => {
    expect(scopesAt(tui, "<blink>x</blink>", "blink")).toContain("invalid.illegal.unknown-tag.tui");
  });
  test("a known tag is not", () => {
    expect(scopesAt(tui, "<list id='a'/>", "list").join(" ")).not.toContain("invalid");
  });
  test("the 0.4b tags are known; radio and option are not", () => {
    for (const t of ["include", "fragment", "template", "use", "checkbox", "select"]) {
      expect(illegal(tui, `<${t} id='a'/>`), t).toEqual([]);
    }
    for (const t of ["radio", "option"]) {
      expect(illegal(tui, `<${t} id='a'/>`).length, t).toBe(1);
    }
  });
  test("the 0.4c slot and {$name} prop references are known", () => {
    expect(illegal(tui, "<slot/>")).toEqual([]);
    expect(scopesAt(tui, "<slot/>", "slot")).toContain("entity.name.tag.tui");
    expect(illegal(tui, '<template id="c" props="title style">\n  <box title="{$title}" style="{$style}"><slot/></box>\n</template>')).toEqual([]);
    expect(illegal(tui, '<template id="c" props="t w a">\n  <text style="width: {$w}">{$t}: {x.y}</text>\n  <button on:click="{$a}" label="{$t}"/>\n</template>')).toEqual([]);
    expect(scopesAt(tui, "<text>{$title}</text>", "title")).toContain("variable.parameter.prop.tui");
    expect(scopesAt(tui, '<box title="{$title}"/>', "title}")).toContain("variable.parameter.prop.tui");
    expect(scopesAt(tui, '<box style="{$style}"/>', "style}")).toContain("variable.parameter.prop.tui");
  });
  test("a brace pair with a dollar sign that is not a prop name is still marked", () => {
    expect(illegal(tui, "<text>{$Title}</text>").length).toBe(1);
    expect(illegal(tui, "<text>{$}</text>").length).toBe(1);
    expect(illegal(tui, "<text>{$a b}</text>").length).toBe(1);
  });
  test("braces that do not hold exactly a path (V003)", () => {
    expect(illegal(tui, "<text>{ folder }</text>").length).toBe(1);
    expect(illegal(tui, "<text>{a + b}</text>").length).toBe(1);
    expect(illegal(tui, "<text>{a} of {b.c}</text>")).toEqual([]);
  });
  test("a lone brace is literal text", () => {
    expect(illegal(tui, "<text>a { b</text>")).toEqual([]);
    expect(illegal(tui, "<text>a } b</text>")).toEqual([]);
  });
  test("an entity outside the five XML ones (V005)", () => {
    expect(illegal(tui, "<text>&nbsp;</text>").length).toBe(1);
    expect(illegal(tui, "<text>&lt;&gt;&amp;&quot;&apos;</text>")).toEqual([]);
  });
  test("a <style> body is TCSS, a <style src/> is a tag", () => {
    expect(scopesAt(tui, "<style>a { gap: 1; }</style>", "gap")).toContain("support.type.property-name.tcss");
    const g = illegal(tui, "<style>\n  #a { gap: 1; }\n</style>\n<text>{x}</text>");
    expect(g).toEqual([]);
  });
  test("a tag over several lines keeps its attributes", () => {
    const text = '<table id="t"\n       each="rows as r"\n       on:select="pick">';
    const r = tui.tokenizeLine('       on:select="pick">', tui.tokenizeLine('       each="rows as r"', tui.tokenizeLine('<table id="t"', INITIAL).ruleStack).ruleStack);
    expect(r.tokens.some((t) => t.scopes.includes("entity.other.attribute-name.action.tui"))).toBe(true);
    expect(illegal(tui, text)).toEqual([]);
  });
});

describe(".tcss: what the runtime rejects is marked", () => {
  test("an unknown property (V003)", () => {
    expect(illegal(tcss, "a { colour: red; }").length).toBeGreaterThan(0);
    expect(illegal(tcss, "text { color: red; }")).toEqual([]);
  });
  test("the form controls are tags", () => {
    expect(illegal(tcss, "checkbox:focus { reverse: true; }\nselect > list { border: single; }")).toEqual([]);
  });
  test("forbidden selectors", () => {
    expect(illegal(tcss, "* { gap: 1; }").length).toBeGreaterThan(0);
    expect(illegal(tcss, "a:hover { gap: 1; }").length).toBeGreaterThan(0);
    expect(illegal(tcss, "text[x] { gap: 1; }").length).toBeGreaterThan(0);
  });
  test("the listed pseudo-classes are fine", () => {
    for (const p of ["focus", "selected", "disabled", "empty", "checked", "focus-within"]) {
      expect(illegal(tcss, `item:${p} { bold: true; }`)).toEqual([]);
    }
  });
  test("media features", () => {
    for (const f of ["max-cols: 80", "min-cols: 81", "max-rows: 20", "min-rows: 21", "theme: dark", "theme: light"]) {
      expect(illegal(tcss, `@media (${f}) { text { bold: true; } }`)).toEqual([]);
    }
    expect(illegal(tcss, "@media (min-width: 80) { text { bold: true; } }").length).toBeGreaterThan(0);
    expect(illegal(tcss, "@media (theme: auto) { text { bold: true; } }").length).toBeGreaterThan(0);
  });
  test("tokens, var(), and colors", () => {
    expect(illegal(tcss, ":root { --accent: bright-cyan; --x: #abc; --y: #A0B1C2; }")).toEqual([]);
    expect(illegal(tcss, "text { color: $accent; background: var(--bg); border-color: default; }")).toEqual([]);
  });
  test("a multi-line declaration list and a comment", () => {
    expect(illegal(tcss, "/* a\n   b */\ntext {\n  color:\n    red;\n}")).toEqual([]);
  });
});
