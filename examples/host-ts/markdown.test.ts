// bun test: the reference producer of the kit's transcript blocks.
import { describe, expect, test } from "bun:test";
import { markdownToBlocks } from "./markdown.ts";

const md = (s: string, streaming = false) => markdownToBlocks(s, { streaming });

describe("blocks", () => {
	test("paragraphs, joined lines and blank-line separation", () => {
		expect(md("one\ntwo\n\nthree\n")).toEqual([
			{ id: 0, runs: ["one two"] },
			{ id: 1, runs: ["three"] },
		]);
	});

	test("headings", () => {
		expect(md("# Title\n\n### Sub ##\ntext")).toEqual([
			{ id: 0, runs: ["Title"], head: true },
			{ id: 1, runs: ["Sub"], head: true },
			{ id: 2, runs: ["text"] },
		]);
	});

	test("a fenced code block keeps its lines, tabs, and blank lines, and ends at the matching fence", () => {
		const blocks = md("```ts\nif (a) {\n\treturn 1;\n\n}\n```\nafter\n");
		expect(blocks).toEqual([
			{ id: 0, runs: ["if (a) {\n\treturn 1;\n\n}"], code: true },
			{ id: 1, runs: ["after"] },
		]);
		// A longer fence is closed only by a fence as long, of the same kind.
		expect(md("````\n```\nx\n````\n")).toEqual([{ id: 0, runs: ["```\nx"], code: true }]);
		expect(md("~~~\n```\n~~~\n")).toEqual([{ id: 0, runs: ["```"], code: true }]);
	});

	test("lists: one block per item, numbered, nested, and continued lines", () => {
		expect(md("- a\n- b\n  more\n  - c\n1. one\n2) two\n")).toEqual([
			{ id: 0, runs: ["• a"], item: true },
			{ id: 1, runs: ["• b more"], item: true },
			{ id: 2, runs: ["  • c"], item: true },
			{ id: 3, runs: ["1. one"], item: true },
			{ id: 4, runs: ["2. two"], item: true },
		]);
	});

	test("quotes join their lines, rules, and a list item ends a paragraph", () => {
		expect(md("> a\n> b\nplain\n- x\n---\n")).toEqual([
			{ id: 0, runs: ["a b"], quote: true },
			{ id: 1, runs: ["plain"] },
			{ id: 2, runs: ["• x"], item: true },
			{ id: 3, runs: [], rule: true },
		]);
		expect(md("***\n___\n")).toEqual([
			{ id: 0, runs: [], rule: true },
			{ id: 1, runs: [], rule: true },
		]);
	});

	test("an empty document has no block", () => {
		expect(md("")).toEqual([]);
		expect(md("\n\n  \n")).toEqual([]);
	});
});

describe("inline runs", () => {
	test("bold, emphasis and code", () => {
		expect(md("a **b** and *c* and `d` and __e__ and _f_")[0]?.runs).toEqual([
			"a ",
			{ text: "b", class: "k-strong" },
			" and ",
			{ text: "c", class: "k-em" },
			" and ",
			{ text: "d", class: "k-code" },
			" and ",
			{ text: "e", class: "k-strong" },
			" and ",
			{ text: "f", class: "k-em" },
		]);
	});

	test("markers inside code are text, and snake_case is not emphasis", () => {
		expect(md("`a **b** c` and snake_case_name")[0]?.runs).toEqual([{ text: "a **b** c", class: "k-code" }, " and snake_case_name"]);
	});

	test("an unclosed marker is text", () => {
		expect(md("a **b c")[0]?.runs).toEqual(["a **b c"]);
		expect(md("a *b c")[0]?.runs).toEqual(["a *b c"]);
		expect(md("a `b c")[0]?.runs).toEqual(["a `b c"]);
		expect(md("**b *c* d")[0]?.runs).toEqual(["**b ", { text: "c", class: "k-em" }, " d"]);
	});
});

describe("streaming", () => {
	test("an open fence is closed at the end of what has arrived", () => {
		expect(md("text\n\n```ts\nlet a = 1;\nlet b", true)).toEqual([
			{ id: 0, runs: ["text"] },
			{ id: 1, runs: ["let a = 1;\nlet b"], code: true },
		]);
		// The fence is complete once its closing line arrives.
		expect(md("```\nx\n```", true)).toEqual([{ id: 0, runs: ["x"], code: true }]);
	});

	test("an unclosed ** stays text while it streams, and is bold when it closes", () => {
		expect(md("a **bol", true)[0]?.runs).toEqual(["a **bol"]);
		expect(md("a **bold*", true)[0]?.runs).toEqual(["a **bold*"]);
		expect(md("a **bold**", true)[0]?.runs).toEqual(["a ", { text: "bold", class: "k-strong" }]);
	});

	test("a last line that is only the start of a marker is held back", () => {
		expect(md("para\n\n#", true)).toEqual([{ id: 0, runs: ["para"] }]);
		expect(md("para\n\n-", true)).toEqual([{ id: 0, runs: ["para"] }]);
		expect(md("para\n\n``", true)).toEqual([{ id: 0, runs: ["para"] }]);
		expect(md("para\n\n1.", true)).toEqual([{ id: 0, runs: ["para"] }]);
		// ... but not once it has text, or a line break, or when not streaming.
		expect(md("para\n\n# ti", true)).toEqual([
			{ id: 0, runs: ["para"] },
			{ id: 1, runs: ["ti"], head: true },
		]);
		expect(md("para\n\n#", false)).toEqual([
			{ id: 0, runs: ["para"] },
			{ id: 1, runs: ["#"] },
		]);
	});

	test("every prefix of a document keeps the blocks before the last", () => {
		const doc = "# T\n\nOne **two** `three`.\n\n- a\n- b\n\n```\ncode\n```\n\n> q\n\n---\nend\n";
		const whole = md(doc);
		let previous: ReturnType<typeof md> = [];
		for (let n = 0; n <= doc.length; n++) {
			const blocks = md(doc.slice(0, n), true);
			// All but the last block of the previous prefix are unchanged.
			const settled = previous.slice(0, -1);
			expect(blocks.slice(0, settled.length)).toEqual(settled);
			previous = blocks;
		}
		expect(md(doc, true)).toEqual(whole);
	});
});
